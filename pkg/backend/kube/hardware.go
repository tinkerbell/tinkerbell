package kube

import (
	"context"
	"fmt"
	"strings"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *Backend) CreateHardware(ctx context.Context, hw *v1alpha1.Hardware) error {
	if err := b.cluster.GetClient().Create(ctx, hw); err != nil {
		return fmt.Errorf("failed to create hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	return nil
}

// ReadHardware looks up a Hardware object by name or agent ID depending on the provided LookupOptions.
// If the LookupOptions does not specify a lookup method, it will default to looking up by agent ID for backwards compatibility.
func (b *Backend) ReadHardware(ctx context.Context, id, namespace string, opts data.ReadListOptions) (*v1alpha1.Hardware, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.ReadHardware")
	defer span.End()
	// If an id is in the format of namespace/name, we should split it and use the namespace and name to look up the hardware object.
	// This allows support for namespaces outside of where the tinkerbell controller/server live.
	hwNamespace, hwName, found := strings.Cut(id, "/")
	if !found {
		hwName = id
		hwNamespace = namespace
	}

	// If no namespace is provided we must do a list operation.
	// If the list option, byAgentID is provided, then we must also do a list operation,
	// regardless of whether a namespace is provided or not, as we cannot do a get by agent ID.

	if hwNamespace == "" || opts.ByName == "" {
		hwList := &v1alpha1.HardwareList{}

		if hwNamespace != opts.InNamespace {
			opts.InNamespace = hwNamespace
		}

		if err := b.cluster.GetClient().List(ctx, hwList, hardwareListOptions(opts)...); err != nil {
			return nil, fmt.Errorf("failed to list hardware with name %s across all namespaces: %w", hwName, err)
		}
		if len(hwList.Items) == 0 {
			err := hardwareNotFoundError{name: hwName, namespace: ternary(hwNamespace == "", "all namespaces", hwNamespace)}
			span.SetStatus(codes.Error, err.Error())

			return nil, err
		}

		if len(hwList.Items) > 1 {
			// This is unexpected, as we should not have multiple hardware objects with the same name.
			err := &foundMultipleHardwareError{id: hwName, namespace: ternary(hwNamespace == "", "all namespaces", hwNamespace), count: len(hwList.Items)}
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		return &hwList.Items[0], nil
	}
	// We only get here if a namespace is provided and we are not looking up by agent ID, so we can do a get by name and namespace.
	hw := &v1alpha1.Hardware{}
	if err := b.cluster.GetClient().Get(ctx, types.NamespacedName{Name: hwName, Namespace: hwNamespace}, hw); err != nil {
		return nil, fmt.Errorf("failed to get hardware %s/%s: %w", hwNamespace, hwName, err)
	}

	return hw, nil
}

func (b *Backend) ListHardware(ctx context.Context, namespace string, opts data.ReadListOptions) ([]v1alpha1.Hardware, error) {
	list := &v1alpha1.HardwareList{}
	err := b.cluster.GetClient().List(ctx, list, hardwareListOptions(opts)...)
	if err != nil {
		return nil, err
	}

	return list.Items, nil
}

func hardwareListOptions(opts data.ReadListOptions) []client.ListOption {
	los := []client.ListOption{}
	if opts.InNamespace != "" {
		los = append(los, client.InNamespace(opts.InNamespace))
	}
	if opts.ByAgentID != "" {
		los = append(los, client.MatchingFields{HardwareAgentIDIndex: opts.ByAgentID})
	}
	if opts.ByName != "" {
		los = append(los, client.MatchingFields{NameIndex: opts.ByName})
	}
	if opts.Hardware.ByIPAddress != "" {
		los = append(los, client.MatchingFields{IPAddrIndex: opts.Hardware.ByIPAddress})
	}
	if opts.Hardware.ByMACAddress != "" {
		los = append(los, client.MatchingFields{MACAddrIndex: opts.Hardware.ByMACAddress})
	}
	if opts.Hardware.ByInstanceID != "" {
		los = append(los, client.MatchingFields{InstanceIDIndex: opts.Hardware.ByInstanceID})
	}

	return los
}

func (b *Backend) UpdateHardware(ctx context.Context, hw *v1alpha1.Hardware, opts data.UpdateOptions) error {
	if opts.StatusOnly {
		if err := b.cluster.GetClient().Status().Update(ctx, hw); err != nil {
			return fmt.Errorf("failed to update hardware status %s/%s: %w", hw.Namespace, hw.Name, err)
		}
		return nil
	}
	if err := b.cluster.GetClient().Update(ctx, hw); err != nil {
		return fmt.Errorf("failed to update hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	return nil
}

func (b *Backend) DeleteHardware(ctx context.Context, id, namespace string) error {
	return nil
}
