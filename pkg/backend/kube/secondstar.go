package kube

import (
	"context"
	"fmt"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell/bmc"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReadBMCMachine implements the handler.BackendReader interface and returns DHCP and netboot data based on a mac address.
func (b *Backend) ReadBMCMachine(ctx context.Context, name string) (*data.BMCMachine, error) {
	// get the hardware object, using the name, from the cluster
	// get the ssh public keys from the hardware object, add them to the return data.BMCMachine object
	// get the bmcRef from the hardware object
	// get the machine.bmc object, using the ref, from the cluster
	// get the bmc host, port, and cipher suites from the machine.bmc object, add them to the return data.BMCMachine object
	// get the secret object, using the machine.bmc object's secret ref, from the cluster
	// get the user and pass from the secret object, parse out the user and password and add them to the return data.BMCMachine object
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.GetByIP")
	defer span.End()

	hardwareList := &v1alpha1.HardwareList{}

	// list hardware objects with the given name
	// We List objects instead of a Get because namespace support is not implemented yet.
	if err := b.cluster.GetClient().List(ctx, hardwareList, &client.MatchingFields{".metadata.name": name}); err != nil {
		span.SetStatus(codes.Error, err.Error())

		return nil, fmt.Errorf("failed listing hardware for (%v): %w", name, err)
	}

	if len(hardwareList.Items) == 0 {
		err := hardwareNotFoundError{name: name, namespace: ternary(b.Namespace == "", "all namespaces", b.Namespace)}
		span.SetStatus(codes.Error, err.Error())

		return nil, err
	}

	if len(hardwareList.Items) > 1 {
		err := fmt.Errorf("got %d hardware objects for ip: %s, expected only 1", len(hardwareList.Items), name)
		span.SetStatus(codes.Error, err.Error())

		return nil, err
	}

	hw := hardwareList.Items[0]
	response := &data.BMCMachine{}
	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil {
		response.SSHPublicKeys = hw.Spec.Metadata.Instance.SSHKeys
	}
	bmcName := hw.Spec.BMCRef.Name

	if err := b.machine(ctx, bmcName, response); err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Backend) machine(ctx context.Context, name string, machine *data.BMCMachine) error {
	if name == "" {
		return fmt.Errorf("BMC Machine name is required")
	}

	bmcList := &bmc.MachineList{}
	if err := b.cluster.GetClient().List(ctx, bmcList, &client.MatchingFields{".metadata.name": name}); err != nil {
		return fmt.Errorf("failed listing bmc machine for (%v): %w", name, err)
	}
	if len(bmcList.Items) == 0 {
		return fmt.Errorf("bmc machine not found: %s", name)
	}
	if len(bmcList.Items) > 1 {
		return fmt.Errorf("got %d bmc machine objects for name: %s, expected only 1", len(bmcList.Items), name)
	}
	bmcMachine := bmcList.Items[0]

	machine.Host = bmcMachine.Spec.Connection.Host
	if bmcMachine.Spec.Connection.ProviderOptions != nil && bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL != nil {
		machine.Port = ternary(bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL.Port == 0, 623, bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL.Port)
		machine.CipherSuite = ternary(bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL.CipherSuite == "", "17", bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL.CipherSuite)
	}

	// lookup the secret object from the cluster. This gives us the user and pass.
	username, password, err := resolveAuthSecretRef(ctx, b.cluster.GetClient(), v1.SecretReference{Name: bmcMachine.Spec.Connection.AuthSecretRef.Name, Namespace: bmcMachine.Spec.Connection.AuthSecretRef.Namespace})
	if err != nil {
		return err
	}
	machine.User = username
	machine.Pass = password

	return nil
}

// resolveAuthSecretRef Gets the Secret from the SecretReference.
// Returns the username and password encoded in the Secret.
func resolveAuthSecretRef(ctx context.Context, c client.Client, secretRef v1.SecretReference) (string, string, error) {
	secret := &v1.Secret{}
	key := types.NamespacedName{Namespace: secretRef.Namespace, Name: secretRef.Name}

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
