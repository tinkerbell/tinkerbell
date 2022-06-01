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

// BMCTaskConditionType represents the condition of the BMC Task.
type BMCTaskConditionType string

const (
	// TaskCompleted represents successful completion of the BMC Task.
	TaskCompleted BMCTaskConditionType = "Completed"
	// TaskFailed represents failure in BMC task execution.
	TaskFailed BMCTaskConditionType = "Failed"
)

// BMCTaskSpec defines the desired state of BMCTask
type BMCTaskSpec struct {
	// Task defines the specific action to be performed.
	Task Task `json:"task"`

	// Connection represents the BaseboardManagement connectivity information.
	Connection Connection `json:"connection,omitempty"`
}

// Task represents the action to be performed.
// A single task can only perform one type of action.
// For example either PowerAction or OneTimeBootDeviceAction.
// +kubebuilder:validation:MaxProperties:=1
type Task struct {
	// PowerAction represents a baseboard management power operation.
	// +kubebuilder:validation:Enum=on;off;soft;status;cycle;reset
	PowerAction *PowerAction `json:"powerAction,omitempty"`

	// OneTimeBootDeviceAction represents a baseboard management one time set boot device operation.
	OneTimeBootDeviceAction *OneTimeBootDeviceAction `json:"oneTimeBootDeviceAction,omitempty"`
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

	// Status is the status of the BMCTask condition.
	// Can be True or False.
	Status ConditionStatus `json:"status"`

	// Message represents human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:generate=false
type BMCTaskSetConditionOption func(*BMCTaskCondition)

// SetCondition applies the cType condition to bmt. If the condition already exists,
// it is updated.
func (bmt *BMCTask) SetCondition(cType BMCTaskConditionType, status ConditionStatus, opts ...BMCTaskSetConditionOption) {
	var condition *BMCTaskCondition

	// Check if there's an existing condition.
	for i, c := range bmt.Status.Conditions {
		if c.Type == cType {
			condition = &bmt.Status.Conditions[i]
			break
		}
	}

	// We didn't find an existing condition so create a new one and append it.
	if condition == nil {
		bmt.Status.Conditions = append(bmt.Status.Conditions, BMCTaskCondition{
			Type: cType,
		})
		condition = &bmt.Status.Conditions[len(bmt.Status.Conditions)-1]
	}

	condition.Status = status
	for _, opt := range opts {
		opt(condition)
	}
}

// WithTaskConditionMessage sets message m to the BMCTaskCondition.
func WithTaskConditionMessage(m string) BMCTaskSetConditionOption {
	return func(c *BMCTaskCondition) {
		c.Message = m
	}
}

// HasCondition checks if the cType condition is present with status cStatus on a bmt.
func (bmt *BMCTask) HasCondition(cType BMCTaskConditionType, cStatus ConditionStatus) bool {
	for _, c := range bmt.Status.Conditions {
		if c.Type == cType {
			return c.Status == cStatus
		}
	}

	return false
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
