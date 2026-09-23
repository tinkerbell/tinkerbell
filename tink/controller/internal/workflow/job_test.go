package workflow

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
		"existing job deleted, new job already created and completed": {
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
		"no status entry yet": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					BootOptions: v1alpha1.BootOptionsStatus{
						Jobs:         map[string]v1alpha1.JobStatus{},
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

		"create new job": {
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
						Reason:  reasonCreated,
						Message: messageJobCreated,
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
			actions: []bmc.Action{
				{PowerAction: toPtr(bmc.PowerStatus)},
			},
			name:       jobNameNetboot,
			wantResult: reconcile.Result{Requeue: true},
		},
		"existing job running, track status": {
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
						Reason:  reasonComplete,
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
			bmc.AddToScheme(scheme)
			v1alpha1.AddToScheme(scheme)
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&v1alpha1.Hardware{}, &v1alpha1.Template{}, &v1alpha1.Workflow{}, &v1alpha1.WorkflowRuleSet{}, &bmc.Job{}, &bmc.Machine{}, &bmc.Task{}).WithRuntimeObjects(tc.hardware, tc.workflow)
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

// TestHandleJobFreshWorkflowKeepsItsOwnJob drives a fresh Workflow (no status entry for the Job
// yet) through consecutive reconciles and asserts the Job handleJob creates is never deleted.
// Gating cleanup on the status entry existing used to skip it on the first reconcile, create
// the Job, record its UID on the second, then delete that same Job on the third and create a
// replacement - leaving the first Job's Tasks (already started by rufio) orphaned and colliding
// by name with the replacement Job's Tasks.
func TestHandleJobFreshWorkflowKeepsItsOwnJob(t *testing.T) {
	scheme := runtime.NewScheme()
	bmc.AddToScheme(scheme)
	v1alpha1.AddToScheme(scheme)

	hw := &v1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hardware", Namespace: "default"},
		Spec: v1alpha1.HardwareSpec{
			BMCRef: &v1.TypedLocalObjectReference{Name: "test-bmc", Kind: "machine.bmc.tinkerbell.org"},
		},
	}
	wf := &v1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workflow", Namespace: "default"},
		Spec:       v1alpha1.WorkflowSpec{HardwareRef: "test-hardware"},
		Status: v1alpha1.WorkflowStatus{
			BootOptions: v1alpha1.BootOptionsStatus{Jobs: map[string]v1alpha1.JobStatus{}},
		},
	}

	jobDeletes := 0
	base := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.Workflow{}, &bmc.Job{}, &bmc.Task{}).
		WithRuntimeObjects(hw, wf).
		Build()
	clnt := interceptor.NewClient(base, interceptor.Funcs{
		// The fake client doesn't assign UIDs; handleJob relies on them to tell Jobs apart.
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if obj.GetUID() == "" {
				obj.SetUID(types.UID(obj.GetName() + "-uid"))
			}
			return c.Create(ctx, obj, opts...)
		},
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			err := c.Delete(ctx, obj, opts...)
			if _, ok := obj.(*bmc.Job); ok && err == nil {
				jobDeletes++
			}
			return err
		},
	})

	s := &state{workflow: wf, client: clnt}
	ctx := context.Background()
	actions := []bmc.Action{{PowerAction: toPtr(bmc.PowerHardOff)}}

	// Cleanup, creation, UID recording. Without the fix, the third reconcile is the one that
	// deletes the Job created by the first.
	for i := range 3 {
		if _, err := s.handleJob(ctx, actions, jobNameNetboot); err != nil {
			t.Fatalf("reconcile %d: unexpected error: %v", i+1, err)
		}
	}

	if jobDeletes != 0 {
		t.Fatalf("handleJob deleted %d Job(s) while setting up a fresh Workflow; it must never delete the Job it created", jobDeletes)
	}

	status := s.workflow.Status.BootOptions.Jobs[jobNameNetboot.String()]
	if status.UID == "" {
		t.Fatal("expected the created Job's UID to be recorded in the Workflow status")
	}
	job := &bmc.Job{}
	if err := clnt.Get(ctx, client.ObjectKey{Name: jobNameNetboot.String(), Namespace: "default"}, job); err != nil {
		t.Fatalf("getting job: %v", err)
	}
	if job.UID != status.UID {
		t.Fatalf("Workflow status tracks Job UID %q, but the live Job's UID is %q", status.UID, job.UID)
	}
}

func toPtr[T any](v T) *T {
	return &v
}
