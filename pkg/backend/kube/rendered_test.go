package kube

import (
	"context"
	"net/http"
	"testing"

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

func TestSecondStarRenderedHardware(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			hw := templated("1")
			hw.UID = "original"
			hw.Spec.UserData = ptr("#cloud-config")
			hw.Spec.BMCRef = &corev1.TypedLocalObjectReference{Kind: "Machine", Name: "stored-machine"}
			hw.Spec.Metadata = &tinkerbell.HardwareMetadata{Instance: &tinkerbell.MetadataInstance{SSHKeys: []string{"stored-key"}}}
			if enabled {
				hw.Spec.BMCRef.Name = "{{ .references.net.data.machine }}"
				hw.Spec.Metadata.Instance.SSHKeys[0] = "{{ .references.net.data.key }}"
			}
			original := hw.DeepCopy()
			scheme := runtime.NewScheme()
			builder := runtime.NewSchemeBuilder(corev1.AddToScheme, tinkerbell.AddToScheme, bmc.AddToScheme)
			if err := builder.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			storedMachine := &bmc.Machine{ObjectMeta: metav1.ObjectMeta{Name: "stored-machine", Namespace: "tink"}, Spec: bmc.MachineSpec{Connection: bmc.Connection{
				Host: "stored-host", AuthSecretRef: corev1.SecretReference{Name: "credentials", Namespace: "tink"},
			}}}
			renderedMachine := storedMachine.DeepCopy()
			renderedMachine.Name, renderedMachine.Spec.Connection.Host = "rendered-machine", "rendered-host"
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "tink"}, Data: map[string][]byte{"username": []byte("user"), "password": []byte("pass")}}
			indexName := func(object client.Object) []string { return []string{object.GetName()} }
			kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(hw, storedMachine, renderedMachine, secret).
				WithIndex(&tinkerbell.Hardware{}, NameIndex, indexName).WithIndex(&bmc.Machine{}, NameIndex, indexName).Build()
			options := func(options *cluster.Options) {
				options.NewClient = func(*rest.Config, client.Options) (client.Client, error) { return kubeClient, nil }
				options.MapperProvider = func(*rest.Config, *http.Client) (meta.RESTMapper, error) { return kubeClient.RESTMapper(), nil }
				options.NewCache = func(*rest.Config, cache.Options) (cache.Cache, error) {
					return &informertest.FakeInformers{Scheme: scheme}, nil
				}
			}
			backend, err := NewBackend(Backend{ClientConfig: &rest.Config{Host: "http://127.0.0.1"}}, options)
			if err != nil {
				t.Fatal(err)
			}
			backend.HardwareTemplating = enabled
			hws, resolver := &fakeHardware{}, &fakeResolver{refs: map[string]any{"net": map[string]any{"data": map[string]any{"machine": "rendered-machine", "key": "rendered-key"}}}}
			backend.store = newTestStore(hws, resolver, &fakeInformers{})
			defer backend.store.queue.ShutDown()
			hws.set(hw)
			filter := data.HardwareFilter{ByName: hw.Name, InNamespace: hw.Namespace}
			if enabled {
				if _, err := backend.FilterBMCMachine(ctx, filter); !apierrors.IsNotFound(err) {
					t.Fatalf("before first render, err = %v; want not-ready", err)
				}
				if !backend.store.render(ctx, client.ObjectKeyFromObject(hw)) {
					t.Fatal("initial render failed")
				}
			}
			got, err := backend.FilterBMCMachine(ctx, filter)
			if err != nil {
				t.Fatal(err)
			}
			wantHost, wantKey := "stored-host", "stored-key"
			if enabled {
				wantHost, wantKey = "rendered-host", "rendered-key"
			}
			if got.Host != wantHost || len(got.SSHPublicKeys) != 1 || got.SSHPublicKeys[0] != wantKey || got.User != "user" || got.Pass != "pass" {
				t.Fatalf("unexpected BMC/auth result: %+v", got)
			}
			if !enabled {
				if resolver.calls != 0 {
					t.Fatal("disabled authentication must not render or resolve references")
				}
				return
			}
			got.SSHPublicKeys[0] = "caller-mutated"
			again, err := backend.FilterBMCMachine(ctx, filter)
			if err != nil || again.SSHPublicKeys[0] != wantKey || resolver.calls != 1 {
				t.Fatal("authentication must use private cached data without resolving references")
			}
			stored := &tinkerbell.Hardware{}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(hw), stored); err != nil {
				t.Fatal(err)
			}
			if stored.Spec.BMCRef.Name != original.Spec.BMCRef.Name || stored.Spec.Metadata.Instance.SSHKeys[0] != original.Spec.Metadata.Instance.SSHKeys[0] {
				t.Fatal("rendered reader must not overwrite source templates")
			}
			stored.Spec.UserData = ptr("{{ .references.missing }}")
			if err := kubeClient.Update(ctx, stored); err != nil {
				t.Fatal(err)
			}
			hws.set(stored)
			if backend.store.render(ctx, client.ObjectKeyFromObject(stored)) {
				t.Fatal("broken render must fail")
			}
			if fallback, err := backend.FilterBMCMachine(ctx, filter); err != nil || fallback.Host != wantHost || fallback.SSHPublicKeys[0] != wantKey {
				t.Fatalf("same-UID fallback = %v, %v", fallback, err)
			}
			if err := kubeClient.Delete(ctx, stored); err != nil {
				t.Fatal(err)
			}
			replacement := original.DeepCopy()
			replacement.UID, replacement.ResourceVersion = "replacement", ""
			if err := kubeClient.Create(ctx, replacement); err != nil {
				t.Fatal(err)
			}
			if _, err := backend.FilterBMCMachine(ctx, filter); !apierrors.IsNotFound(err) {
				t.Fatalf("replacement must not inherit old authentication keys: %v", err)
			}
		})
	}
}

// hardwareNotFound mirrors how Smee and Tootles recognize a missing Hardware.
func hardwareNotFound(err error) bool {
	nf, ok := err.(interface{ NotFound() bool })
	return ok && nf.NotFound()
}
