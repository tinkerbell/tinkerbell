//go:build integration

package kubernetes_test

import (
	"context"
	"testing"

	. "github.com/tinkerbell/hegel/internal/backend/kubernetes"
	tinkv1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// TestBackend performs a simple sanity check on the backend initializing constructor to ensure
// it can in-fact talk to a real API server. More rigerous testing of business logic is performed
// in unit tests.
func TestBackend(t *testing.T) {
	// Configure a test environment and launch it.
	scheme := runtime.NewScheme()
	if err := tinkv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	env := envtest.Environment{
		Scheme: scheme,
		CRDDirectoryPaths: []string{
			// CRDs are not automatically updated and will require manual updates whenever
			// we bump our Tink repository dependency version.
			"testdata/integration",
		},
	}

	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := env.Stop(); err != nil {
			t.Logf("Stopping test env: %v", err)
		}
	}()

	// Build a client and add a Hardware resource.
	client, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	const ip = "10.10.10.10"
	const hostname = "foobar"

	hw := tinkv1.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: tinkv1.HardwareSpec{
			Interfaces: []tinkv1.Interface{
				{
					DHCP: &tinkv1.DHCP{
						IP: &tinkv1.IP{
							Address: ip,
							Family:  4,
						},
					},
				},
			},
			Metadata: &tinkv1.HardwareMetadata{
				Instance: &tinkv1.MetadataInstance{
					Hostname: hostname,
				},
			},
		},
	}

	if err := client.Create(context.Background(), &hw); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Construct the backend and attempt to retrieve our test Hardware resource.
	backend, err := NewBackend(BackendConfig{Context: ctx, ClientConfig: cfg})
	if err != nil {
		t.Fatal(err)
	}
	backend.WaitForCacheSync(ctx)

	instance, err := backend.GetEC2Instance(ctx, ip)
	if err != nil {
		t.Fatal(err)
	}

	if instance.Metadata.Hostname != hostname {
		t.Fatalf("Expected Hostname: %s; Received Hostname: %s\n", instance.Metadata.Hostname, hostname)
	}
}
