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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
)

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmctasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmctasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=bmctasks/finalizers,verbs=update

// BMCTaskHandler handles a BMCTask creation and execution.
type BMCTaskHandler struct{}

// NewBMCTaskHandler returns a new BMCTaskHandler
func NewBMCTaskHandler() BMCTaskHandler {
	return BMCTaskHandler{}
}

// CreateBMCTaskWithOwner creates a BMCTask object with an OwnerReference set to the BMCJob
func (r *BMCTaskHandler) CreateBMCTaskWithOwner(ctx context.Context, client client.Client, task bmcv1alpha1.Task, taskIndex int, bmcTask *bmcv1alpha1.BMCTask, bmj *bmcv1alpha1.BMCJob) error {
	bmcTask.ObjectMeta = metav1.ObjectMeta{
		Name:      bmj.GetTaskName(taskIndex),
		Namespace: bmj.Namespace,
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: bmj.APIVersion,
				Kind:       bmj.Kind,
				Name:       bmj.Name,
				UID:        bmj.ObjectMeta.UID,
			},
		},
	}
	bmcTask.Spec = bmcv1alpha1.BMCTaskSpec{
		Task: task,
	}

	err := client.Create(ctx, bmcTask)
	if err != nil {
		return fmt.Errorf("failed to create BMCTask %s/%s: %v", bmcTask.Namespace, bmcTask.Name, err)
	}

	return nil
}

// RunBMCTask executes the defined Task in a BMCTask
func (r *BMCTaskHandler) RunBMCTask(ctx context.Context, bmcTask *bmcv1alpha1.BMCTask, bmcClient BMCClient) error {
	now := metav1.Now()
	bmcTask.Status.StartTime = &now
	if bmcTask.Spec.Task.PowerAction != nil {
		_, err := bmcClient.SetPowerState(ctx, string(bmcTask.Spec.Task.PowerAction.PowerControl))
		if err != nil {
			bmcTask.SetCondition(bmcv1alpha1.TaskFailed, bmcv1alpha1.ConditionTrue, bmcv1alpha1.WithTaskConditionMessage(fmt.Sprintf("Failed to perform PowerAction: %v", err)))
			return fmt.Errorf("failed to perform PowerAction: %v", err)
		}
	}

	if bmcTask.Spec.Task.OneTimeBootDeviceAction != nil {
		// OneTimeBootDeviceAction currently sets the first boot device from Devices.
		// setPersistent is false.
		_, err := bmcClient.SetBootDevice(ctx, string(bmcTask.Spec.Task.OneTimeBootDeviceAction.Devices[0]), false, bmcTask.Spec.Task.OneTimeBootDeviceAction.EFIBoot)
		if err != nil {
			bmcTask.SetCondition(bmcv1alpha1.TaskFailed, bmcv1alpha1.ConditionTrue, bmcv1alpha1.WithTaskConditionMessage(fmt.Sprintf("Failed to perform OneTimeBootDeviceAction: %v", err)))
			return fmt.Errorf("failed to perform OneTimeBootDeviceAction: %v", err)
		}
	}

	now = metav1.Now()
	bmcTask.Status.CompletionTime = &now
	bmcTask.SetCondition(bmcv1alpha1.TaskCompleted, bmcv1alpha1.ConditionTrue)

	return nil
}

// PatchBMCTaskStatus patches the specified patch on the BMCTask.
func (r *BMCTaskHandler) PatchBMCTaskStatus(ctx context.Context, client client.Client, bmcTask *bmcv1alpha1.BMCTask, patch client.Patch) error {
	err := client.Status().Patch(ctx, bmcTask, patch)
	if err != nil {
		return fmt.Errorf("failed to patch BMCTask %s/%s status: %v", bmcTask.Namespace, bmcTask.Name, err)
	}

	return nil
}
