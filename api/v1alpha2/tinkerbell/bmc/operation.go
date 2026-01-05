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
// +kubebuilder:resource:path=operations,scope=Namespaced,categories=tinkerbell,singular=operation,shortName=o

// Operation is the Schema for the Operation API.
type Operation struct {
	metav1.TypeMeta   `json:""`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OperationSpec   `json:"spec,omitempty"`
	Status OperationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OperationList contains a list of Operation.
type OperationList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Operation `json:"items"`
}

// OperationSpec defines the desired state of Operation.
type OperationSpec struct {
	// Operation defines the specific operation to be performed.
	Operation Operations `json:"operation,omitempty"`

	// TimeoutSeconds defines the maximum time in seconds the Operation is allowed to run.
	// If not defined, the default from the runtime is used.
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`

	// Connection represents the BMC connectivity information.
	Connection Connection `json:"connection,omitempty"`
}

// OperationStatus defines the observed state of Operation.
type OperationStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`

	// StartTime represents time when the Operation started processing.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime represents time when the Operation was completed.
	// The completion time is only set when the Operation finishes successfully.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}
