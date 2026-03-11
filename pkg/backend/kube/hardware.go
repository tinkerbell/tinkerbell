package kube

import (
	"context"
	"fmt"

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

// ReadHardware looks up a Hardware object by name and namespace using a direct Get.
func (b *Backend) ReadHardware(ctx context.Context, name, namespace string) (*v1alpha1.Hardware, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.ReadHardware")
	defer span.End()

	hw := &v1alpha1.Hardware{}
	if err := b.cluster.GetClient().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, hw); err != nil {
		return nil, fmt.Errorf("failed to get hardware %s/%s: %w", namespace, name, err)
	}

	return hw, nil
}

// FilterHardware looks up a single Hardware object using selector-based list filtering.
// Exactly one result is expected; zero results returns a not-found error and multiple results returns a multiple-found error.
func (b *Backend) FilterHardware(ctx context.Context, opts data.HardwareFilter) (*v1alpha1.Hardware, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "backend.kube.FilterHardware")
	defer span.End()

	hwList := &v1alpha1.HardwareList{}
	if err := b.cluster.GetClient().List(ctx, hwList, hardwareListOptions(opts)...); err != nil {
		return nil, fmt.Errorf("failed to list hardware %s: %w", filterQueryDesc(opts), err)
	}

	nsDesc := ternary(opts.InNamespace == "", "all namespaces", opts.InNamespace)

	if len(hwList.Items) == 0 {
		err := hardwareNotFoundError{name: filterQueryDesc(opts), namespace: nsDesc}
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	if len(hwList.Items) > 1 {
		err := &foundMultipleHardwareError{id: filterQueryDesc(opts), namespace: nsDesc, count: len(hwList.Items)}
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	return &hwList.Items[0], nil
}

// filterQueryDesc builds a human-readable description of a hardware filter for error messages.
func filterQueryDesc(opts data.HardwareFilter) string {
	nsDesc := opts.InNamespace
	if nsDesc == "" {
		nsDesc = "all namespaces"
	}
	desc := fmt.Sprintf("in %s", nsDesc)
	if opts.ByName != "" {
		desc = fmt.Sprintf("%s with name %q", desc, opts.ByName)
	}
	if opts.ByAgentID != "" {
		desc = fmt.Sprintf("%s with agentID %q", desc, opts.ByAgentID)
	}
	if opts.ByMACAddress != "" {
		desc = fmt.Sprintf("%s with MAC %q", desc, opts.ByMACAddress)
	}
	if opts.ByIPAddress != "" {
		desc = fmt.Sprintf("%s with IP %q", desc, opts.ByIPAddress)
	}
	if opts.ByInstanceID != "" {
		desc = fmt.Sprintf("%s with instanceID %q", desc, opts.ByInstanceID)
	}
	return desc
}

func (b *Backend) ListHardware(ctx context.Context, opts data.HardwareFilter) ([]v1alpha1.Hardware, error) {
	list := &v1alpha1.HardwareList{}
	err := b.cluster.GetClient().List(ctx, list, hardwareListOptions(opts)...)
	if err != nil {
		return nil, err
	}

	return list.Items, nil
}

func hardwareListOptions(opts data.HardwareFilter) []client.ListOption {
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
	if opts.ByIPAddress != "" {
		los = append(los, client.MatchingFields{IPAddrIndex: opts.ByIPAddress})
	}
	if opts.ByMACAddress != "" {
		los = append(los, client.MatchingFields{MACAddrIndex: opts.ByMACAddress})
	}
	if opts.ByInstanceID != "" {
		los = append(los, client.MatchingFields{InstanceIDIndex: opts.ByInstanceID})
	}

	return los
}

func (b *Backend) UpdateHardware(ctx context.Context, hw *v1alpha1.Hardware, opts data.UpdateOptions) error {
	cc := b.cluster.GetClient()

	if p, err := patchFromOpts(opts); err != nil {
		return fmt.Errorf("invalid patch options for hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	} else if p != nil {
		if opts.StatusOnly {
			if err := cc.Status().Patch(ctx, hw, p); err != nil {
				return fmt.Errorf("failed to patch hardware status %s/%s: %w", hw.Namespace, hw.Name, err)
			}
			return nil
		}
		if err := cc.Patch(ctx, hw, p); err != nil {
			return fmt.Errorf("failed to patch hardware %s/%s: %w", hw.Namespace, hw.Name, err)
		}
		return nil
	}

	if opts.StatusOnly {
		if err := cc.Status().Update(ctx, hw); err != nil {
			return fmt.Errorf("failed to update hardware status %s/%s: %w", hw.Namespace, hw.Name, err)
		}
		return nil
	}
	if err := cc.Update(ctx, hw); err != nil {
		return fmt.Errorf("failed to update hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	return nil
}
