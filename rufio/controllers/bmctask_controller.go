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
	"strings"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
)

// BMCTaskReconciler reconciles a BMCTask object
type BMCTaskReconciler struct {
	client           client.Client
	bmcClientFactory BMCClientFactoryFunc
	logger           logr.Logger
}

// NewBMCTaskReconciler returns a new BMCTaskReconciler
func NewBMCTaskReconciler(client client.Client, bmcClientFactory BMCClientFactoryFunc, logger logr.Logger) *BMCTaskReconciler {
	return &BMCTaskReconciler{
		client:           client,
		bmcClientFactory: bmcClientFactory,
		logger:           logger,
	}
}

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmctasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmctasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmctasks/finalizers,verbs=update

// Reconcile runs a BMCTask.
// Establishes a connection to the BMC.
// Runs the specified action in the Task.
func (r *BMCTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.WithValues("BMCTask", req.NamespacedName)
	logger.Info("Reconciling BMCTask")

	// Fetch the BMCTask object
	bmcTask := &bmcv1alpha1.BMCTask{}
	err := r.client.Get(ctx, req.NamespacedName, bmcTask)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get BMCTask")
		return ctrl.Result{}, err
	}

	// Deletion is a noop.
	if !bmcTask.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Task is Completed or Failed is noop.
	if bmcTask.HasCondition(bmcv1alpha1.TaskFailed, bmcv1alpha1.ConditionTrue) ||
		bmcTask.HasCondition(bmcv1alpha1.TaskCompleted, bmcv1alpha1.ConditionTrue) {
		return ctrl.Result{}, nil
	}

	// Create a patch from the initial BMCTask object
	// Patch is used to update Status after reconciliation
	bmcTaskPatch := client.MergeFrom(bmcTask.DeepCopy())

	return r.reconcile(ctx, bmcTask, bmcTaskPatch, logger)
}

func (r *BMCTaskReconciler) reconcile(ctx context.Context, bmcTask *bmcv1alpha1.BMCTask, bmcTaskPatch client.Patch, logger logr.Logger) (ctrl.Result, error) {
	// Fetching username, password from SecretReference in Connection.
	// Requeue if error fetching secret
	username, password, err := resolveAuthSecretRef(ctx, r.client, bmcTask.Spec.Connection.AuthSecretRef)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("resolving Connection SecretReference for BMCTask %s/%s: %v", bmcTask.Namespace, bmcTask.Name, err)
	}

	// Initializing BMC Client
	bmcClient, err := r.bmcClientFactory(ctx, bmcTask.Spec.Connection.Host, strconv.Itoa(bmcTask.Spec.Connection.Port), username, password)
	if err != nil {
		logger.Error(err, "BMC connection failed", "host", bmcTask.Spec.Connection.Host)
		bmcTask.SetCondition(bmcv1alpha1.TaskFailed, bmcv1alpha1.ConditionTrue, bmcv1alpha1.WithTaskConditionMessage(fmt.Sprintf("Failed to connect to BMC: %v", err)))
		patchErr := r.patchStatus(ctx, bmcTask, bmcTaskPatch)
		if patchErr != nil {
			return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
		}

		return ctrl.Result{}, err
	}

	defer func() {
		// Close BMC connection after reconcilation
		err = bmcClient.Close(ctx)
		if err != nil {
			logger.Error(err, "BMC close connection failed", "host", bmcTask.Spec.Connection.Host)
		}
	}()

	// Task has StartTime, we check the status.
	// Requeue if actions did not complete.
	if !bmcTask.Status.StartTime.IsZero() {
		jobRunningTime := time.Since(bmcTask.Status.StartTime.Time)
		// TODO(pokearu): add timeout for tasks on API spec
		if jobRunningTime >= 3*time.Minute {
			timeOutErr := fmt.Errorf("bmc task timeout: %d", jobRunningTime)
			// Set Task Condition Failed True
			bmcTask.SetCondition(bmcv1alpha1.TaskFailed, bmcv1alpha1.ConditionTrue, bmcv1alpha1.WithTaskConditionMessage(timeOutErr.Error()))
			patchErr := r.patchStatus(ctx, bmcTask, bmcTaskPatch)
			if patchErr != nil {
				return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, timeOutErr})
			}

			return ctrl.Result{}, timeOutErr
		}

		result, err := r.checkBMCTaskStatus(ctx, bmcTask.Spec.Task, bmcClient)
		if err != nil {
			return result, fmt.Errorf("bmc task status check: %s", err)
		}

		if !result.IsZero() {
			return result, nil
		}

		// Set the Task CompletionTime
		now := metav1.Now()
		bmcTask.Status.CompletionTime = &now
		// Set Task Condition Completed True
		bmcTask.SetCondition(bmcv1alpha1.TaskCompleted, bmcv1alpha1.ConditionTrue)
		if err := r.patchStatus(ctx, bmcTask, bmcTaskPatch); err != nil {
			return result, err
		}

		return result, nil
	}

	// Set the Task StartTime
	now := metav1.Now()
	bmcTask.Status.StartTime = &now
	// run the specified Task in BMCTask
	if err := r.runBMCTask(ctx, bmcTask.Spec.Task, bmcClient); err != nil {
		// Set Task Condition Failed True
		bmcTask.SetCondition(bmcv1alpha1.TaskFailed, bmcv1alpha1.ConditionTrue, bmcv1alpha1.WithTaskConditionMessage(err.Error()))
		patchErr := r.patchStatus(ctx, bmcTask, bmcTaskPatch)
		if patchErr != nil {
			return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
		}

		return ctrl.Result{}, err
	}

	if err := r.patchStatus(ctx, bmcTask, bmcTaskPatch); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// runBMCTask executes the defined Task in a BMCTask
