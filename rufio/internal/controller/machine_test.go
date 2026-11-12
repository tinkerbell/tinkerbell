package controller_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMachineReconcile(t *testing.T) {
	tests := map[string]struct {
		provider  *testProvider
		shouldErr bool
		secret    *corev1.Secret
		machine   *bmc.Machine
	}{
		"success power on": {
			provider: &testProvider{Powerstate: "on"},
			secret:   createSecret(),
		},
		"success power off": {
			provider: &testProvider{Powerstate: "off"},
			secret:   createSecret(),
		},
		"success power on with RPC provider": {
			provider: &testProvider{Powerstate: "on", Proto: "rpc"},
			secret:   createHMACSecret(),
			machine:  createMachineWithRPC(createHMACSecret()),
		},
		"success power on with RPC provider w/o secrets": {
			provider: &testProvider{Powerstate: "on", Proto: "rpc"},
			secret:   createSecret(),
			machine:  createMachineWithRPC(nil),
		},
		"fail to find secret with RPC provider": {
			provider:  &testProvider{Powerstate: "on", Proto: "rpc"},
			secret:    createHMACSecret(),
			shouldErr: true,
			machine: createMachineWithRPC(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-bm-auths",
				},
				Data: map[string][]byte{
					"secret": []byte("test"),
				},
			}),
		},
		"success power on with a secret reference without a namespace": {
			provider: &testProvider{Powerstate: "on"},
			secret:   createSecret(),
			machine:  withoutSecretNamespaces(createMachine()),
		},
		"success power on with RPC provider and a secret reference without a namespace": {
			provider: &testProvider{Powerstate: "on", Proto: "rpc"},
			secret:   createHMACSecret(),
			machine:  withoutSecretNamespaces(createMachineWithRPC(createHMACSecret())),
		},
		"fail on open": {
			provider: &testProvider{ErrOpen: errors.New("failed to open connection")},
			secret:   createSecret(),
		},
		"fail on power get": {
			provider: &testProvider{ErrPowerStateGet: errors.New("failed to set power state")},
			secret:   createSecret(),
		},
		"fail bad power state": {
			provider: &testProvider{Powerstate: "bad"},
			secret:   createSecret(),
		},
		"fail on close": {
			provider: &testProvider{ErrClose: errors.New("failed to close connection")},
			secret:   createSecret(),
		},
		"fail secret not found": {
			provider:  &testProvider{Powerstate: "on"},
			shouldErr: true,
			secret:    &corev1.Secret{},
		},
		"fail secret username not found": {
			provider:  &testProvider{Powerstate: "on"},
			shouldErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-bm-auth",
				},
				Data: map[string][]byte{
					"password": []byte("test"),
				},
			},
		},
		"fail secret password not found": {
			provider:  &testProvider{Powerstate: "on"},
			shouldErr: true,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-bm-auth",
				},
				Data: map[string][]byte{
					"username": []byte("test"),
				},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var bm *bmc.Machine
			if tt.machine != nil {
				bm = tt.machine
			} else {
				bm = createMachine()
			}

			client := newClientBuilder().
				WithObjects(bm, tt.secret).
				Build()

			fakeRecorder := events.NewFakeRecorder(2)

			reconciler := controller.NewMachineReconciler(
				client,
				fakeRecorder,
				newTestClient(tt.provider),
				0,
				0,
				true,
			)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test-namespace",
					Name:      "test-bm",
				},
			}

			_, err := reconciler.Reconcile(context.Background(), req)
			if !tt.shouldErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.shouldErr && err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func createMachineWithRPC(secret *corev1.Secret) *bmc.Machine {
	machine := &bmc.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: bmc.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bm",
			Namespace: "test-namespace",
		},
		Spec: bmc.MachineSpec{
			Connection: bmc.Connection{
				Host:        "127.1.1.1",
				InsecureTLS: false,
				ProviderOptions: &bmc.ProviderOptions{
					RPC: &bmc.RPCOptions{
						ConsumerURL: "http://127.0.0.1:7777",
					},
				},
			},
		},
	}

	if secret != nil {
		machine.Spec.Connection.ProviderOptions.RPC.HMAC = &bmc.HMACOpts{
			Secrets: bmc.HMACSecrets{
				"sha256": []corev1.SecretReference{
					{
						Name:      secret.Name,
						Namespace: secret.Namespace,
					},
				},
			},
		}
	}

	return machine
}

