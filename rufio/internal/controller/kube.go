package controller

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// resolveAuthSecretRef Gets the Secret from the SecretReference.
// A SecretReference without a namespace refers to a Secret in namespace.
// Returns the username and password encoded in the Secret.
func resolveAuthSecretRef(ctx context.Context, c client.Client, secretRef v1.SecretReference, namespace string) (string, string, error) {
	secret := &v1.Secret{}
	key := secretKey(secretRef, namespace)

	if err := c.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", fmt.Errorf("secret %s not found: %w", key, err)
		}

		return "", "", fmt.Errorf("failed to retrieve secret %s : %w", secretRef, err)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("'username' required in Machine secret")
	}

	password, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("'password' required in Machine secret")
	}

	return string(username), string(password), nil
}

// secretKey returns the key of the Secret that ref refers to. A ref without a namespace refers to a
// Secret in namespace, which is the namespace of the object holding ref.
func secretKey(ref v1.SecretReference, namespace string) types.NamespacedName {
	if ref.Namespace != "" {
		namespace = ref.Namespace
	}

	return types.NamespacedName{Namespace: namespace, Name: ref.Name}
}

// resolveCertificatePEM returns the PEM-encoded certificate stored at ref in the ConfigMap in
// namespace. It fails if the ConfigMap or key is missing, or the value isn't a PEM-encoded X.509
// certificate, so a bad reference is caught before anything is sent to a BMC.
func resolveCertificatePEM(ctx context.Context, c client.Client, namespace string, ref bmc.ConfigMapKeyReference) (string, error) {
	cm := &v1.ConfigMap{}
	key := types.NamespacedName{Namespace: namespace, Name: ref.Name}

	if err := c.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("configmap %s not found: %w", key, err)
		}

		return "", fmt.Errorf("failed to retrieve configmap %s: %w", key, err)
	}

	value, ok := cm.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %q not found in the data of configmap %s", ref.Key, key)
	}

	block, _ := pem.Decode([]byte(value))
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("key %q of configmap %s does not hold a PEM-encoded certificate", ref.Key, key)
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return "", fmt.Errorf("key %q of configmap %s does not hold a valid certificate: %w", ref.Key, key, err)
	}

	return value, nil
}

// toPowerState takes a raw BMC power state response and converts it to a v1alpha1.PowerState.
func toPowerState(state string) bmc.PowerState {
	// The IPMI providers report "Chassis Power is on"/"Chassis Power is off", and
	// the ipmitool provider returns ipmitool's combined stdout and stderr, so the
	// state line can follow warning lines. Redfish and the others report the bare
	// state. Check each line on its own.
	//
	// Only terminal states map to On/Off. Transition states such as Redfish's
	// "PoweringOff" and "PoweringOn" must stay Unknown so callers keep waiting.
	for line := range strings.Lines(state) {
		line = strings.TrimSpace(strings.ToLower(line))
		line = strings.TrimPrefix(line, "chassis power is ")
		switch line {
		case "on":
			return bmc.On
		case "off":
			return bmc.Off
		}
	}

	return bmc.Unknown
}