func (r *BMCTaskReconciler) runBMCTask(ctx context.Context, task bmcv1alpha1.Task, bmcClient BMCClient) error {
	if task.PowerAction != nil {
		_, err := bmcClient.SetPowerState(ctx, string(*task.PowerAction))
		if err != nil {
			return fmt.Errorf("failed to perform PowerAction: %v", err)
		}
	}

	if task.OneTimeBootDeviceAction != nil {
		// OneTimeBootDeviceAction currently sets the first boot device from Devices.
		// setPersistent is false.
		_, err := bmcClient.SetBootDevice(ctx, string(task.OneTimeBootDeviceAction.Devices[0]), false, task.OneTimeBootDeviceAction.EFIBoot)
		if err != nil {
			return fmt.Errorf("failed to perform OneTimeBootDeviceAction: %v", err)
		}
	}

	return nil
}

// checkBMCTaskStatus checks if Task action completed.
// This is currently limited only to a few PowerAction types.
func (r *BMCTaskReconciler) checkBMCTaskStatus(ctx context.Context, task bmcv1alpha1.Task, bmcClient BMCClient) (ctrl.Result, error) {
	// TODO(pokearu): Extend to all actions.
	if task.PowerAction != nil {
		powerStatus, err := bmcClient.GetPowerState(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get power state: %v", err)
		}

		switch *task.PowerAction {
		case bmcv1alpha1.PowerOn:
			if bmcv1alpha1.On != bmcv1alpha1.PowerState(strings.ToLower(powerStatus)) {
				return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
			}
		case bmcv1alpha1.HardPowerOff, bmcv1alpha1.SoftPowerOff:
			if bmcv1alpha1.Off != bmcv1alpha1.PowerState(strings.ToLower(powerStatus)) {
				return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
			}
		}
	}

	// Other Task action types do not support checking status. So noop.
	return ctrl.Result{}, nil
}

// patchStatus patches the specified patch on the BMCTask.
func (r *BMCTaskReconciler) patchStatus(ctx context.Context, bmcTask *bmcv1alpha1.BMCTask, patch client.Patch) error {
	err := r.client.Status().Patch(ctx, bmcTask, patch)
	if err != nil {
		return fmt.Errorf("failed to patch BMCTask %s/%s status: %v", bmcTask.Namespace, bmcTask.Name, err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BMCTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bmcv1alpha1.BMCTask{}).
		Complete(r)
}
