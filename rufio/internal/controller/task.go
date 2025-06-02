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

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const powerActionRequeueAfter = 3 * time.Second

// TaskReconciler reconciles a Task object.
type TaskReconciler struct {
	client           client.Client
	bmcClientFactory ClientFunc
}

// NewTaskReconciler returns a new TaskReconciler.
func NewTaskReconciler(c client.Client, bmcClientFactory ClientFunc) *TaskReconciler {
	return &TaskReconciler{
		client:           c,
		bmcClientFactory: bmcClientFactory,
	}
}

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=tasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=tasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=tasks/finalizers,verbs=update

// Reconcile runs a Task.
// Establishes a connection to the BMC.
// Runs the specified action in the Task.
func (r *TaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithName("controllers/Task").WithValues("task", req.NamespacedName)
	logger.Info("Reconciling Task")

	// Fetch the Task object
	task := &bmc.Task{}
	if err := r.client.Get(ctx, req.NamespacedName, task); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get Task")
		return ctrl.Result{}, err
	}

	// Deletion is a noop.
	if !task.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Task is Completed or Failed is noop.
	if task.HasCondition(bmc.TaskFailed, bmc.ConditionTrue) ||
		task.HasCondition(bmc.TaskCompleted, bmc.ConditionTrue) {
		return ctrl.Result{}, nil
	}

	// Create a patch from the initial Task object
	// Patch is used to update Status after reconciliation
	taskPatch := client.MergeFrom(task.DeepCopy())
	logger = logger.WithValues("action", task.Spec.Task, "host", task.Spec.Connection.Host)

	return r.doReconcile(ctx, task, taskPatch, logger)
}

func (r *TaskReconciler) doReconcile(ctx context.Context, task *bmc.Task, taskPatch client.Patch, logger logr.Logger) (ctrl.Result, error) {
	var username, password string
	opts := &BMCOptions{
		ProviderOptions: task.Spec.Connection.ProviderOptions,
	}
	if task.Spec.Connection.ProviderOptions != nil && task.Spec.Connection.ProviderOptions.RPC != nil {
		opts.ProviderOptions = task.Spec.Connection.ProviderOptions
		if task.Spec.Connection.ProviderOptions.RPC.HMAC != nil && len(task.Spec.Connection.ProviderOptions.RPC.HMAC.Secrets) > 0 {
			se, err := retrieveHMACSecrets(ctx, r.client, task.Spec.Connection.ProviderOptions.RPC.HMAC.Secrets)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to get hmac secrets: %w", err)
			}
			opts.rpcSecrets = se
		}
	} else {
		// Fetching username, password from SecretReference in Connection.
		// Requeue if error fetching secret
		var err error
		username, password, err = resolveAuthSecretRef(ctx, r.client, task.Spec.Connection.AuthSecretRef)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("resolving connection secret for task %s/%s: %w", task.Namespace, task.Name, err)
		}
	}

	// Initializing BMC Client
	bmcClient, err := r.bmcClientFactory(ctx, logger, task.Spec.Connection.Host, username, password, opts)
	if err != nil {
		logger.Error(err, "BMC connection failed", "host", task.Spec.Connection.Host)
		task.SetCondition(bmc.TaskFailed, bmc.ConditionTrue, bmc.WithTaskConditionMessage(fmt.Sprintf("Failed to connect to BMC: %v", err)))
		patchErr := r.patchStatus(ctx, task, taskPatch)
		if patchErr != nil {
			return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
		}

		return ctrl.Result{}, err
	}
	defer func() {
		// Close BMC connection after reconciliation
		if err := bmcClient.Close(ctx); err != nil {
			md := bmcClient.GetMetadata()
			logger.Error(err, "BMC close connection failed", "providersAttempted", md.ProvidersAttempted)

			return
		}
		md := bmcClient.GetMetadata()
		logger.Info("BMC connection closed", "successfulCloseConns", md.SuccessfulCloseConns, "providersAttempted", md.ProvidersAttempted, "successfulProvider", md.SuccessfulProvider)
	}()

	// Task has StartTime, we check the status.
	// Requeue if actions did not complete.
	if !task.Status.StartTime.IsZero() {
		jobRunningTime := time.Since(task.Status.StartTime.Time)
		// TODO(pokearu): add timeout for tasks on API spec
		if jobRunningTime >= 10*time.Minute {
			timeOutErr := fmt.Errorf("bmc task timeout: %d", jobRunningTime)
			// Set Task Condition Failed True
			task.SetCondition(bmc.TaskFailed, bmc.ConditionTrue, bmc.WithTaskConditionMessage(timeOutErr.Error()))
			patchErr := r.patchStatus(ctx, task, taskPatch)
			if patchErr != nil {
				return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, timeOutErr})
			}

			return ctrl.Result{}, timeOutErr
		}

		result, err := r.checkTaskStatus(ctx, logger, task.Spec.Task, bmcClient)
		if err != nil {
			return result, fmt.Errorf("bmc task status check: %w", err)
		}

		if !result.IsZero() {
			return result, nil
		}

		// Set the Task CompletionTime
		now := metav1.Now()
		task.Status.CompletionTime = &now
		// Set Task Condition Completed True
		task.SetCondition(bmc.TaskCompleted, bmc.ConditionTrue)
		if err := r.patchStatus(ctx, task, taskPatch); err != nil {
			return result, err
		}

		return result, nil
	}

	logger.Info("new task run")

	// Set the Task StartTime
	now := metav1.Now()
	task.Status.StartTime = &now
	// run the specified Task in Task
	if err := r.runTask(ctx, logger, task.Spec.Task, bmcClient); err != nil {
		md := bmcClient.GetMetadata()
		logger.Info("failed to perform action", "providersAttempted", md.ProvidersAttempted, "action", task.Spec.Task)
		// Set Task Condition Failed True
		task.SetCondition(bmc.TaskFailed, bmc.ConditionTrue, bmc.WithTaskConditionMessage(err.Error()))
		patchErr := r.patchStatus(ctx, task, taskPatch)
		if patchErr != nil {
			return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
		}

		return ctrl.Result{}, err
	}

	if err := r.patchStatus(ctx, task, taskPatch); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// runTask executes the defined Task in a Task.
