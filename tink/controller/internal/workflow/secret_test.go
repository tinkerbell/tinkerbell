package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestRetrieveSecretData(t *testing.T) {
	tests := map[string]struct {
		secret          *corev1.Secret
		secretName      string
		secretNamespace string
		want            map[string]interface{}
		wantErr         bool
		errContains     string
	}{
		"successful retrieval with single key": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"password": []byte("secret123"),
				},
			},
			secretName:      "test-secret",
			secretNamespace: "default",
			want: map[string]interface{}{
				"password": "secret123",
			},
			wantErr: false,
		},
		"successful retrieval with multiple keys": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"db_password": []byte("dbpass123"),
					"api_token":   []byte("token456"),
					"username":    []byte("admin"),
				},
			},
			secretName:      "multi-secret",
			secretNamespace: "default",
			want: map[string]interface{}{
				"db_password": "dbpass123",
				"api_token":   "token456",
				"username":    "admin",
			},
			wantErr: false,
		},
		"empty secret data": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{},
			},
			secretName:      "empty-secret",
			secretNamespace: "default",
			want:            map[string]interface{}{},
			wantErr:         false,
		},
		"secret with binary data": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "binary-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"cert": []byte("-----BEGIN CERTIFICATE-----\nMIID...\n-----END CERTIFICATE-----"),
					"key":  []byte("-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----"),
				},
			},
			secretName:      "binary-secret",
			secretNamespace: "default",
			want: map[string]interface{}{
				"cert": "-----BEGIN CERTIFICATE-----\nMIID...\n-----END CERTIFICATE-----",
				"key":  "-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----",
			},
			wantErr: false,
		},
		"secret not found": {
			secret:          nil,
			secretName:      "nonexistent",
			secretNamespace: "default",
			want:            nil,
			wantErr:         true,
			errContains:     "secret not found",
		},
		"empty secret name": {
			secret:          nil,
			secretName:      "",
			secretNamespace: "default",
			want:            nil,
			wantErr:         true,
			errContains:     "secret name cannot be empty",
		},
		"secret in different namespace": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-secret",
					Namespace: "other-namespace",
				},
				Data: map[string][]byte{
					"data": []byte("value"),
				},
			},
			secretName:      "other-secret",
			secretNamespace: "other-namespace",
			want: map[string]interface{}{
				"data": "value",
			},
			wantErr: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// Create fake client with the secret if it exists
			builder := GetFakeClientBuilder()
			if tc.secret != nil {
				builder = builder.WithObjects(tc.secret)
			}
			fakeClient := builder.Build()

			got, err := retrieveSecretData(ctx, fakeClient, tc.secretName, tc.secretNamespace)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errContains)
				}
				if tc.errContains != "" && !contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error containing %q, got %q", tc.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("retrieveSecretData mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestProcessWorkflowWithSecrets(t *testing.T) {
	tests := map[string]struct {
		workflow      *v1alpha1.Workflow
		template      *v1alpha1.Template
		hardware      *v1alpha1.Hardware
		secret        *corev1.Secret
		wantErr       bool
		errContains   string
		checkRendered func(*testing.T, *v1alpha1.Workflow)
	}{
		"template with secret reference": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "secret-template",
					HardwareRef: "test-hardware",
					HardwareMap: map[string]string{
						"device_1": "00:00:00:00:00:01",
					},
				},
			},
			template: &v1alpha1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-template",
					Namespace: "default",
				},
				Spec: v1alpha1.TemplateSpec{
					Data: valueToPointer(`version: "0.1"
name: secret-test
global_timeout: 600
tasks:
  - name: "test-task"
    worker: "{{.device_1}}"
    actions:
      - name: "use-secret"
        image: alpine
        timeout: 60
        environment:
          PASSWORD: "{{.secret.password}}"
          TOKEN: "{{.secret.api_token}}"`),
					SecretRef: &v1alpha1.TemplateSecretReference{
						Name: "test-secret",
					},
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"password":  []byte("mypassword123"),
					"api_token": []byte("token456"),
				},
			},
			wantErr: false,
			checkRendered: func(t *testing.T, wf *v1alpha1.Workflow) {
				t.Helper()
				if wf.Status.TemplateRendering != v1alpha1.TemplateRenderingSuccessful {
					t.Errorf("expected TemplateRenderingSuccessful, got %v", wf.Status.TemplateRendering)
				}
				if len(wf.Status.Tasks) != 1 {
					t.Fatalf("expected 1 task, got %d", len(wf.Status.Tasks))
				}
				if len(wf.Status.Tasks[0].Actions) != 1 {
					t.Fatalf("expected 1 action, got %d", len(wf.Status.Tasks[0].Actions))
				}
				action := wf.Status.Tasks[0].Actions[0]
				if action.Environment["PASSWORD"] != "mypassword123" {
					t.Errorf("expected PASSWORD=mypassword123, got %v", action.Environment["PASSWORD"])
				}
				if action.Environment["TOKEN"] != "token456" {
					t.Errorf("expected TOKEN=token456, got %v", action.Environment["TOKEN"])
				}
			},
		},
		"template without secret reference": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "no-secret-template",
					HardwareRef: "test-hardware",
					HardwareMap: map[string]string{
						"device_1": "00:00:00:00:00:01",
					},
				},
			},
			template: &v1alpha1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-secret-template",
					Namespace: "default",
				},
				Spec: v1alpha1.TemplateSpec{
					Data: valueToPointer(`version: "0.1"
name: no-secret
global_timeout: 600
tasks:
  - name: "test-task"
    worker: "{{.device_1}}"
    actions:
      - name: "simple-action"
        image: alpine
        timeout: 60`),
					SecretRef: nil,
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
			},
			secret:  nil,
			wantErr: false,
			checkRendered: func(t *testing.T, wf *v1alpha1.Workflow) {
				t.Helper()
				if wf.Status.TemplateRendering != v1alpha1.TemplateRenderingSuccessful {
					t.Errorf("expected TemplateRenderingSuccessful, got %v", wf.Status.TemplateRendering)
				}
			},
		},
		"secret not found error": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "secret-template",
					HardwareRef: "test-hardware",
					HardwareMap: map[string]string{
						"device_1": "00:00:00:00:00:01",
					},
				},
			},
			template: &v1alpha1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-template",
					Namespace: "default",
				},
				Spec: v1alpha1.TemplateSpec{
					Data: valueToPointer(`version: "0.1"
name: secret-test
global_timeout: 600
tasks:
  - name: "test-task"
    worker: "{{.device_1}}"
    actions:
      - name: "use-secret"
        image: alpine
        timeout: 60`),
					SecretRef: &v1alpha1.TemplateSecretReference{
						Name: "missing-secret",
					},
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
			},
			secret:      nil,
			wantErr:     true,
			errContains: "secret not found",
			checkRendered: func(t *testing.T, wf *v1alpha1.Workflow) {
				t.Helper()
				if wf.Status.TemplateRendering != v1alpha1.TemplateRenderingFailed {
					t.Errorf("expected TemplateRenderingFailed, got %v", wf.Status.TemplateRendering)
				}
			},
		},
		"secret with special characters": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "special-secret-template",
					HardwareRef: "test-hardware",
					HardwareMap: map[string]string{
						"device_1": "00:00:00:00:00:01",
					},
				},
			},
			template: &v1alpha1.Template{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "special-secret-template",
					Namespace: "default",
				},
				Spec: v1alpha1.TemplateSpec{
					Data: valueToPointer(`version: "0.1"
name: special-test
global_timeout: 600
tasks:
  - name: "test-task"
    worker: "{{.device_1}}"
    actions:
      - name: "use-secret"
        image: alpine
        timeout: 60
        environment:
          KEY: "{{.secret.special_key}}"`),
					SecretRef: &v1alpha1.TemplateSecretReference{
						Name: "special-secret",
					},
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "special-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"special_key": []byte("value-with-dashes_and_underscores"),
				},
			},
			wantErr: false,
			checkRendered: func(t *testing.T, wf *v1alpha1.Workflow) {
				t.Helper()
				if wf.Status.TemplateRendering != v1alpha1.TemplateRenderingSuccessful {
					t.Errorf("expected TemplateRenderingSuccessful, got %v", wf.Status.TemplateRendering)
				}
				if len(wf.Status.Tasks) != 1 || len(wf.Status.Tasks[0].Actions) != 1 {
					t.Fatal("expected 1 task with 1 action")
				}
				action := wf.Status.Tasks[0].Actions[0]
				expected := "value-with-dashes_and_underscores"
				if action.Environment["KEY"] != expected {
					t.Errorf("expected KEY=%q, got %q", expected, action.Environment["KEY"])
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// Build fake client with all resources
			builder := GetFakeClientBuilder()
			if tc.workflow != nil {
				builder = builder.WithObjects(tc.workflow)
			}
			if tc.template != nil {
				builder = builder.WithObjects(tc.template)
			}
			if tc.hardware != nil {
				builder = builder.WithObjects(tc.hardware)
			}
			if tc.secret != nil {
				builder = builder.WithObjects(tc.secret)
			}
			fakeClient := builder.Build()

			reconciler := &Reconciler{
				client:        fakeClient,
				dynamicClient: &fakeDynamicClient{},
			}

			err := reconciler.processWorkflow(ctx, logr.Discard(), tc.workflow)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errContains)
				}
				if tc.errContains != "" && !contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error containing %q, got %q", tc.errContains, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.checkRendered != nil {
				tc.checkRendered(t, tc.workflow)
			}
		})
	}
}

