package workflow

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// conflictingClient wraps a fake client to simulate conflict errors on update.
type conflictingClient struct {
	client.Client
	conflictCount int
	maxConflicts  int
}

func (c *conflictingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.conflictCount < c.maxConflicts {
		c.conflictCount++
		return apierrors.NewConflict(
			schema.GroupResource{Group: "tinkerbell.org", Resource: "hardware"},
			obj.GetName(),
			nil,
		)
	}
	return c.Client.Update(ctx, obj, opts...)
}

func TestSetAllowPXE(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = api.AddToSchemeTinkerbell(scheme)

	tests := map[string]struct {
		workflow         *v1alpha1.Workflow
		hardware         *v1alpha1.Hardware
		existingObjects  []client.Object
		allowPXE         bool
		expectError      bool
		expectedHardware *v1alpha1.Hardware
	}{
		"both workflow and hardware nil": {
			workflow:    nil,
			hardware:    nil,
			allowPXE:    true,
			expectError: true,
		},
		"hardware provided, set allowPXE to true": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			},
			allowPXE:    true,
			expectError: false,
			expectedHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
		},
		"hardware provided, set allowPXE to false": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
			allowPXE:    false,
			expectError: false,
			expectedHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			},
		},
		"hardware nil, fetch from cluster and set allowPXE": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
			},
			hardware: nil,
			existingObjects: []client.Object{
				&v1alpha1.Hardware{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hardware",
						Namespace: "default",
					},
					Spec: v1alpha1.HardwareSpec{
						Interfaces: []v1alpha1.Interface{
							{
								Netboot: &v1alpha1.Netboot{
									AllowPXE: valueToPointer(false),
								},
							},
						},
					},
				},
			},
			allowPXE:    true,
			expectError: false,
			expectedHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
		},
		"hardware nil, hardware not found in cluster": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "missing-hardware",
				},
			},
			hardware:        nil,
			existingObjects: []client.Object{},
			allowPXE:        true,
			expectError:     true,
		},
		"empty interfaces slice": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{},
				},
			},
			allowPXE:    true,
			expectError: false,
			expectedHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: nil, // After processing, empty slice becomes nil
				},
			},
		},
		"interface with nil Netboot field": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: nil, // This should be handled gracefully now
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			},
			allowPXE:    true,
			expectError: false,
			expectedHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: nil, // Should remain nil
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true), // Should be updated
							},
						},
					},
				},
			},
		},
		"hardware fetched from cluster with multiple interfaces": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
			},
			hardware: nil,
			existingObjects: []client.Object{
				&v1alpha1.Hardware{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hardware",
						Namespace: "default",
					},
					Spec: v1alpha1.HardwareSpec{
						Interfaces: []v1alpha1.Interface{
							{
								Netboot: &v1alpha1.Netboot{
									AllowPXE: valueToPointer(false),
								},
							},
							{
								Netboot: &v1alpha1.Netboot{
									AllowPXE: valueToPointer(true),
								},
							},
							{
								Netboot: &v1alpha1.Netboot{
									AllowPXE: valueToPointer(false),
								},
							},
						},
					},
				},
			},
			allowPXE:    true,
			expectError: false,
			expectedHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
		},
		"hardware with mixed allowPXE values, set to false": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
			allowPXE:    false,
			expectError: false,
			expectedHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			},
		},
		"workflow with different namespace than hardware": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "workflow-namespace",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
			},
			hardware: nil,
			existingObjects: []client.Object{
				&v1alpha1.Hardware{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-hardware",
						Namespace: "workflow-namespace", // Must match workflow namespace
					},
					Spec: v1alpha1.HardwareSpec{
						Interfaces: []v1alpha1.Interface{
							{
								Netboot: &v1alpha1.Netboot{
									AllowPXE: valueToPointer(false),
								},
							},
						},
					},
				},
			},
			allowPXE:    true,
			expectError: false,
			expectedHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "workflow-namespace",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create fake client with existing objects
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			if tc.existingObjects != nil {
				clientBuilder = clientBuilder.WithObjects(tc.existingObjects...)
			}
			if tc.hardware != nil {
				clientBuilder = clientBuilder.WithObjects(tc.hardware)
			}
			fakeClient := clientBuilder.Build()

			// Call the function
			err := setAllowPXE(context.Background(), fakeClient, tc.workflow, tc.hardware, tc.allowPXE)

			// Check error expectation
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify the hardware was updated correctly
			if tc.expectedHardware != nil {
				actualHardware := &v1alpha1.Hardware{}
				err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(tc.expectedHardware), actualHardware)
				if err != nil {
					t.Fatalf("failed to get updated hardware: %v", err)
				}

				// Compare the hardware specs (ignore metadata changes like resource version)
				if diff := cmp.Diff(tc.expectedHardware.Spec, actualHardware.Spec); diff != "" {
					t.Errorf("unexpected hardware spec diff (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestSetAllowPXE_RetryMechanism(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = api.AddToSchemeTinkerbell(scheme)

	tests := map[string]struct {
		maxConflicts    int
		expectError     bool
		expectedRetries int
	}{
		"success after 1 conflict": {
			maxConflicts:    1,
			expectError:     false,
			expectedRetries: 1,
		},
		"success after 2 conflicts": {
			maxConflicts:    2,
			expectError:     false,
			expectedRetries: 2,
		},
		"success after 3 conflicts": {
			maxConflicts:    3,
			expectError:     false,
			expectedRetries: 3,
		},
		"failure after 4 conflicts (exceeds retry limit)": {
			maxConflicts:    4,
			expectError:     true,
			expectedRetries: 4,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create hardware object
			hardware := &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			}

			// Create workflow
			workflow := &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
			}

			// Create base client
			baseClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(hardware).
				Build()

			// Wrap with conflicting client
			conflictClient := &conflictingClient{
				Client:       baseClient,
				maxConflicts: tc.maxConflicts,
			}

			// Call the function
			err := setAllowPXE(context.Background(), conflictClient, workflow, nil, true)

			// Check error expectation
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				// Verify it's a conflict-related error
				if conflictClient.conflictCount != tc.expectedRetries {
					t.Errorf("expected %d conflicts, got %d", tc.expectedRetries, conflictClient.conflictCount)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify the retry count
			if conflictClient.conflictCount != tc.expectedRetries {
				t.Errorf("expected %d retries, got %d", tc.expectedRetries, conflictClient.conflictCount)
			}

			// Verify the hardware was eventually updated
			updatedHardware := &v1alpha1.Hardware{}
			err = baseClient.Get(context.Background(), client.ObjectKeyFromObject(hardware), updatedHardware)
			if err != nil {
				t.Fatalf("failed to get updated hardware: %v", err)
			}

			// Check that allowPXE was set correctly
			if len(updatedHardware.Spec.Interfaces) > 0 &&
				updatedHardware.Spec.Interfaces[0].Netboot != nil &&
				updatedHardware.Spec.Interfaces[0].Netboot.AllowPXE != nil {
				if !*updatedHardware.Spec.Interfaces[0].Netboot.AllowPXE {
					t.Error("expected allowPXE to be true after successful retry")
				}
			} else {
				t.Error("hardware interface netboot configuration is missing")
			}
		})
	}
}
