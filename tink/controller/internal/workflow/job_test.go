package workflow

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell/bmc"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestHandleJob(t *testing.T) {
	tests := map[string]struct {
		workflow     *v1alpha1.Workflow
		wantWorkflow *v1alpha1.WorkflowStatus
		hardware     *v1alpha1.Hardware
		actions      []bmc.Action
		name         jobName
		wantError    bool
		wantResult   reconcile.Result
		job          *bmc.Job
	}{
		"existing job deleted, new job created and completed": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							jobNameNetboot.String(): {
								ExistingJobDeleted: true,
								UID:                types.UID("1234"),
								Complete:           true,
							},
						},
						AllowNetboot: v1alpha1.AllowNetbootStatus{},
					},
				},
			},
			wantWorkflow: &v1alpha1.WorkflowStatus{
				BootOptions: v1alpha1.BootOptionsStatus{
					Jobs: map[string]v1alpha1.JobStatus{
						jobNameNetboot.String(): {
							ExistingJobDeleted: true,
							UID:                types.UID("1234"),
							Complete:           true,
						},
					},
					AllowNetboot: v1alpha1.AllowNetbootStatus{},
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			name:       jobNameNetboot,
			wantResult: reconcile.Result{Requeue: true},
		},
		"existing job not deleted": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							jobNameNetboot.String(): {},
						},
						AllowNetboot: v1alpha1.AllowNetbootStatus{},
					},
				},
			},
			wantWorkflow: &v1alpha1.WorkflowStatus{
				BootOptions: v1alpha1.BootOptionsStatus{
					Jobs: map[string]v1alpha1.JobStatus{
						jobNameNetboot.String(): {
							ExistingJobDeleted: true,
						},
					},
					AllowNetboot: v1alpha1.AllowNetbootStatus{},
				},
			},
			name:       jobNameNetboot,
			hardware:   new(v1alpha1.Hardware),
			wantResult: reconcile.Result{Requeue: true},
		},
		"existing job deleted, create new job": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
				Status: v1alpha1.WorkflowStatus{
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							jobNameNetboot.String(): {
								ExistingJobDeleted: true,
							},
						},
						AllowNetboot: v1alpha1.AllowNetbootStatus{},
					},
				},
			},
			wantWorkflow: &v1alpha1.WorkflowStatus{
				Conditions: []v1alpha1.WorkflowCondition{
					{
						Type:    v1alpha1.BootJobSetupComplete,
						Status:  metav1.ConditionTrue,
						Reason:  "Created",
						Message: "job created",
					},
				},
				BootOptions: v1alpha1.BootOptionsStatus{
					Jobs: map[string]v1alpha1.JobStatus{
						jobNameNetboot.String(): {
							ExistingJobDeleted: true,
						},
					},
					AllowNetboot: v1alpha1.AllowNetbootStatus{},
				},
			},
			hardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hardware",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					BMCRef: &v1.TypedLocalObjectReference{
						Name: "test-bmc",
						Kind: "machine.bmc.tinkerbell.org",
					},
				},
			},
			actions:    []bmc.Action{},
			name:       jobNameNetboot,
			wantResult: reconcile.Result{Requeue: true},
		},
		"existing job deleted, new job created": {
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Status: v1alpha1.WorkflowStatus{
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs: map[string]v1alpha1.JobStatus{
							jobNameNetboot.String(): {
								ExistingJobDeleted: true,
								UID:                types.UID("1234"),
							},
						},
						AllowNetboot: v1alpha1.AllowNetbootStatus{},
					},
				},
			},
			wantWorkflow: &v1alpha1.WorkflowStatus{
				Conditions: []v1alpha1.WorkflowCondition{
					{
						Type:    v1alpha1.BootJobComplete,
						Status:  metav1.ConditionTrue,
						Reason:  "Complete",
						Message: "job completed",
					},
				},
				BootOptions: v1alpha1.BootOptionsStatus{
					Jobs: map[string]v1alpha1.JobStatus{
						jobNameNetboot.String(): {
							ExistingJobDeleted: true,
							UID:                types.UID("1234"),
							Complete:           true,
						},
					},
					AllowNetboot: v1alpha1.AllowNetbootStatus{},
				},
			},
			hardware:   new(v1alpha1.Hardware),
			actions:    []bmc.Action{},
			name:       jobNameNetboot,
			wantResult: reconcile.Result{},
			job: &bmc.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobNameNetboot.String(),
					Namespace: "default",
					UID:       types.UID("1234"),
				},
				Status: bmc.JobStatus{
					Conditions: []bmc.JobCondition{
						{
							Type:   bmc.JobCompleted,
							Status: bmc.ConditionTrue,
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			api.AddToSchemeBMC(scheme)
			api.AddToSchemeTinkerbell(scheme)
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tc.hardware, tc.workflow)
			if tc.job != nil {
				clientBuilder.WithRuntimeObjects(tc.job)
			}
			s := &state{
				workflow: tc.workflow,
				client:   clientBuilder.Build(),
			}
			ctx := context.Background()
			r, err := s.handleJob(ctx, tc.actions, tc.name)
			if (err != nil) != tc.wantError {
				t.Errorf("expected error: %v, got: %v", tc.wantError, err)
			}
			if diff := cmp.Diff(tc.wantResult, r); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(*tc.wantWorkflow, s.workflow.Status, cmpopts.IgnoreFields(v1alpha1.WorkflowCondition{}, "Time")); diff != "" {
				t.Errorf("unexpected workflow status (-want +got):\n%s", diff)
			}
		})
	}
}