func createMachine() *bmc.Machine {
	return &bmc.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: bmc.GroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bm",
			Namespace: "test-namespace",
		},
		Spec: bmc.MachineSpec{
			Connection: bmc.Connection{
				Host: "0.0.0.0",
				Port: 623,
				AuthSecretRef: corev1.SecretReference{
					Name:      "test-bm-auth",
					Namespace: "test-namespace",
				},
				InsecureTLS: false,
				ProviderOptions: &bmc.ProviderOptions{
					Redfish: &bmc.RedfishOptions{
						Port: 443,
					},
				},
			},
		},
	}
}

// withoutSecretNamespaces clears the namespace of every Secret reference in the Machine's Connection.
func withoutSecretNamespaces(machine *bmc.Machine) *bmc.Machine {
	clearSecretNamespaces(&machine.Spec.Connection)
	return machine
}

// clearSecretNamespaces clears the namespace of every Secret reference in conn.
func clearSecretNamespaces(conn *bmc.Connection) {
	conn.AuthSecretRef.Namespace = ""
	if conn.ProviderOptions != nil && conn.ProviderOptions.RPC != nil && conn.ProviderOptions.RPC.HMAC != nil {
		for _, refs := range conn.ProviderOptions.RPC.HMAC.Secrets {
			for i := range refs {
				refs[i].Namespace = ""
			}
		}
	}
}

func createSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-bm-auth",
		},
		Data: map[string][]byte{
			"username": []byte("test"),
			"password": []byte("test"),
		},
	}
}

func createHMACSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-bm-hmac",
		},
		Data: map[string][]byte{
			"secret": []byte("superSecret1"),
		},
	}
}

// newOfflineManager returns a manager whose REST mapper knows Machine and
// Hardware without discovery, so controllers can be set up without an API
// server as long as the manager is never started.
func newOfflineManager(t *testing.T) ctrl.Manager {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := bmc.AddToScheme(scheme); err != nil {
		t.Fatalf("bmc.AddToScheme() error = %v", err)
	}
	if err := tinkerbell.AddToScheme(scheme); err != nil {
		t.Fatalf("tinkerbell.AddToScheme() error = %v", err)
	}
	mgr, err := ctrl.NewManager(&rest.Config{Host: "https://127.0.0.1:1"}, ctrl.Options{
		Scheme: scheme,
		MapperProvider: func(*rest.Config, *http.Client) (meta.RESTMapper, error) {
			mapper := meta.NewDefaultRESTMapper(nil)
			mapper.Add(bmc.GroupVersion.WithKind("Machine"), meta.RESTScopeNamespace)
			mapper.Add(tinkerbell.GroupVersion.WithKind("Hardware"), meta.RESTScopeNamespace)
			return mapper, nil
		},
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	})
	if err != nil {
		t.Fatalf("ctrl.NewManager() error = %v", err)
	}
	return mgr
}

func TestMachineReconcilerSetupWithManager(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		inventoryCollectionEnabled bool
		wantHardwareIndex          bool
	}{
		"inventory collection enabled registers the Hardware bmcRef index": {inventoryCollectionEnabled: true, wantHardwareIndex: true},
		"inventory collection disabled skips the Hardware bmcRef index":    {inventoryCollectionEnabled: false, wantHardwareIndex: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mgr := newOfflineManager(t)
			r := controller.NewMachineReconciler(mgr.GetClient(), events.NewFakeRecorder(1), nil, 0, 0, tt.inventoryCollectionEnabled)

			if err := r.SetupWithManager(t.Context(), mgr, ctrlcontroller.Options{SkipNameValidation: new(true)}); err != nil {
				t.Fatalf("SetupWithManager() error = %v, want nil", err)
			}

			// Registering an index name a second time fails, so this reveals
			// whether SetupWithManager registered it.
			err := mgr.GetFieldIndexer().IndexField(t.Context(), &tinkerbell.Hardware{}, controller.HardwareBMCRefIndexKey, controller.HardwareBMCRefIndexFunc)
			if gotHardwareIndex := err != nil; gotHardwareIndex != tt.wantHardwareIndex {
				t.Errorf("Hardware bmcRef index registered = %v, want %v (re-register error: %v)", gotHardwareIndex, tt.wantHardwareIndex, err)
			}
		})
	}
}
