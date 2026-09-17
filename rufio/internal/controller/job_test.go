package controller_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// staleList returns interceptor.Funcs that make List a no-op, leaving the
// caller's list empty. Used to simulate an informer cache that hasn't yet
// observed a Task a previous reconcile already created, while Get and Create
// against the same client still see it - the actual condition that lets
// createTaskWithOwner's Create call race with an existing Task.
func staleList() interceptor.Funcs {
	return interceptor.Funcs{
		List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
			return nil
		},
	}
}

func TestJobReconcile(t *testing.T) {
	tests := map[string]struct {
		machine   *bmc.Machine
		secret    *corev1.Secret
		job       *bmc.Job
		shouldErr bool
		testAll   bool
	}{
		"success taskless job": {
			machine: createMachine(),
			secret:  createSecret(),
			job:     createJob(createMachine()),
		},
		"failure unknown machine": {
			machine: &bmc.Machine{},
			secret:  createSecret(),
			job:     createJob(createMachine()), shouldErr: true,
		},
		"success power on job": {
			machine: createMachine(),
			secret:  createSecret(),
			job:     createJob(createMachine(), getAction("PowerOn")),
			testAll: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			clnt := newClientBuilder().
				WithObjects(tt.job, tt.machine, tt.secret).
				WithIndex(&bmc.Task{}, ".metadata.controller", controller.TaskOwnerIndexFunc).
				Build()

			reconciler := controller.NewJobReconciler(clnt, clnt)

			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: tt.job.Namespace,
					Name:      tt.job.Name,
				},
			}

			_, err := reconciler.Reconcile(context.Background(), request)
			if !tt.shouldErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tt.shouldErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.shouldErr || !tt.testAll {
				return
			}
			var retrieved1 bmc.Job
			if err = clnt.Get(context.Background(), request.NamespacedName, &retrieved1); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			// TODO: g.Expect(retrieved1.Status.StartTime.Unix()).To(gomega.BeNumerically("~", time.Now().Unix(), 10))
			if !retrieved1.Status.CompletionTime.IsZero() {
				t.Fatalf("expected CompletionTime to be zero, got %v", retrieved1.Status.CompletionTime)
			}
			if len(retrieved1.Status.Conditions) != 1 {
				t.Fatalf("expected 1 condition, got %v", len(retrieved1.Status.Conditions))
			}
			if retrieved1.Status.Conditions[0].Type != bmc.JobRunning {
				t.Fatalf("expected condition type %v, got %v", bmc.JobRunning, retrieved1.Status.Conditions[0].Type)
			}
			if retrieved1.Status.Conditions[0].Status != bmc.ConditionTrue {
				t.Fatalf("expected condition status %v, got %v", bmc.ConditionTrue, retrieved1.Status.Conditions[0].Status)
			}

			var task bmc.Task
			taskKey := types.NamespacedName{
				Namespace: tt.job.Namespace,
				Name:      bmc.FormatTaskName(*tt.job, 0),
			}
			if err = clnt.Get(context.Background(), taskKey, &task); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if diff := cmp.Diff(task.Spec.Task, tt.job.Spec.Tasks[0]); diff != "" {
				t.Fatalf("expected task %v, got %v", tt.job.Spec.Tasks[0], task.Spec.Task)
			}
			if len(task.OwnerReferences) != 1 {
				t.Fatalf("expected 1 owner reference, got %v", len(task.OwnerReferences))
			}
			if task.OwnerReferences[0].Name != tt.job.Name {
				t.Fatalf("expected owner reference name %v, got %v", tt.job.Name, task.OwnerReferences[0].Name)
			}
			if task.OwnerReferences[0].Kind != "Job" {
				t.Fatalf("expected OwnerReferences[0].Kind = 'Job', got '%v'", task.OwnerReferences[0].Kind)
			}

			// Ensure re-reconciling a job does nothing given the task is still outstanding.
			result, err := reconciler.Reconcile(context.Background(), request)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if diff := cmp.Diff(result, reconcile.Result{}); diff != "" {
				t.Fatal(diff)
			}

			var retrieved2 bmc.Job
			if err = clnt.Get(context.Background(), request.NamespacedName, &retrieved2); err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if diff := cmp.Diff(retrieved1, retrieved2); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func createJob(machine *bmc.Machine, t ...bmc.Action) *bmc.Job {
	tasks := []bmc.Action{}
	if len(t) > 0 {
		tasks = t
	}
	return &bmc.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: bmc.GroupVersion.String(),
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: bmc.JobSpec{
			MachineRef: bmc.MachineRef{Name: machine.Name, Namespace: machine.Namespace},
			Tasks:      tasks,
		},
	}
}

