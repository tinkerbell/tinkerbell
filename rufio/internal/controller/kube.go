package controller

import (
	"context"
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

// toPowerState takes a raw BMC power state response and converts it to a v1alpha1.PowerState.
func toPowerState(state string) bmc.PowerState {
	// Normalize the response string for comparison.
	state = strings.ToLower(state)

	switch {
	case strings.Contains(state, "on"):
		return bmc.On
	case strings.Contains(state, "off"):
		return bmc.Off
	default:
		return bmc.Unknown
	}
}
