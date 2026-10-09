package kube

import (
	"context"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func newFakeBackend(t *testing.T, cfg Backend, objs ...client.Object) *Backend {
	t.Helper()
	scheme := runtime.NewScheme()
	builder := runtime.NewSchemeBuilder(corev1.AddToScheme, tinkerbell.AddToScheme, bmc.AddToScheme)
	if err := builder.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	indexName := func(object client.Object) []string { return []string{object.GetName()} }
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithIndex(&tinkerbell.Hardware{}, NameIndex, indexName).WithIndex(&bmc.Machine{}, NameIndex, indexName).Build()
	options := func(options *cluster.Options) {
		options.NewClient = func(*rest.Config, client.Options) (client.Client, error) { return kubeClient, nil }
		options.MapperProvider = func(*rest.Config, *http.Client) (meta.RESTMapper, error) { return kubeClient.RESTMapper(), nil }
		options.NewCache = func(*rest.Config, cache.Options) (cache.Cache, error) {
			return &informertest.FakeInformers{Scheme: scheme}, nil
		}
	}
	cfg.ClientConfig = &rest.Config{Host: "http://127.0.0.1"}
	b, err := NewBackend(cfg, options)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNewBackendRendering(t *testing.T) {
	if b := newFakeBackend(t, Backend{}); b.store != nil {
		t.Fatal("rendering disabled must not build a render store")
	}
	registry := metrics.Registry
	metrics.Registry = prometheus.NewRegistry()
	t.Cleanup(func() { metrics.Registry = registry })
	b := newFakeBackend(t, Backend{Rendering: true})
	if b.store == nil {
		t.Fatal("rendering enabled must build a render store")
	}
	b.store.queue.ShutDown()
}

func TestRenderedReader(t *testing.T) {
	ctx := context.Background()
	hw := templated("1")
	b := newFakeBackend(t, Backend{}, hw)
	filter := data.HardwareFilter{ByName: hw.Name, InNamespace: hw.Namespace}

	stored, err := b.RenderedReader().FilterHardware(ctx, filter)
	if err != nil || *stored.Spec.UserData != *hw.Spec.UserData {
		t.Fatalf("without rendering got %v, %v; want the stored Hardware", stored, err)
	}

	hws, res := &fakeHardware{}, &fakeResolver{refs: netRefs("example.org")}
	b.store = newTestStore(hws, res, &fakeInformers{})
	defer b.store.queue.ShutDown()
	hws.set(stored)
	if _, err := b.RenderedReader().FilterHardware(ctx, filter); !apierrors.IsNotFound(err) {
		t.Fatalf("err = %v, want not found before the first render", err)
	}
	b.store.render(ctx, client.ObjectKeyFromObject(hw))
	if got, err := b.RenderedReader().FilterHardware(ctx, filter); err != nil || *got.Spec.UserData != "domain=example.org" {
		t.Fatalf("got %v, %v; want the rendering", got, err)
	}
	res.set(nil, nil)
	b.store.render(ctx, client.ObjectKeyFromObject(hw))
	if _, err := b.RenderedReader().FilterHardware(ctx, filter); !apierrors.IsNotFound(err) {
		t.Fatalf("err = %v, want a failed rendering to be not found", err)
	}
}

func TestFilterBMCMachineRendered(t *testing.T) {
	ctx := context.Background()
	hw := templated("1")
	hw.Spec.UserData = nil
	hw.Spec.BMCRef = &corev1.TypedLocalObjectReference{Kind: "Machine", Name: "{{ .references.net.data.machine }}"}
	hw.Spec.Metadata = &tinkerbell.HardwareMetadata{Instance: &tinkerbell.MetadataInstance{SSHKeys: []string{"{{ .references.net.data.key }}"}}}
	machine := &bmc.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m", Namespace: "tink"}, Spec: bmc.MachineSpec{Connection: bmc.Connection{
		Host: "host", AuthSecretRef: corev1.SecretReference{Name: "credentials", Namespace: "tink"},
	}}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "tink"}, Data: map[string][]byte{"username": []byte("user"), "password": []byte("pass")}}
	b := newFakeBackend(t, Backend{}, hw, machine, secret)
	filter := data.HardwareFilter{ByName: hw.Name, InNamespace: hw.Namespace}
	stored, err := b.FilterHardware(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	hws := &fakeHardware{}
	b.store = newTestStore(hws, &fakeResolver{refs: map[string]any{"net": map[string]any{"data": map[string]any{"machine": "m", "key": "k"}}}}, &fakeInformers{})
	defer b.store.queue.ShutDown()
	hws.set(stored)
	b.store.render(ctx, client.ObjectKeyFromObject(hw))

	got, err := b.FilterBMCMachine(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "host" || len(got.SSHPublicKeys) != 1 || got.SSHPublicKeys[0] != "k" {
		t.Fatalf("FilterBMCMachine = %+v, want the rendered Machine name and SSH key", got)
	}
}
