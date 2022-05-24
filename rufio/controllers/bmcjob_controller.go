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
	"strconv"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
)

// BMCJobReconciler reconciles a BMCJob object
type BMCJobReconciler struct {
	client           client.Client
	bmcClientFactory BMCClientFactoryFunc
	bmcTaskHandler   BMCTaskHandler
	logger           logr.Logger
}

// NewBMCJobReconciler returns a new BMCJobReconciler
func NewBMCJobReconciler(client client.Client, bmcClientFactory BMCClientFactoryFunc, bmcTaskHandler BMCTaskHandler, logger logr.Logger) *BMCJobReconciler {
	return &BMCJobReconciler{
		client:           client,
		bmcClientFactory: bmcClientFactory,
		bmcTaskHandler:   bmcTaskHandler,
		logger:           logger,
	}
}

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmcjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmcjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmcjobs/finalizers,verbs=update

// Reconcile runs a BMCJob.
// Creates the individual BMCTasks on the cluster.
// Runs each Task in a BMCTask and updates the BMCJob status.
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

	// Job has StartTime is noop.
	if !bmcJob.Status.StartTime.IsZero() {
		return ctrl.Result{}, nil
	}
	// Create a patch from the initial BMCJob object
	// Patch is used to update Status after reconciliation
	bmcJobPatch := client.MergeFrom(bmcJob.DeepCopy())

	return r.reconcile(ctx, bmcJob, bmcJobPatch, logger)
}

func (r *BMCJobReconciler) reconcile(ctx context.Context, bmj *bmcv1alpha1.BMCJob, bmjPatch client.Patch, logger logr.Logger) (ctrl.Result, error) {
	// Get BaseboardManagement object for the Job
	// Requeue if error
	baseboardManagement := &bmcv1alpha1.BaseboardManagement{}
	err := r.resolveBaseboardManagementRef(ctx, bmj, baseboardManagement)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("resolving BMCJob %s/%s BaseboardManagementRef: %v", bmj.Namespace, bmj.Name, err)
	}

	// Fetching username, password from SecretReference
	// Requeue if error fetching secret
	username, password, err := resolveAuthSecretRef(ctx, r.client, baseboardManagement.Spec.Connection.AuthSecretRef)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("resolving BaseboardManagement %s/%s SecretReference: %v", baseboardManagement.Namespace, baseboardManagement.Name, err)
	}

	// Initializing BMC Client
	bmcClient, err := r.bmcClientFactory(ctx, baseboardManagement.Spec.Connection.Host, strconv.Itoa(baseboardManagement.Spec.Connection.Port), username, password)
	if err != nil {
		logger.Error(err, "BMC connection failed", "host", baseboardManagement.Spec.Connection.Host)
		bmj.SetCondition(bmcv1alpha1.JobFailed, bmcv1alpha1.BMCJobConditionTrue, bmcv1alpha1.WithJobConditionMessage(fmt.Sprintf("Failed to connect to BMC: %v", err)))
		result, patchErr := r.patchBMCJobStatus(ctx, bmj, bmjPatch)
		if patchErr != nil {
			return result, utilerrors.NewAggregate([]error{patchErr, err})
		}

		return result, err
	}

	// Close BMC connection after reconcilation
	defer func() {
		err = bmcClient.Close(ctx)
		if err != nil {
			logger.Error(err, "BMC close connection failed", "host", baseboardManagement.Spec.Connection.Host)
		}
	}()

	return r.reconcileBMCTasks(ctx, bmj, bmjPatch, bmcClient, logger)
}

