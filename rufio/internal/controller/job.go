/*
Copyright 2022 Tinkerbell.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// Index key for Job Owner UID.
const jobOwnerKey = ".metadata.controller"

// JobReconciler reconciles a Job object.
type JobReconciler struct {
	client client.Client
	// reader reads from the API server without the cache, for objects whose cached copy may be stale.
	reader client.Reader
}

// NewJobReconciler returns a new JobReconciler.
func NewJobReconciler(c client.Client, reader client.Reader) *JobReconciler {
	return &JobReconciler{
		client: c,
		reader: reader,
	}
}

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=jobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=jobs/finalizers,verbs=update

// Reconcile runs a Job.
// Creates the individual Tasks on the cluster.
// Watches for Task and creates next Job Task based on conditions.
func (r *JobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithName("controllers/Job").WithValues("job", req.NamespacedName)
	logger.Info("Reconciling Job")

	// Fetch the job object
	job := &bmc.Job{}
	err := r.client.Get(ctx, req.NamespacedName, job)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get Job")
		return ctrl.Result{}, err
	}

	// Deletion is a noop.
	if !job.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Job is Completed or Failed is noop.
	if job.HasCondition(bmc.JobCompleted, bmc.ConditionTrue) ||
		job.HasCondition(bmc.JobFailed, bmc.ConditionTrue) {
		return ctrl.Result{}, nil
	}

	// Create a patch from the initial Job object
	// Patch is used to update Status after reconciliation
	jobPatch := client.MergeFrom(job.DeepCopy())

	return r.doReconcile(ctx, job, jobPatch)
}

func (r *JobReconciler) doReconcile(ctx context.Context, job *bmc.Job, jobPatch client.Patch) (ctrl.Result, error) {
	// Check if Job is not currently Running
	// Initialize the StartTime for the Job
	// Set the Job to Running condition True
	if !job.HasCondition(bmc.JobRunning, bmc.ConditionTrue) {
		now := metav1.Now()
		job.Status.StartTime = &now
		job.SetCondition(bmc.JobRunning, bmc.ConditionTrue)
	}

	// Get Machine object for the Job
	// Requeue if error
	machine := &bmc.Machine{}
	err := r.getMachine(ctx, job.Spec.MachineRef, machine)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get Job %s/%s MachineRef: %w", job.Namespace, job.Name, err)
	}

	// List all Task owned by Job. Match the owner's UID, not its name: Tasks left by a deleted Job
	// of the same name belong to that Job, not to this one.
	tasks := &bmc.TaskList{}
	err = r.client.List(ctx, tasks, client.MatchingFields{jobOwnerKey: string(job.UID)}, client.InNamespace(job.Namespace))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list owned Tasks for Job %s/%s: %w", job.Namespace, job.Name, err)
	}

	completedTasksCount := 0
	// Iterate Task Items.
	// Count the number of completed tasks.
	// Set the Job condition Failed True if Task has failed.
	// If the Task has neither Completed or Failed is noop.
	for _, task := range tasks.Items {
		if task.HasCondition(bmc.TaskCompleted, bmc.ConditionTrue) {
			completedTasksCount++
			continue
		}

		if task.HasCondition(bmc.TaskFailed, bmc.ConditionTrue) {
			err := fmt.Errorf("task %s/%s failed", task.Namespace, task.Name)
			job.SetCondition(bmc.JobFailed, bmc.ConditionTrue, bmc.WithJobConditionMessage(err.Error()))
			patchErr := r.patchStatus(ctx, job, jobPatch)
			if patchErr != nil {
				return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
			}

			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Check if all Job tasks have Completed
	// Set the Task CompletionTime
	// Set Task Condition Completed True
	if completedTasksCount == len(job.Spec.Tasks) {
		job.SetCondition(bmc.JobCompleted, bmc.ConditionTrue)
		now := metav1.Now()
		job.Status.CompletionTime = &now
		err = r.patchStatus(ctx, job, jobPatch)
		return ctrl.Result{}, err
	}

	// Create the first Task for the Job
	if err := r.createTaskWithOwner(ctx, *job, completedTasksCount, machine.Spec.Connection); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return r.handleExistingTask(ctx, job, jobPatch, completedTasksCount)
		}
		// Set the Job condition Failed True
		job.SetCondition(bmc.JobFailed, bmc.ConditionTrue, bmc.WithJobConditionMessage(err.Error()))
		patchErr := r.patchStatus(ctx, job, jobPatch)
		if patchErr != nil {
			return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
		}

		return ctrl.Result{}, err
	}

	// Patch the status at the end of reconcile loop
	err = r.patchStatus(ctx, job, jobPatch)
	return ctrl.Result{}, err
}

// handleExistingTask handles a Task that could not be created because one with its name already exists.
// Task names are deterministic, so this happens when the cache doesn't show yet a Task that this Job
// created on an earlier pass, or when a deleted Job of the same name left its Tasks behind. The Task is
// read without the cache. The Job waits for the Task's watch events, which enqueue it, if the Task is this
// Job's, or is being deleted and owned by a Job of this name; it checks again later for any other Task
// being deleted. A Task left by a deleted Job of the same name is deleted. Only a live Task owned by
// something else fails the Job.
func (r *JobReconciler) handleExistingTask(ctx context.Context, job *bmc.Job, jobPatch client.Patch, taskIndex int) (ctrl.Result, error) {
	key := types.NamespacedName{Namespace: job.Namespace, Name: bmc.FormatTaskName(*job, taskIndex)}
	task := &bmc.Task{}
	err := r.reader.Get(ctx, key, task)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get existing Task %s: %w", key, err)
	}
	if apierrors.IsNotFound(err) {
		// Deleted since the create failed: try again.
		if err := r.patchStatus(ctx, job, jobPatch); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	ref := metav1.GetControllerOf(task)
	ownedByJobOfThisName := ref != nil && ref.Kind == "Job" && ref.APIVersion == bmc.GroupVersion.String() && ref.Name == job.Name
	switch {
	case ownedByJobOfThisName && (ref.UID == job.UID || !task.DeletionTimestamp.IsZero()):
		// Wait for the Task's watch events: they enqueue the Job its controller reference names, this one.
	case !task.DeletionTimestamp.IsZero():
		// The Task's watch events enqueue another Job, or none, so check again once it may be gone.
		if err := r.patchStatus(ctx, job, jobPatch); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	case ownedByJobOfThisName:
		// This Job holds the name now, so the Job that owned the Task is gone and the Task is an orphan.
		// Nothing deletes it with the embedded kube-apiserver, which has no garbage collector.
		opts := []client.DeleteOption{
			client.PropagationPolicy(metav1.DeletePropagationBackground),
			client.Preconditions{UID: &task.UID},
		}
		if err := r.client.Delete(ctx, task, opts...); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete Task %s left by a deleted Job: %w", key, err)
		}
	default:
		owner := "no controller"
		if ref != nil {
			owner = fmt.Sprintf("%s %s (uid %s)", ref.Kind, ref.Name, ref.UID)
		}
		err := fmt.Errorf("task %s already exists and is owned by %s", key, owner)
		job.SetCondition(bmc.JobFailed, bmc.ConditionTrue, bmc.WithJobConditionMessage(err.Error()))
		if patchErr := r.patchStatus(ctx, job, jobPatch); patchErr != nil {
			return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
		}
		return ctrl.Result{}, err
	}

	err = r.patchStatus(ctx, job, jobPatch)
	return ctrl.Result{}, err
}

// getMachine Gets the Machine from MachineRef.
func (r *JobReconciler) getMachine(ctx context.Context, reference bmc.MachineRef, machine *bmc.Machine) error {
	key := types.NamespacedName{Namespace: reference.Namespace, Name: reference.Name}
	err := r.client.Get(ctx, key, machine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("machine %s not found: %w", key, err)
		}
		return fmt.Errorf("failed to get Machine %s: %w", key, err)
	}

	return nil
}

// createTaskWithOwner creates a Task object with an OwnerReference set to the Job.
func (r *JobReconciler) createTaskWithOwner(ctx context.Context, job bmc.Job, taskIndex int, conn bmc.Connection) error {
	isController := true
	task := &bmc.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bmc.FormatTaskName(job, taskIndex),
			Namespace: job.Namespace,
			Labels: map[string]string{
				"owner-name": job.Name,
			},
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
		Spec: bmc.TaskSpec{
			Task:       job.Spec.Tasks[taskIndex],
			Connection: conn,
		},
	}

	err := r.client.Create(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to create Task %s/%s: %w", task.Namespace, task.Name, err)
	}

	return nil
}

// patchStatus patches the specified patch on the Job.
func (r *JobReconciler) patchStatus(ctx context.Context, job *bmc.Job, patch client.Patch) error {
	err := r.client.Status().Patch(ctx, job, patch)
	if err != nil {
		return fmt.Errorf("failed to patch Job %s/%s status: %w", job.Namespace, job.Name, err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *JobReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, opts ctrlcontroller.Options) error {
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&bmc.Task{},
		jobOwnerKey,
		TaskOwnerIndexFunc,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(opts).
		For(&bmc.Job{}).
		Watches(
			&bmc.Task{},
			handler.EnqueueRequestForOwner(
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&bmc.Job{},
				handler.OnlyControllerOwner(),
			),
		).
		Complete(r)
}

// TaskOwnerIndexFunc is Indexer func which returns the owner UID for obj.
func TaskOwnerIndexFunc(obj client.Object) []string {
	task, ok := obj.(*bmc.Task)
	if !ok {
		return nil
	}

	owner := metav1.GetControllerOf(task)
	if owner == nil {
		return nil
	}

	// Check if owner is Job
	if owner.Kind != "Job" || owner.APIVersion != bmc.GroupVersion.String() {
		return nil
	}

	return []string{string(owner.UID)}
}
