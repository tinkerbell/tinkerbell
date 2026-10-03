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

// renderWorkers is the number of Hardware rendered concurrently.
const renderWorkers = 4

// RenderedHardware returns the stored or latest successfully rendered Hardware.
// When templating is enabled and no successful rendering exists yet, it returns
// a not-found error until the render store is ready.
func (b *Backend) RenderedHardware(_ context.Context, hw *tinkerbell.Hardware) (*tinkerbell.Hardware, error) {
	if !b.HardwareTemplating {
		return hw, nil
	}
	if b.store == nil {
		return nil, hardwareNotRenderedError{hardwareNotFoundError{name: hw.Name, namespace: hw.Namespace}}
	}
	rendered, ok := b.store.rendered(hw)
	if !ok {
		return nil, hardwareNotRenderedError{hardwareNotFoundError{name: hw.Name, namespace: hw.Namespace}}
	}
	return rendered, nil
}

// RenderedReader serves Hardware rendered under Hardware-wide reference policy.
// It only reads, so rendered Hardware cannot be written back over its templates.
type RenderedReader struct {
	stored hardwareFilterer
	store  *renderStore
}

type hardwareFilterer interface {
	FilterHardware(ctx context.Context, opts data.HardwareFilter) (*tinkerbell.Hardware, error)
}

// RenderedReader returns a reader of Hardware rendered under Hardware-wide reference policy.
func (b *Backend) RenderedReader() *RenderedReader {
	return &RenderedReader{stored: b, store: b.store}
}

// FilterHardware is Backend.FilterHardware, returning the rendered Hardware. A
// Hardware whose templates have never rendered successfully is not found.
func (r *RenderedReader) FilterHardware(ctx context.Context, opts data.HardwareFilter) (*tinkerbell.Hardware, error) {
	hw, err := r.stored.FilterHardware(ctx, opts)
	if err != nil {
		return nil, err
	}
	if r.store == nil {
		return hw, nil
	}
	rendered, ok := r.store.rendered(hw)
	if !ok {
		return nil, hardwareNotRenderedError{hardwareNotFoundError{name: hw.Name, namespace: hw.Namespace}}
	}
	return rendered, nil
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
		metadataClient, get, b.ResolveReferences, metrics.Registry), nil
}
