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

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
)

// Index key for Job Owner Name
const jobOwnerKey = ".metadata.controller"

// BMCJobReconciler reconciles a BMCJob object
type BMCJobReconciler struct {
	client client.Client
	logger logr.Logger
}

// NewBMCJobReconciler returns a new BMCJobReconciler
func NewBMCJobReconciler(client client.Client, logger logr.Logger) *BMCJobReconciler {
	return &BMCJobReconciler{
		client: client,
		logger: logger,
	}
}

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmcjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmcjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmcjobs/finalizers,verbs=update

// Reconcile runs a BMCJob.
// Creates the individual BMCTasks on the cluster.
// Watches for BMCTask and creates next BMCJob Task based on conditions.
func (r *BMCJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.WithValues("BMCJob", req.NamespacedName)
	logger.Info("Reconciling BMCJob")

	// Fetch the bmcJob object
	bmcJob := &bmcv1alpha1.BMCJob{}
	err := r.client.Get(ctx, req.NamespacedName, bmcJob)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get BMCJob")
		return ctrl.Result{}, err
	}

	// Deletion is a noop.
	if !bmcJob.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Job is Completed or Failed is noop.
	if bmcJob.HasCondition(bmcv1alpha1.JobCompleted, bmcv1alpha1.ConditionTrue) ||
		bmcJob.HasCondition(bmcv1alpha1.JobFailed, bmcv1alpha1.ConditionTrue) {
		return ctrl.Result{}, nil
	}

	// Create a patch from the initial BMCJob object
	// Patch is used to update Status after reconciliation
	bmcJobPatch := client.MergeFrom(bmcJob.DeepCopy())

	return r.reconcile(ctx, bmcJob, bmcJobPatch, logger)
}

func (r *BMCJobReconciler) reconcile(ctx context.Context, bmj *bmcv1alpha1.BMCJob, bmjPatch client.Patch, logger logr.Logger) (ctrl.Result, error) {
	// Check if Job is not currently Running
	// Initialize the StartTime for the Job
	// Set the Job to Running condition True
	if !bmj.HasCondition(bmcv1alpha1.JobRunning, bmcv1alpha1.ConditionTrue) {
		now := metav1.Now()
		bmj.Status.StartTime = &now
		bmj.SetCondition(bmcv1alpha1.JobRunning, bmcv1alpha1.ConditionTrue)
	}

	// Get BaseboardManagement object for the Job
	// Requeue if error
	baseboardManagement := &bmcv1alpha1.BaseboardManagement{}
	err := r.getBaseboardManagement(ctx, bmj.Spec.BaseboardManagementRef, baseboardManagement)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get BMCJob %s/%s BaseboardManagementRef: %v", bmj.Namespace, bmj.Name, err)
	}

	// List all BMCTask owned by BMCJob
	bmcTasks := &bmcv1alpha1.BMCTaskList{}
	err = r.client.List(ctx, bmcTasks, client.MatchingFields{jobOwnerKey: bmj.Name})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list owned BMCTasks for BMCJob %s/%s", bmj.Namespace, bmj.Name)
	}

	completedTasksCount := 0
	// Iterate BMCTask Items.
	// Count the number of completed tasks.
	// Set the Job condition Failed True if BMCTask has failed.
	// If the BMCTask has neither Completed or Failed is noop.
	for _, task := range bmcTasks.Items {
		if task.HasCondition(bmcv1alpha1.TaskCompleted, bmcv1alpha1.ConditionTrue) {
			completedTasksCount += 1
			continue
		}

		if task.HasCondition(bmcv1alpha1.TaskFailed, bmcv1alpha1.ConditionTrue) {
			err := fmt.Errorf("BMCTask %s/%s failed", task.Namespace, task.Name)
			bmj.SetCondition(bmcv1alpha1.JobFailed, bmcv1alpha1.ConditionTrue, bmcv1alpha1.WithJobConditionMessage(err.Error()))
			patchErr := r.patchStatus(ctx, bmj, bmjPatch)
			if patchErr != nil {
				return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
			}

			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Check if all BMCJob tasks have Completed
	// Set the Task CompletionTime
	// Set Task Condition Completed True
	if completedTasksCount == len(bmj.Spec.Tasks) {
		bmj.SetCondition(bmcv1alpha1.JobCompleted, bmcv1alpha1.ConditionTrue)
		now := metav1.Now()
		bmj.Status.CompletionTime = &now
		err = r.patchStatus(ctx, bmj, bmjPatch)
		return ctrl.Result{}, err
	}

	// Create the first Task for the BMCJob
	if err := r.createBMCTaskWithOwner(ctx, *bmj, completedTasksCount, baseboardManagement.Spec.Connection); err != nil {
		// Set the Job condition Failed True
		bmj.SetCondition(bmcv1alpha1.JobFailed, bmcv1alpha1.ConditionTrue, bmcv1alpha1.WithJobConditionMessage(err.Error()))
		patchErr := r.patchStatus(ctx, bmj, bmjPatch)
		if patchErr != nil {
			return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
		}

		return ctrl.Result{}, err
	}

	// Patch the status at the end of reconcile loop
	err = r.patchStatus(ctx, bmj, bmjPatch)
	return ctrl.Result{}, err
}

// getBaseboardManagement Gets the BaseboardManagement from BaseboardManagementRef
func (r *BMCJobReconciler) getBaseboardManagement(ctx context.Context, bmRef bmcv1alpha1.BaseboardManagementRef, bm *bmcv1alpha1.BaseboardManagement) error {
	key := types.NamespacedName{Namespace: bmRef.Namespace, Name: bmRef.Name}
	err := r.client.Get(ctx, key, bm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("BaseboardManagement %s not found: %v", key, err)
		}
		return fmt.Errorf("failed to get BaseboardManagement %s: %v", key, err)
	}

	return nil
}

