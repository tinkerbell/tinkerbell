package controller_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// A Task with the name of the Job's next Task already exists. The Job must
// only fail when that Task belongs to something other than this Job or a
// predecessor of the same name.
func TestJobReconcileTaskAlreadyExists(t *testing.T) {
	tests := map[string]struct {
		// cacheLags hides Tasks from the cached client, as a cache that
		// hasn't seen a Task created a moment ago would (tinkerbell/rufio#235).
		cacheLags   bool
		ownerKind   string
		ownerName   string
		ownerUID    types.UID
		deleting    bool
		completed   bool
		wantFailed  string
		wantDeleted bool
		// wantRequeueAfter is set when the Task's watch events don't
		// enqueue this Job, so it must check again by itself.
		wantRequeueAfter time.Duration
	}{
		"created by this job, not in the cache yet": {
			cacheLags: true,
			ownerUID:  "job-uid",
		},
		"being deleted, not in the cache yet": {
			cacheLags: true,
			ownerUID:  "old-job-uid",
			deleting:  true,
		},
		"being deleted, owned by a job of another name": {
			ownerName:        "other",
			ownerUID:         "other-uid",
			deleting:         true,
			wantRequeueAfter: time.Second,
		},
		"being deleted, owned by something other than a job": {
			ownerKind:        "Workflow",
			ownerUID:         "workflow-uid",
			deleting:         true,
			wantRequeueAfter: time.Second,
		},
		"left over from a deleted job of the same name": {
			ownerUID:    "old-job-uid",
			wantDeleted: true,
		},
		"left over and completed, from a deleted job of the same name": {
			ownerUID:    "old-job-uid",
			completed:   true,
			wantDeleted: true,
		},
		"owned by a job of another name": {
			ownerName:  "other",
			ownerUID:   "other-uid",
			wantFailed: "task default/boot-task-0 already exists and is owned by Job other (uid other-uid)",
		},
		"owned by something other than a job": {
			ownerKind:  "Workflow",
			ownerUID:   "workflow-uid",
			wantFailed: "task default/boot-task-0 already exists and is owned by Workflow boot (uid workflow-uid)",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			machine := createMachine()
			job := createJob("boot", machine, getAction("PowerOn"))
			job.UID = "job-uid"
			ownerKind, ownerName := "Job", job.Name
			if tt.ownerKind != "" {
				ownerKind = tt.ownerKind
			}
			if tt.ownerName != "" {
				ownerName = tt.ownerName
			}
			task := &bmc.Task{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bmc.FormatTaskName(*job, 0),
					Namespace: job.Namespace,
					UID:       "task-uid",
					Labels:    map[string]string{"owner-name": job.Name},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: bmc.GroupVersion.String(),
						Kind:       ownerKind,
						Name:       ownerName,
						UID:        tt.ownerUID,
						Controller: toPtr(true),
					}},
				},
				Spec: bmc.TaskSpec{Task: job.Spec.Tasks[0], Connection: machine.Spec.Connection},
			}
			if tt.deleting {
				task.Finalizers = []string{"test"}
				task.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			}
			if tt.completed {
				task.Status.Conditions = []bmc.TaskCondition{{Type: bmc.TaskCompleted, Status: bmc.ConditionTrue}}
			}

			base := newClientBuilder().
				WithObjects(job, machine, createSecret(), task).
				WithIndex(&bmc.Task{}, ".metadata.controller", controller.TaskOwnerIndexFunc).
				Build()
			var clnt client.Client = base
			if tt.cacheLags {
				clnt = interceptor.NewClient(base, interceptor.Funcs{
					List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
						if _, ok := list.(*bmc.TaskList); ok {
							return nil
						}
						return c.List(ctx, list, opts...)
					},
					Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*bmc.Task); ok {
							return apierrors.NewNotFound(bmc.GroupVersion.WithResource("tasks").GroupResource(), key.Name)
						}
						return c.Get(ctx, key, obj, opts...)
					},
				})
			}

			key := types.NamespacedName{Namespace: job.Namespace, Name: job.Name}
			result, err := controller.NewJobReconciler(clnt, base).Reconcile(context.Background(), reconcile.Request{NamespacedName: key})
			if (err != nil) != (tt.wantFailed != "") {
				t.Errorf("Reconcile error = %v, want error: %v", err, tt.wantFailed != "")
			}
			// The Job waits for the Task's own watch events (created, changed
			// or deleted) instead of polling for it, when they enqueue it.
			if result.RequeueAfter != tt.wantRequeueAfter || result.Requeue {
				t.Errorf("result = %+v, want RequeueAfter %v", result, tt.wantRequeueAfter)
			}

			got := &bmc.Job{}
			if err := base.Get(context.Background(), key, got); err != nil {
				t.Fatal(err)
			}
			if !got.HasCondition(bmc.JobRunning, bmc.ConditionTrue) {
				t.Errorf("Job is not Running: %+v", got.Status.Conditions)
			}
			if got.HasCondition(bmc.JobCompleted, bmc.ConditionTrue) {
				t.Errorf("Job is Completed without running its Task")
			}
			failed := ""
			for _, c := range got.Status.Conditions {
				if c.Type == bmc.JobFailed && c.Status == bmc.ConditionTrue {
					failed = c.Message
				}
			}
			if tt.wantFailed == "" && failed != "" {
				t.Errorf("Job failed: %s", failed)
			}
			if tt.wantFailed != "" && !strings.Contains(failed, tt.wantFailed) {
				t.Errorf("Job failure = %q, want it to contain %q", failed, tt.wantFailed)
			}

			err = base.Get(context.Background(), client.ObjectKeyFromObject(task), &bmc.Task{})
			if tt.wantDeleted && !apierrors.IsNotFound(err) {
				t.Errorf("leftover Task still exists (err %v)", err)
			}
			if !tt.wantDeleted && err != nil {
				t.Errorf("Task was deleted (err %v)", err)
			}
		})
	}
}
