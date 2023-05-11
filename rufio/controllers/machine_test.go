package controllers_test

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/rufio/controllers"
)

func TestMachineReconcile(t *testing.T) {
	tests := map[string]struct {
		provider  *testProvider
		shouldErr bool
		secret    *corev1.Secret
	}{
		"success power on":      {provider: &testProvider{Powerstate: "on"}, secret: createSecret()},
		"success power off":     {provider: &testProvider{Powerstate: "off"}, secret: createSecret()},
		"fail on open":          {provider: &testProvider{ErrOpen: errors.New("failed to open connection")}, shouldErr: true, secret: createSecret()},
		"fail on power get":     {provider: &testProvider{ErrPowerStateGet: errors.New("failed to set power state")}, shouldErr: true, secret: createSecret()},
		"fail bad power state":  {provider: &testProvider{Powerstate: "bad"}, shouldErr: true, secret: createSecret()},
		"fail on close":         {provider: &testProvider{ErrClose: errors.New("failed to close connection")}, shouldErr: true, secret: createSecret()},
		"fail secret not found": {provider: &testProvider{Powerstate: "on"}, shouldErr: true, secret: &corev1.Secret{}},
		"fail secret username not found": {provider: &testProvider{Powerstate: "on"}, shouldErr: true, secret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test-namespace",
				Name:      "test-bm-auth",
			},
			Data: map[string][]byte{
				"password": []byte("test"),
			},
		}},
		"fail secret password not found": {provider: &testProvider{Powerstate: "on"}, shouldErr: true, secret: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test-namespace",
				Name:      "test-bm-auth",
			},
			Data: map[string][]byte{
				"username": []byte("test"),
			},
		}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = v1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)

			clientBuilder := fake.NewClientBuilder()
			bm := createMachine()
			objs := []runtime.Object{bm, tt.secret}
			client := clientBuilder.WithScheme(scheme).WithRuntimeObjects(objs...).Build()
			fakeRecorder := record.NewFakeRecorder(2)

			reconciler := controllers.NewMachineReconciler(
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