// TestJobReconcileTaskAlreadyExists reproduces a Job reconcile racing with
// itself: a previous reconcile already created the Task, correctly owned by
// this Job, but the List used to inventory owned Tasks is stale (e.g. an
// informer cache that hasn't observed the Create yet) and misses it, so
// reconcile proceeds to (re)create it. The reconciler must recognize the
// existing Task as this Job's own and treat that as the desired state rather
// than an error.
//
// Seen in production while 27 Workflows were synced at once - two Jobs failed
// with "failed to create Task .../<job>-task-0: tasks.bmc.tinkerbell.org
// "<job>-task-0" already exists", marking the Workflow FAILED even though the
// Task existed and its action had completed successfully.
func TestJobReconcileTaskAlreadyExists(t *testing.T) {
	machine := createMachine()
	secret := createSecret()
	job := createJob(createMachine(), getAction("PowerOn"))
	job.UID = "job-uid"

	isController := true
	// The Task a previous reconcile of this same Job already created.
	existing := &bmc.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bmc.FormatTaskName(*job, 0),
			Namespace: job.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: job.APIVersion,
					Kind:       job.Kind,
					Name:       job.Name,
					UID:        job.UID,
					Controller: &isController,
				},
			},
		},
		Spec: bmc.TaskSpec{Task: job.Spec.Tasks[0]},
	}

	base := newClientBuilder().
		WithObjects(job, machine, secret, existing).
		WithIndex(&bmc.Task{}, ".metadata.controller", controller.TaskOwnerIndexFunc).
		Build()
	clnt := interceptor.NewClient(base, staleList())

	// apiReader reads directly against base, bypassing the stale List seen
	// by the cache client - modeling mgr.GetAPIReader() talking straight to
	// the API server instead of the informer cache.
	reconciler := controller.NewJobReconciler(clnt, base)

	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: job.Namespace, Name: job.Name},
	})
	if err != nil {
		t.Fatalf("reconcile must be idempotent when the Task already exists and is owned by this Job, got: %v", err)
	}

	// The Job must not be marked Failed for an already-existing, correctly owned Task.
	var got bmc.Job
	if err := clnt.Get(context.Background(),
		types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, &got); err != nil {
		t.Fatalf("getting job: %v", err)
	}
	for _, c := range got.Status.Conditions {
		if c.Type == bmc.JobFailed && c.Status == bmc.ConditionTrue {
			t.Fatalf("Job marked Failed for an already-existing, correctly owned Task: %s", c.Message)
		}
	}
}

// TestJobReconcileTaskAlreadyExistsForeignOwner ensures the AlreadyExists
// guard exercised by TestJobReconcileTaskAlreadyExists does not paper over a
// genuine conflict: a Task with the name this Job would create, but owned by
// a different Job instance (e.g. a previous Job with the same name whose
// Task wasn't garbage collected yet). Silently treating that as success would
// leave this Job's own Task never created, and nothing would ever re-enqueue
// it since neither the owner watch nor the name-keyed Task index recognize
// the foreign Task as unrelated.
func TestJobReconcileTaskAlreadyExistsForeignOwner(t *testing.T) {
	machine := createMachine()
	secret := createSecret()
	job := createJob(createMachine(), getAction("PowerOn"))
	job.UID = "current-job-uid"

	isController := true
	// A Task with the name this Job would create, owned by a different Job UID.
	foreign := &bmc.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bmc.FormatTaskName(*job, 0),
			Namespace: job.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: job.APIVersion,
					Kind:       job.Kind,
					Name:       job.Name,
					UID:        "previous-job-uid",
					Controller: &isController,
				},
			},
		},
		Spec: bmc.TaskSpec{Task: job.Spec.Tasks[0]},
	}

	base := newClientBuilder().
		WithObjects(job, machine, secret, foreign).
		WithIndex(&bmc.Task{}, ".metadata.controller", controller.TaskOwnerIndexFunc).
		Build()
	clnt := interceptor.NewClient(base, staleList())

	reconciler := controller.NewJobReconciler(clnt, base)

	_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: job.Namespace, Name: job.Name},
	})
	if err == nil {
		t.Fatal("expected an error when the existing Task is owned by a different Job, got nil")
	}

	var got bmc.Job
	if err := clnt.Get(context.Background(),
		types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, &got); err != nil {
		t.Fatalf("getting job: %v", err)
	}
	failed := false
	for _, c := range got.Status.Conditions {
		if c.Type == bmc.JobFailed && c.Status == bmc.ConditionTrue {
			failed = true
		}
	}
	if !failed {
		t.Fatal("expected Job to be marked Failed when the existing Task belongs to a different owner")
	}
}
