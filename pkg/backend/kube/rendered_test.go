package kube

import (
	"context"
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type storedHardware struct{ hw *tinkerbell.Hardware }

func (s storedHardware) FilterHardware(context.Context, data.HardwareFilter) (*tinkerbell.Hardware, error) {
	return s.hw.DeepCopy(), nil
}

func TestRenderedReader(t *testing.T) {
	ctx := context.Background()
	hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}
	store := newTestStore(hws, res, &fakeInformers{})
	hw := templated("1")
	hws.set(hw)
	r := &RenderedReader{stored: storedHardware{hw}, store: store}

	_, err := r.FilterHardware(ctx, data.HardwareFilter{})
	if !apierrors.IsNotFound(err) || !hardwareNotFound(err) {
		t.Fatalf("err = %v, want a not found error before the first render", err)
	}

	store.render(ctx, client.ObjectKeyFromObject(hw))
	got, err := r.FilterHardware(ctx, data.HardwareFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if *got.Spec.UserData != "domain=example.org" {
		t.Fatalf("userData = %q", *got.Spec.UserData)
	}
}

func TestRenderedReaderWithoutStore(t *testing.T) {
	hw := templated("1")
	r := &RenderedReader{stored: storedHardware{hw}}

	got, err := r.FilterHardware(context.Background(), data.HardwareFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != hw.Name {
		t.Fatalf("Hardware = %q, want %q", got.Name, hw.Name)
	}
}

func TestRenderedHardware(t *testing.T) {
	hw := templated("1")
	if got, err := (&Backend{}).RenderedHardware(context.Background(), hw); err != nil || got != hw {
		t.Fatalf("with templating disabled got %v, %v; want stored Hardware", got, err)
	}

	hws := &fakeHardware{}
	store := newTestStore(hws, &fakeResolver{refs: netRefs("example.org")}, &fakeInformers{})
	b := &Backend{HardwareTemplating: true, store: store}
	hws.set(hw)
	if got, err := b.RenderedHardware(context.Background(), hw); err == nil || got != nil {
		t.Fatalf("before first render got %v, %v; want not-ready error", got, err)
	}
	if !store.render(context.Background(), client.ObjectKeyFromObject(hw)) {
		t.Fatal("render failed")
	}
	got, err := b.RenderedHardware(context.Background(), hw)
	if err != nil {
		t.Fatal(err)
	}
	if *got.Spec.UserData != "domain=example.org" {
		t.Fatalf("UserData = %q, want rendered value", *got.Spec.UserData)
	}
}

// hardwareNotFound mirrors how Smee and Tootles recognize a missing Hardware.
func hardwareNotFound(err error) bool {
	nf, ok := err.(interface{ NotFound() bool })
	return ok && nf.NotFound()
}
