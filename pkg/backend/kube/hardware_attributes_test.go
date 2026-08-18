package kube

import (
	"context"
	"net/http"
	"testing"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// newAttributesTestBackend builds a Backend backed by a fake client that keeps
// controller-runtime's default FieldManagedObjectTracker, needed for real
// Server-Side Apply patch semantics against the Hardware status subresource.
func newAttributesTestBackend(t *testing.T, objs ...client.Object) *Backend {
	t.Helper()

	rs := runtime.NewScheme()
	if err := scheme.AddToScheme(rs); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(rs); err != nil {
		t.Fatal(err)
	}

	cl := fake.NewClientBuilder().
		WithScheme(rs).
		WithStatusSubresource(&v1alpha1.Hardware{}).
		WithObjects(objs...).
		Build()

	fn := func(o *cluster.Options) {
		o.NewClient = func(*rest.Config, client.Options) (client.Client, error) {
			return cl, nil
		}
		o.MapperProvider = func(*rest.Config, *http.Client) (meta.RESTMapper, error) {
			return cl.RESTMapper(), nil
		}
		o.NewCache = func(*rest.Config, cache.Options) (cache.Cache, error) {
			return &informertest.FakeInformers{Scheme: cl.Scheme()}, nil
		}
	}

	b, err := NewBackend(Backend{ClientConfig: new(rest.Config)}, fn)
	if err != nil {
		t.Fatal(err)
	}
	go b.Start(context.Background())

	return b
}

func TestApplyHardwareInBandAttributes(t *testing.T) {
	hw := &v1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "hw-1", Namespace: "default"},
	}
	b := newAttributesTestBackend(t, hw)

	attrs := &v1alpha1.Attributes{
		CollectionMethod: "agent",
		BIOS:             &v1alpha1.BIOS{Vendor: "American Megatrends", FirmwareVersion: "1.2.3"},
	}
	if err := b.ApplyHardwareInBandAttributes(context.Background(), hw.Name, hw.Namespace, attrs); err != nil {
		t.Fatal(err)
	}

	got, err := b.ReadHardware(context.Background(), hw.Name, hw.Namespace)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Attributes == nil || got.Status.Attributes.InBand == nil {
		t.Fatal("Status.Attributes.InBand is nil, want it populated")
	}
	if got.Status.Attributes.InBand.CollectionMethod != "agent" {
		t.Errorf("CollectionMethod = %q, want agent", got.Status.Attributes.InBand.CollectionMethod)
	}
	if got.Status.Attributes.InBand.BIOS == nil || got.Status.Attributes.InBand.BIOS.Vendor != "American Megatrends" {
		t.Errorf("BIOS = %+v, want Vendor=American Megatrends", got.Status.Attributes.InBand.BIOS)
	}
}

func TestApplyHardwareInBandAttributes_DoesNotClobberOutOfBand(t *testing.T) {
	hw := &v1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "hw-1", Namespace: "default"},
	}
	b := newAttributesTestBackend(t, hw)

	// Simulate Rufio having already applied status.attributes.outOfBand under
	// its own field manager before tink-server applies status.attributes.inBand.
	oob := hw.DeepCopy()
	oob.Status.Attributes = &v1alpha1.HardwareAttributes{
		OutOfBand: &v1alpha1.Attributes{CollectionMethod: "redfish"},
	}
	if err := b.cluster.GetClient().Status().Update(context.Background(), oob); err != nil {
		t.Fatal(err)
	}

	if err := b.ApplyHardwareInBandAttributes(context.Background(), hw.Name, hw.Namespace, &v1alpha1.Attributes{CollectionMethod: "agent"}); err != nil {
		t.Fatal(err)
	}

	got, err := b.ReadHardware(context.Background(), hw.Name, hw.Namespace)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status.Attributes.OutOfBand == nil || got.Status.Attributes.OutOfBand.CollectionMethod != "redfish" {
		t.Errorf("OutOfBand = %+v, want it left untouched with CollectionMethod=redfish", got.Status.Attributes.OutOfBand)
	}
	if got.Status.Attributes.InBand == nil || got.Status.Attributes.InBand.CollectionMethod != "agent" {
		t.Errorf("InBand = %+v, want CollectionMethod=agent", got.Status.Attributes.InBand)
	}
}
