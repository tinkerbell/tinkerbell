package workflow

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/tink/controller/internal/workflow/journal"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestPrepareWorkflow(t *testing.T) {
	tests := map[string]struct {
		wantResult   reconcile.Result
		wantError    bool
		hardware     *tinkerbell.Hardware
		wantHardware *tinkerbell.Hardware
		workflow     *tinkerbell.Workflow
		wantWorkflow *tinkerbell.Workflow
		job          *rufio.Job
	}{
		"nothing to do": {
			wantResult:   reconcile.Result{},
			hardware:     &tinkerbell.Hardware{},
			wantHardware: &tinkerbell.Hardware{},
			workflow:     &tinkerbell.Workflow{},
			wantWorkflow: &tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					State: tinkerbell.WorkflowStatePending,
				},
			},
		},
		"toggle allowPXE": {
			wantResult: reconcile.Result{},
			hardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							Netboot: &tinkerbell.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			},
			wantHardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							Netboot: &tinkerbell.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					HardwareRef: "test-hardware",
					BootOptions: tinkerbell.BootOptions{
						ToggleAllowNetboot: true,
					},
				},
			},
			wantWorkflow: &tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					State: tinkerbell.WorkflowStatePending,
					BootOptions: tinkerbell.BootOptionsStatus{
						AllowNetboot: tinkerbell.AllowNetbootStatus{
							ToggledTrue: true,
						},
					},
					Conditions: []tinkerbell.WorkflowCondition{
						{
							Type:    tinkerbell.ToggleAllowNetbootTrue,
							Status:  metav1.ConditionTrue,
							Reason:  "Complete",
							Message: "set allowPXE to true",
						},
					},
				},
			},
		},
		"boot mode netboot": {
			wantResult: reconcile.Result{Requeue: true},
			hardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			wantHardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					HardwareRef: "test-hardware",
					BootOptions: tinkerbell.BootOptions{
						BootMode: "netboot",
					},
				},
				Status: tinkerbell.WorkflowStatus{
					BootOptions: tinkerbell.BootOptionsStatus{
						Jobs: map[string]tinkerbell.JobStatus{},
					},
				},
			},
			wantWorkflow: &tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					BootOptions: tinkerbell.BootOptionsStatus{
						Jobs: map[string]tinkerbell.JobStatus{
							fmt.Sprintf("%s-test-workflow", jobNameNetboot): {ExistingJobDeleted: true},
						},
					},
				},
			},
		},
		"boot mode iso": {
			wantResult: reconcile.Result{Requeue: true},
			hardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			wantHardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					HardwareRef: "test-hardware",
					BootOptions: tinkerbell.BootOptions{
						BootMode: "iso",
						ISOURL:   "http://example.com",
					},
				},
				Status: tinkerbell.WorkflowStatus{
					BootOptions: tinkerbell.BootOptionsStatus{
						Jobs: map[string]tinkerbell.JobStatus{},
					},
				},
			},
			wantWorkflow: &tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					BootOptions: tinkerbell.BootOptionsStatus{
						Jobs: map[string]tinkerbell.JobStatus{
							fmt.Sprintf("%s-test-workflow", jobNameISOMount): {ExistingJobDeleted: true},
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			rufio.AddToScheme(scheme)
			tinkerbell.AddToScheme(scheme)
			ro := []runtime.Object{}
			if tc.hardware != nil {
				ro = append(ro, tc.hardware)
			}
			if tc.workflow != nil {
				ro = append(ro, tc.workflow)
			}
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(ro...)
			if tc.job != nil {
				clientBuilder.WithRuntimeObjects(tc.job)
			}
			s := &state{
				workflow: tc.workflow,
				client:   clientBuilder.Build(),
			}
			ctx := context.Background()
			ctx = journal.New(ctx)
			result, err := s.prepareWorkflow(ctx)
			if (err != nil) != tc.wantError {
				t.Errorf("expected error: %v, got: %v", tc.wantError, err)
			}
			if diff := cmp.Diff(result, tc.wantResult); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
				t.Logf("journal: %v", journal.Journal(ctx))
			}

			// get the Hardware object in cluster
			gotHardware := &tinkerbell.Hardware{}
			if err := s.client.Get(ctx, types.NamespacedName{Name: tc.hardware.Name, Namespace: tc.hardware.Namespace}, gotHardware); err != nil {
				t.Fatalf("error getting hardware: %v", err)
			}
			if diff := cmp.Diff(gotHardware.Spec, tc.wantHardware.Spec); diff != "" {
				t.Errorf("unexpected hardware (-want +got):\n%s", diff)
				for _, entry := range journal.Journal(ctx) {
					t.Logf("journal: %+v", entry)
				}
			}

			if diff := cmp.Diff(tc.workflow.Status, tc.wantWorkflow.Status, cmpopts.IgnoreFields(tinkerbell.WorkflowCondition{}, "Time")); diff != "" {
				t.Errorf("unexpected workflow status (-want +got):\n%s", diff)
				for _, entry := range journal.Journal(ctx) {
					t.Logf("journal: %+v", entry)
				}
			}
		})
	}
}
