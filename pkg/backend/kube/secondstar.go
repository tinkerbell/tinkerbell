package kube

import (
	"context"
	"fmt"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FilterBMCMachine looks up a machine.bmc.tinkerbell.org object based on the bmcRef in the hardware object matching the given filter.
func (b *Backend) FilterBMCMachine(ctx context.Context, opts data.HardwareFilter) (*data.BMCMachine, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.FilterBMCMachine")
	defer span.End()

	hw, err := b.FilterHardware(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to filter hardware: %w", err)
	}
	response := &data.BMCMachine{}
	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil {
		response.SSHPublicKeys = hw.Spec.Metadata.Instance.SSHKeys
	}

	bmcMachine, err := b.filterMachine(ctx, hw.Spec.BMCRef.Name)
	if err != nil {
		return nil, err
	}

	response.Host = bmcMachine.Spec.Connection.Host
	if bmcMachine.Spec.Connection.ProviderOptions != nil && bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL != nil {
		response.Port = ternary(bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL.Port == 0, 623, bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL.Port)
		response.CipherSuite = ternary(bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL.CipherSuite == "", "17", bmcMachine.Spec.Connection.ProviderOptions.IPMITOOL.CipherSuite)
	}

	username, password, err := b.ReadAuthSecret(ctx, bmcMachine.Spec.Connection.AuthSecretRef.Name, bmcMachine.Spec.Connection.AuthSecretRef.Namespace)
	if err != nil {
		return nil, err
	}
	response.User = username
	response.Pass = password

	return response, nil
}

// filterMachine looks up a single bmc.Machine object by name.
// Exactly one result is expected; zero returns a not-found error and multiple returns a multiple-found error.
func (b *Backend) filterMachine(ctx context.Context, name string) (*bmc.Machine, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.filterMachine")
	defer span.End()

	if name == "" {
		return nil, fmt.Errorf("BMC Machine name is required")
	}

	bmcList := &bmc.MachineList{}
	if err := b.cluster.GetClient().List(ctx, bmcList, &client.MatchingFields{NameIndex: name}); err != nil {
		return nil, fmt.Errorf("failed listing bmc machine for (%v): %w", name, err)
	}

	if len(bmcList.Items) == 0 {
		err := fmt.Errorf("bmc machine not found: %s", name)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	if len(bmcList.Items) > 1 {
		err := fmt.Errorf("got %d bmc machine objects for name: %s, expected only 1", len(bmcList.Items), name)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return &bmcList.Items[0], nil
}

// ReadAuthSecret looks up a Secret by name and namespace using a direct Get.
// It expects the Secret to contain "username" and "password" keys in its data.
func (b *Backend) ReadAuthSecret(ctx context.Context, name, namespace string) (string, string, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.ReadAuthSecret")
	defer span.End()

	secret := &v1.Secret{}
	if err := b.cluster.GetClient().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return "", "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, name, err)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("'username' required in Machine secret %s/%s", namespace, name)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("'password' required in Machine secret %s/%s", namespace, name)
	}

	return string(username), string(password), nil
}
