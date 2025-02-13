package controller_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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
			job:     createJob("test", createMachine()),
		},
		"failure unknown machine": {
			machine: &bmc.Machine{},
			secret:  createSecret(),
			job:     createJob("test", createMachine()), shouldErr: true,
		},
		"success power on job": {
			machine: createMachine(),
			secret:  createSecret(),
			job:     createJob("test", createMachine(), getAction("PowerOn")),
			testAll: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			clnt := newClientBuilder().
				WithObjects(tt.job, tt.machine, tt.secret).
				WithStatusSubresource(tt.job, tt.machine).
				WithIndex(&bmc.Task{}, ".metadata.controller", controller.TaskOwnerIndexFunc).
				Build()

			reconciler := controller.NewJobReconciler(clnt)

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

func createJob(name string, machine *bmc.Machine, t ...bmc.Action) *bmc.Job {
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
			Name:      name,
		},
		Spec: bmc.JobSpec{
			MachineRef: bmc.MachineRef{Name: machine.Name, Namespace: machine.Namespace},
			Tasks:      tasks,
		},
	}
}
