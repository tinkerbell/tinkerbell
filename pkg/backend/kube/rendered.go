package kube

import (
	"context"
	"fmt"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/metadata"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const renderWorkers = 4

// RenderedHardware returns the rendering of hw to serve. Without rendering, it
// returns hw. A Hardware with no rendering to serve is not found.
func (b *Backend) RenderedHardware(_ context.Context, hw *tinkerbell.Hardware) (*tinkerbell.Hardware, error) {
	if b.store == nil {
		return hw, nil
	}
	rendered, ok := b.store.rendered(hw)
	if !ok {
		return nil, hardwareNotRenderedError{hardwareNotFoundError{name: hw.Name, namespace: hw.Namespace}}
	}
	return rendered, nil
}

// RenderedReader serves rendered Hardware. It only reads, so rendered Hardware
// cannot be written back over its templates.
type RenderedReader struct {
	backend *Backend
}

// RenderedReader returns a reader of rendered Hardware.
func (b *Backend) RenderedReader() *RenderedReader {
	return &RenderedReader{backend: b}
}

// FilterHardware is Backend.FilterHardware, returning the rendered Hardware.
func (r *RenderedReader) FilterHardware(ctx context.Context, opts data.HardwareFilter) (*tinkerbell.Hardware, error) {
	hw, err := r.backend.FilterHardware(ctx, opts)
	if err != nil {
		return nil, err
	}
	return r.backend.RenderedHardware(ctx, hw)
}

// hardwareNotRenderedError is not found to consumers, which cannot use a Hardware
// before its templates render, but says why.
type hardwareNotRenderedError struct {
	hardwareNotFoundError
}

func (h hardwareNotRenderedError) Error() string {
	return fmt.Sprintf("hardware %s/%s has templates that have not rendered successfully", h.namespace, h.name)
}

func (b *Backend) newRenderStore() (*renderStore, error) {
	metadataClient, err := metadata.NewForConfig(b.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("create Hardware reference metadata client: %w", err)
	}
	get := func(ctx context.Context, key types.NamespacedName) (*tinkerbell.Hardware, error) {
		hw := &tinkerbell.Hardware{}
		return hw, b.cluster.GetClient().Get(ctx, key, hw)
	}
	return newRenderStore(b.Logger.WithName("hardware-render"), b.cluster.GetCache(), b.cluster.GetRESTMapper(),
		metadataClient, b.Namespace, get, b.ResolveReferences, metrics.Registry), nil
}
