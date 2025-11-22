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

package bmc

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:path=tasks,scope=Namespaced,categories=tinkerbell,singular=task,shortName=t

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

// TaskSpec defines the desired state of Task.
type TaskSpec struct {
	// Task defines the specific operation to be performed.
	Task Operations `json:"task,omitempty"`

	// TimeoutSeconds defines the maximum time in seconds the Task is allowed to run.
	// If not defined, the default from the runtime is used.
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`

	// Connection represents the Machine connectivity information.
	Connection Connection `json:"connection,omitempty"`
}

// TaskStatus defines the observed state of Task.
type TaskStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`

	// StartTime represents time when the Task started processing.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime represents time when the task was completed.
	// The completion time is only set when the task finishes successfully.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}
