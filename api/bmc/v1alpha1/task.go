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

// TaskConditionType represents the condition type on for Tasks.
type TaskConditionType string

const (
	// TaskCompleted represents successful completion of the Task.
	TaskCompleted TaskConditionType = "Completed"
	// TaskFailed represents failure in Task execution.
	TaskFailed TaskConditionType = "Failed"
)

// TaskSpec defines the desired state of Task.
type TaskSpec struct {
	// Task defines the specific action to be performed.
	Task Action `json:"task"`

	// Connection represents the Machine connectivity information.
	Connection Connection `json:"connection,omitempty"`
}

// Action represents the action to be performed.
// A single task can only perform one type of action.
// For example either PowerAction or OneTimeBootDeviceAction.
// +kubebuilder:validation:MaxProperties:=1
type Action struct {
	// PowerAction represents a baseboard management power operation.
	// +kubebuilder:validation:Enum=on;off;soft;status;cycle;reset
	PowerAction *PowerAction `json:"powerAction,omitempty"`

	// OneTimeBootDeviceAction represents a baseboard management one time set boot device operation.
	OneTimeBootDeviceAction *OneTimeBootDeviceAction `json:"oneTimeBootDeviceAction,omitempty"`

	// VirtualMediaAction represents a baseboard management virtual media insert/eject.
	VirtualMediaAction *VirtualMediaAction `json:"virtualMediaAction,omitempty"`
}

// TaskStatus defines the observed state of Task.
type TaskStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	Conditions []TaskCondition `json:"conditions,omitempty"`

	// StartTime represents time when the Task started processing.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime represents time when the task was completed.
	// The completion time is only set when the task finishes successfully.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

type TaskCondition struct {
	// Type of the Task condition.
	Type TaskConditionType `json:"type"`

	// Status is the status of the Task condition.
	// Can be True or False.
	Status ConditionStatus `json:"status"`

	// Message represents human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:generate=false
type TaskSetConditionOption func(*TaskCondition)

// SetCondition applies the cType condition to bmt. If the condition already exists,
// it is updated.
func (t *Task) SetCondition(cType TaskConditionType, status ConditionStatus, opts ...TaskSetConditionOption) {
	var condition *TaskCondition

	// Check if there's an existing condition.
	for i, c := range t.Status.Conditions {
		if c.Type == cType {
			condition = &t.Status.Conditions[i]
			break
		}
	}

	// We didn't find an existing condition so create a new one and append it.
	if condition == nil {
		t.Status.Conditions = append(t.Status.Conditions, TaskCondition{
			Type: cType,
		})
		condition = &t.Status.Conditions[len(t.Status.Conditions)-1]
	}

	condition.Status = status
	for _, opt := range opts {
		opt(condition)
	}
}

// WithTaskConditionMessage sets message m to the TaskCondition.
func WithTaskConditionMessage(m string) TaskSetConditionOption {
	return func(c *TaskCondition) {
		c.Message = m
	}
}

// HasCondition checks if the cType condition is present with status cStatus on a bmt.
func (t *Task) HasCondition(cType TaskConditionType, cStatus ConditionStatus) bool {
	for _, c := range t.Status.Conditions {
		if c.Type == cType {
			return c.Status == cStatus
		}
	}

	return false
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=tasks,scope=Namespaced,categories=tinkerbell,singular=task,shortName=t

// Task is the Schema for the Task API.
type Task struct {
	metav1.TypeMeta   `json:""`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskSpec   `json:"spec,omitempty"`
	Status TaskStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TaskList contains a list of Task.
type TaskList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Task{}, &TaskList{})
}
