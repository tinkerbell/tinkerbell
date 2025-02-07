package workflow

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	rufio "github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestHandleJob(t *testing.T) {
	tests := map[string]struct {
		workflow     *tinkerbell.Workflow
		wantWorkflow *tinkerbell.WorkflowStatus
		hardware     *tinkerbell.Hardware
		actions      []rufio.Action
		name         jobName
		wantError    bool
		wantResult   reconcile.Result
		job          *rufio.Job
	}{
		"existing job deleted, new job created and completed": {
			workflow: &tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					BootOptions: tinkerbell.BootOptionsStatus{
						Jobs: map[string]tinkerbell.JobStatus{
							jobNameNetboot.String(): {
								ExistingJobDeleted: true,
								UID:                types.UID("1234"),
								Complete:           true,
							},
						},
						AllowNetboot: tinkerbell.AllowNetbootStatus{},
					},
				},
			},
			wantWorkflow: &tinkerbell.WorkflowStatus{
				BootOptions: tinkerbell.BootOptionsStatus{
					Jobs: map[string]tinkerbell.JobStatus{
						jobNameNetboot.String(): {
							ExistingJobDeleted: true,
							UID:                types.UID("1234"),
							Complete:           true,
						},
					},
					AllowNetboot: tinkerbell.AllowNetbootStatus{},
				},
			},
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
			name:       jobNameNetboot,
			wantResult: reconcile.Result{Requeue: true},
		},
		"existing job not deleted": {
			workflow: &tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					BootOptions: tinkerbell.BootOptionsStatus{
						Jobs: map[string]tinkerbell.JobStatus{
							jobNameNetboot.String(): {},
						},
						AllowNetboot: tinkerbell.AllowNetbootStatus{},
					},
				},
			},
			wantWorkflow: &tinkerbell.WorkflowStatus{
				BootOptions: tinkerbell.BootOptionsStatus{
					Jobs: map[string]tinkerbell.JobStatus{
						jobNameNetboot.String(): {
							ExistingJobDeleted: true,
						},
					},
					AllowNetboot: tinkerbell.AllowNetbootStatus{},
				},
			},
			name:       jobNameNetboot,
			hardware:   new(tinkerbell.Hardware),
			wantResult: reconcile.Result{Requeue: true},
		},
		"existing job deleted, create new job": {
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					HardwareRef: "test-hardware",
				},
				Status: tinkerbell.WorkflowStatus{
					BootOptions: tinkerbell.BootOptionsStatus{
						Jobs: map[string]tinkerbell.JobStatus{
							jobNameNetboot.String(): {
								ExistingJobDeleted: true,
							},
						},
						AllowNetboot: tinkerbell.AllowNetbootStatus{},
					},
				},
			},
			wantWorkflow: &tinkerbell.WorkflowStatus{
				Conditions: []tinkerbell.WorkflowCondition{
					{
						Type:    tinkerbell.NetbootJobSetupComplete,
						Status:  metav1.ConditionTrue,
						Reason:  "Created",
						Message: "job created",
					},
				},
				BootOptions: tinkerbell.BootOptionsStatus{
					Jobs: map[string]tinkerbell.JobStatus{
						jobNameNetboot.String(): {
							ExistingJobDeleted: true,
						},
					},
					AllowNetboot: tinkerbell.AllowNetbootStatus{},
				},
			},
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
			actions:    []rufio.Action{},
			name:       jobNameNetboot,
			wantResult: reconcile.Result{Requeue: true},
		},
		"existing job deleted, new job created": {
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Status: tinkerbell.WorkflowStatus{
					BootOptions: tinkerbell.BootOptionsStatus{
						Jobs: map[string]tinkerbell.JobStatus{
							jobNameNetboot.String(): {
								ExistingJobDeleted: true,
								UID:                types.UID("1234"),
							},
						},
						AllowNetboot: tinkerbell.AllowNetbootStatus{},
					},
				},
			},
			wantWorkflow: &tinkerbell.WorkflowStatus{
				Conditions: []tinkerbell.WorkflowCondition{
					{
						Type:    tinkerbell.NetbootJobComplete,
						Status:  metav1.ConditionTrue,
						Reason:  "Complete",
						Message: "job completed",
					},
				},
				BootOptions: tinkerbell.BootOptionsStatus{
					Jobs: map[string]tinkerbell.JobStatus{
						jobNameNetboot.String(): {
							ExistingJobDeleted: true,
							UID:                types.UID("1234"),
							Complete:           true,
						},
					},
					AllowNetboot: tinkerbell.AllowNetbootStatus{},
				},
			},
			hardware:   new(tinkerbell.Hardware),
			actions:    []rufio.Action{},
			name:       jobNameNetboot,
			wantResult: reconcile.Result{},
			job: &rufio.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobNameNetboot.String(),
					Namespace: "default",
					UID:       types.UID("1234"),
				},
				Status: rufio.JobStatus{
					Conditions: []rufio.JobCondition{
						{
							Type:   rufio.JobCompleted,
							Status: rufio.ConditionTrue,
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
			if diff := cmp.Diff(*tc.wantWorkflow, s.workflow.Status, cmpopts.IgnoreFields(tinkerbell.WorkflowCondition{}, "Time")); diff != "" {
				t.Errorf("unexpected workflow status (-want +got):\n%s", diff)
			}
		})
	}
}