func (r *TaskReconciler) runTask(ctx context.Context, logger logr.Logger, task bmc.Action, bmcClient *bmclib.Client) error {
	if task.PowerAction != nil {
		ok, err := bmcClient.SetPowerState(ctx, string(*task.PowerAction))
		if err != nil {
			return fmt.Errorf("failed to perform PowerAction: %w", err)
		}
		md := bmcClient.GetMetadata()
		logger.Info("power state set successfully", "providersAttempted", md.ProvidersAttempted, "successfulProvider", md.SuccessfulProvider, "ok", ok)
	}

	if task.OneTimeBootDeviceAction != nil {
		// OneTimeBootDeviceAction currently sets the first boot device from Devices.
		// setPersistent is false.
		ok, err := bmcClient.SetBootDevice(ctx, string(task.OneTimeBootDeviceAction.Devices[0]), false, task.OneTimeBootDeviceAction.EFIBoot)
		if err != nil {
			return fmt.Errorf("failed to perform OneTimeBootDeviceAction: %w", err)
		}
		md := bmcClient.GetMetadata()
		logger.Info("one time boot device set successfully", "providersAttempted", md.ProvidersAttempted, "successfulProvider", md.SuccessfulProvider, "ok", ok)
	}

	if task.VirtualMediaAction != nil {
		ok, err := bmcClient.SetVirtualMedia(ctx, string(task.VirtualMediaAction.Kind), task.VirtualMediaAction.MediaURL)
		if err != nil {
			return fmt.Errorf("failed to perform SetVirtualMedia: %w", err)
		}
		md := bmcClient.GetMetadata()
		logger.Info("virtual media set successfully", "providersAttempted", md.ProvidersAttempted, "successfulProvider", md.SuccessfulProvider, "ok", ok)
	}

	return nil
}

// checkTaskStatus checks if Task action completed.
// This is currently limited only to a few PowerAction types.
func (r *TaskReconciler) checkTaskStatus(ctx context.Context, log logr.Logger, task bmc.Action, bmcClient *bmclib.Client) (ctrl.Result, error) {
	// TODO(pokearu): Extend to all actions.
	if task.PowerAction != nil {
		rawState, err := bmcClient.GetPowerState(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get power state: %w", err)
		}
		log = log.WithValues("currentPowerState", rawState)
		log.Info("power state check")

		state := toPowerState(rawState)

		switch *task.PowerAction { //nolint:exhaustive // we only support a few power actions right now.
		case bmc.PowerOn:
			if state != bmc.On {
				log.Info("requeuing task", "requeueAfter", powerActionRequeueAfter)
				return ctrl.Result{RequeueAfter: powerActionRequeueAfter}, nil
			}
		case bmc.PowerHardOff, bmc.PowerSoftOff:
			if bmc.Off != state {
				return ctrl.Result{RequeueAfter: powerActionRequeueAfter}, nil
			}
		}
	}

	// Other Task action types do not support checking status. So noop.
	return ctrl.Result{}, nil
}

// patchStatus patches the specified patch on the Task.
func (r *TaskReconciler) patchStatus(ctx context.Context, task *bmc.Task, patch client.Patch) error {
	err := r.client.Status().Patch(ctx, task, patch)
	if err != nil {
		return fmt.Errorf("failed to patch Task %s/%s status: %w", task.Namespace, task.Name, err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bmc.Task{}).
		Complete(r)
}