func TestReconcileWithSecrets(t *testing.T) {
	tests := map[string]struct {
		objects       []client.Object
		wantErr       bool
		checkWorkflow func(*testing.T, *v1alpha1.Workflow)
	}{
		"NewWorkflow with secret": {
			objects: []client.Object{
				&v1alpha1.Workflow{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workflow",
						Namespace: "default",
					},
					Spec: v1alpha1.WorkflowSpec{
						TemplateRef: "secret-template",
						HardwareRef: "test-hardware",
						HardwareMap: map[string]string{
							"device_1": "00:00:00:00:00:01",
						},
					},
				},
				&v1alpha1.Template{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-template",
						Namespace: "default",
					},
					Spec: v1alpha1.TemplateSpec{
						Data: valueToPointer(minimalTemplate),
						SecretRef: &v1alpha1.TemplateSecretReference{
							Name: "test-secret",
						},
					},
				},
				&v1alpha1.Hardware{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hardware",
						Namespace: "default",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"password": []byte("secret123"),
					},
				},
			},
			wantErr: false,
			checkWorkflow: func(t *testing.T, wf *v1alpha1.Workflow) {
				t.Helper()
				if wf.Status.TemplateRendering != v1alpha1.TemplateRenderingSuccessful {
					t.Errorf("expected TemplateRenderingSuccessful, got %v", wf.Status.TemplateRendering)
				}
				if wf.Status.State != v1alpha1.WorkflowStatePending {
					t.Errorf("expected WorkflowStatePending, got %v", wf.Status.State)
				}
			},
		},
		"NewWorkflow with missing secret": {
			objects: []client.Object{
				&v1alpha1.Workflow{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workflow",
						Namespace: "default",
					},
					Spec: v1alpha1.WorkflowSpec{
						TemplateRef: "secret-template",
						HardwareRef: "test-hardware",
						HardwareMap: map[string]string{
							"device_1": "00:00:00:00:00:01",
						},
					},
				},
				&v1alpha1.Template{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-template",
						Namespace: "default",
					},
					Spec: v1alpha1.TemplateSpec{
						Data: valueToPointer(minimalTemplate),
						SecretRef: &v1alpha1.TemplateSecretReference{
							Name: "missing-secret",
						},
					},
				},
				&v1alpha1.Hardware{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hardware",
						Namespace: "default",
					},
				},
			},
			wantErr: true,
			checkWorkflow: func(t *testing.T, wf *v1alpha1.Workflow) {
				t.Helper()
				if wf.Status.TemplateRendering != v1alpha1.TemplateRenderingFailed {
					t.Errorf("expected TemplateRenderingFailed, got %v", wf.Status.TemplateRendering)
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// Build client with status subresource support
			builder := GetFakeClientBuilder().WithObjects(tc.objects...)
			workflow := tc.objects[0].(*v1alpha1.Workflow)
			builder = builder.WithStatusSubresource(workflow)
			fakeClient := builder.Build()

			reconciler := &Reconciler{
				client:        fakeClient,
				dynamicClient: &fakeDynamicClient{},
				nowFunc:       TestTime.Now,
			}

			req := reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(workflow),
			}

			_, err := reconciler.Reconcile(ctx, req)

			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Get updated workflow
			updatedWorkflow := &v1alpha1.Workflow{}
			if err := fakeClient.Get(ctx, client.ObjectKeyFromObject(workflow), updatedWorkflow); err != nil {
				if !kerrors.IsNotFound(err) {
					t.Fatalf("failed to get workflow: %v", err)
				}
			}

			if tc.checkWorkflow != nil && !kerrors.IsNotFound(err) {
				tc.checkWorkflow(t, updatedWorkflow)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