// createBMCTaskWithOwner creates a BMCTask object with an OwnerReference set to the BMCJob
func (r *BMCJobReconciler) createBMCTaskWithOwner(ctx context.Context, bmj bmcv1alpha1.BMCJob, taskIndex int, conn bmcv1alpha1.Connection) error {
	isController := true
	bmcTask := &bmcv1alpha1.BMCTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bmcv1alpha1.FormatTaskName(bmj, taskIndex),
			Namespace: bmj.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: bmj.APIVersion,
					Kind:       bmj.Kind,
					Name:       bmj.Name,
					UID:        bmj.ObjectMeta.UID,
					Controller: &isController,
				},
			},
		},
		Spec: bmcv1alpha1.BMCTaskSpec{
			Task:       bmj.Spec.Tasks[taskIndex],
			Connection: conn,
		},
	}

	err := r.client.Create(ctx, bmcTask)
	if err != nil {
		return fmt.Errorf("failed to create BMCTask %s/%s: %v", bmcTask.Namespace, bmcTask.Name, err)
	}

	return nil
}

// patchStatus patches the specified patch on the BMCJob.
func (r *BMCJobReconciler) patchStatus(ctx context.Context, bmj *bmcv1alpha1.BMCJob, patch client.Patch) error {
	err := r.client.Status().Patch(ctx, bmj, patch)
	if err != nil {
		return fmt.Errorf("failed to patch BMCJob %s/%s status: %v", bmj.Namespace, bmj.Name, err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCJobReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&bmcv1alpha1.BMCTask{},
		jobOwnerKey,
		bmcTaskOwnerIndexFunc,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&bmcv1alpha1.BMCJob{}).
		Watches(
			&source.Kind{Type: &bmcv1alpha1.BMCTask{}},
			&handler.EnqueueRequestForOwner{
				OwnerType:    &bmcv1alpha1.BMCJob{},
				IsController: true,
			}).
		Complete(r)
}

// bmcTaskOwnerIndexFunc is Indexer func which returns the owner name for obj.
func bmcTaskOwnerIndexFunc(obj client.Object) []string {
	task, ok := obj.(*bmcv1alpha1.BMCTask)
	if !ok {
		return nil
	}

	owner := metav1.GetControllerOf(task)
	if owner == nil {
		return nil
	}

	// Check if owner is BMCJob
	if owner.Kind != "BMCJob" || owner.APIVersion != bmcv1alpha1.GroupVersion.String() {
		return nil
	}

	return []string{owner.Name}
}