// reconcileBMCTasks Creates the individual BMCTasks
// Runs each of the BMCTasks
func (r *BMCJobReconciler) reconcileBMCTasks(ctx context.Context, bmj *bmcv1alpha1.BMCJob, bmjPatch client.Patch, bmcClient BMCClient, logger logr.Logger) (ctrl.Result, error) {
	// Initialize the StartTime for the Job
	now := metav1.Now()
	bmj.Status.StartTime = &now
	// Set the Job to Running
	bmj.SetCondition(bmcv1alpha1.JobRunning, bmcv1alpha1.BMCJobConditionTrue)
	if result, err := r.patchBMCJobStatus(ctx, bmj, bmjPatch); err != nil {
		return result, err
	}

	// aggErr aggregates errors
	var aggErr utilerrors.Aggregate
	for i, task := range bmj.Spec.Tasks {
		bmcTask := &bmcv1alpha1.BMCTask{}
		err := r.bmcTaskHandler.CreateBMCTaskWithOwner(ctx, r.client, task, i, bmcTask, bmj)
		if err != nil {
			logger.Error(err, "failed to reconcile BMCJob Tasks")
			bmj.SetCondition(bmcv1alpha1.JobFailed, bmcv1alpha1.BMCJobConditionTrue, bmcv1alpha1.WithJobConditionMessage(fmt.Sprintf("Failed to reconcile BMCJob Tasks: %v", err)))
			result, patchErr := r.patchBMCJobStatus(ctx, bmj, bmjPatch)
			if patchErr != nil {
				return result, utilerrors.NewAggregate([]error{patchErr, err})
			}

			return result, err
		}
		// Create patch from initial BMCTask object
		bmcTaskPatch := client.MergeFrom(bmcTask.DeepCopy())
		err = r.bmcTaskHandler.RunBMCTask(ctx, bmcTask, bmcClient)
		if err != nil {
			logger.Error(err, "failed to run BMCJob Task", "Name", bmcTask.Name, "Namespace", bmcTask.Namespace)
			bmj.SetCondition(bmcv1alpha1.JobFailed, bmcv1alpha1.BMCJobConditionTrue, bmcv1alpha1.WithJobConditionMessage(fmt.Sprintf("Failed to run BMCJob Task %s/%s: %v", bmcTask.Namespace, bmcTask.Name, err)))
			// Patch the BMCTask status
			if taskPatchError := r.bmcTaskHandler.PatchBMCTaskStatus(ctx, r.client, bmcTask, bmcTaskPatch); taskPatchError != nil {
				aggErr = utilerrors.NewAggregate([]error{taskPatchError, aggErr})
			}
			// Patch the BMCJob status
			result, jobPatchErr := r.patchBMCJobStatus(ctx, bmj, bmjPatch)
			if jobPatchErr != nil {
				aggErr = utilerrors.NewAggregate([]error{jobPatchErr, aggErr})
			}

			return result, utilerrors.Flatten(utilerrors.NewAggregate([]error{err, aggErr}))
		}
		// Patch BMCTask status
		if taskPatchError := r.bmcTaskHandler.PatchBMCTaskStatus(ctx, r.client, bmcTask, bmcTaskPatch); taskPatchError != nil {
			// Aggregate the errors if the Task status patch failed
			// Continue execution of other tasks
			aggErr = utilerrors.NewAggregate([]error{taskPatchError, aggErr})
		}
	}

	now = metav1.Now()
	bmj.Status.CompletionTime = &now
	bmj.SetCondition(bmcv1alpha1.JobCompleted, bmcv1alpha1.BMCJobConditionTrue)

	result, patchErr := r.patchBMCJobStatus(ctx, bmj, bmjPatch)
	if patchErr != nil {
		return result, utilerrors.Flatten(utilerrors.NewAggregate([]error{patchErr, aggErr}))
	}

	return result, utilerrors.Flatten(aggErr)
}

// resolveBaseboardManagementRef Gets the BaseboardManagement from BaseboardManagementRef
func (r *BMCJobReconciler) resolveBaseboardManagementRef(ctx context.Context, bmj *bmcv1alpha1.BMCJob, bm *bmcv1alpha1.BaseboardManagement) error {
	key := types.NamespacedName{Namespace: bmj.Spec.BaseboardManagementRef.Namespace, Name: bmj.Spec.BaseboardManagementRef.Name}
	err := r.client.Get(ctx, key, bm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("BaseboardManagement %s not found: %v", key, err)
		}
		return fmt.Errorf("failed to get BaseboardManagement %s: %v", key, err)
	}

	return nil
}

// patchBMCJobStatus patches the specified patch on the BMCJob.
func (r *BMCJobReconciler) patchBMCJobStatus(ctx context.Context, bmj *bmcv1alpha1.BMCJob, patch client.Patch) (ctrl.Result, error) {
	err := r.client.Status().Patch(ctx, bmj, patch)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch BMCJob %s/%s status: %v", bmj.Namespace, bmj.Name, err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bmcv1alpha1.BMCJob{}).
		Owns(&bmcv1alpha1.BMCTask{}).
		Complete(r)
}
