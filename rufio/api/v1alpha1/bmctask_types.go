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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCTaskConditionType represents the condition of the BMC Job.
type BMCTaskConditionType string

const (
	// TaskCompleted represents successful completion of the BMC Task.
	TaskCompleted BMCJobConditionType = "Completed"
	// TaskFailed represents failure in BMC task execution.
	TaskFailed BMCJobConditionType = "Failed"
)

// BMCTaskSpec defines the desired state of BMCTask
type BMCTaskSpec struct {
	// Task defines the specific action to be performed.
	Task Task `json:"task"`
}

// Task represents the action to be performed.
// A single task can only perform one type of action.
// For example either PowerAction or OneTimeBootDeviceAction.
// +kubebuilder:validation:MaxProperties:=1
type Task struct {
	// PowerAction represents a baseboard management power operation.
	PowerAction *PowerAction `json:"powerAction,omitempty"`

	// OneTimeBootDeviceAction represents a baseboard management one time set boot device operation.
	OneTimeBootDeviceAction *OneTimeBootDeviceAction `json:"oneTimeBootDeviceAction,omitempty"`
}

type PowerAction struct {
	// State represents the requested power state to set for the baseboard management.
	// +kubebuilder:validation:Enum=PowerOn;HardPowerOff;SoftPowerOff;Status;Cycle;Reset
	PowerControl PowerControl `json:"powerControl"`
}

type OneTimeBootDeviceAction struct {
	// Devices represents the boot devices, in order for setting one time boot.
	// Currently only the first device in the slice is used to set one time boot.
	Devices []BootDevice `json:"device"`

	// EFIBoot specifies to EFI boot for the baseboard management.
	// When true, enables options=efiboot while setting the boot device.
	// If false, no options are passed.
	// +kubebuilder:default=false
	EFIBoot bool `json:"efiBoot,omitempty"`
}

// BMCTaskStatus defines the observed state of BMCTask
type BMCTaskStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	Conditions []BMCTaskCondition `json:"conditions,omitempty"`

	// StartTime represents time when the BMCTask started processing.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime represents time when the task was completed.
	// The completion time is only set when the task finishes successfully.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

type BMCTaskCondition struct {
	// Type of the BMCTask condition.
	Type BMCTaskConditionType `json:"type"`

	// Message represents human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=bmctasks,scope=Namespaced,categories=tinkerbell,singular=bmctask,shortName=bmt

// BMCTask is the Schema for the bmctasks API
type BMCTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCTaskSpec   `json:"spec,omitempty"`
	Status BMCTaskStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BMCTaskList contains a list of BMCTask
type BMCTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCTask{}, &BMCTaskList{})
}
