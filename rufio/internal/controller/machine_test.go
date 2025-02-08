package controller_test

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/bmc/v1alpha1"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
)

func TestMachineReconcile(t *testing.T) {
	tests := map[string]struct {
		provider  *testProvider
		shouldErr bool
		secret    *corev1.Secret
		machine   *v1alpha1.Machine
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
			var bm *v1alpha1.Machine
			if tt.machine != nil {
				bm = tt.machine
			} else {
				bm = createMachine()
			}

			client := newClientBuilder().
				WithObjects(bm, tt.secret).
				WithStatusSubresource(bm).
				Build()

			fakeRecorder := record.NewFakeRecorder(2)

			reconciler := controller.NewMachineReconciler(
				client,
				fakeRecorder,
				newTestClient(tt.provider),
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

func createMachineWithRPC(secret *corev1.Secret) *v1alpha1.Machine {
	return &v1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bm",
			Namespace: "test-namespace",
		},
		Spec: v1alpha1.MachineSpec{
			Connection: v1alpha1.Connection{
				Host:        "127.1.1.1",
				InsecureTLS: false,
				ProviderOptions: &v1alpha1.ProviderOptions{
					RPC: &v1alpha1.RPCOptions{
						ConsumerURL: "http://127.0.0.1:7777",
						HMAC: &v1alpha1.HMACOpts{
							Secrets: v1alpha1.HMACSecrets{
								"sha256": []corev1.SecretReference{
									{
										Name:      secret.Name,
										Namespace: secret.Namespace,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func createMachine() *v1alpha1.Machine {
	return &v1alpha1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bm",
			Namespace: "test-namespace",
		},
		Spec: v1alpha1.MachineSpec{
			Connection: v1alpha1.Connection{
				Host: "0.0.0.0",
				Port: 623,
				AuthSecretRef: corev1.SecretReference{
					Name:      "test-bm-auth",
					Namespace: "test-namespace",
				},
				InsecureTLS: false,
				ProviderOptions: &v1alpha1.ProviderOptions{
					Redfish: &v1alpha1.RedfishOptions{
						Port: 443,
					},
				},
			},
		},
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
