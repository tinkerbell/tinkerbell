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

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion
//+kubebuilder:resource:path=jobs,scope=Namespaced,categories=tinkerbell,singular=job,shortName=j

// Job is the Schema for the bmcjobs API.
type Job struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JobSpec   `json:"spec,omitempty"`
	Status JobStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// JobList contains a list of Job.
type JobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Job `json:"items"`
}

// JobSpec defines the desired state of Job.
// +kubebuilder:validation:XValidation:rule="!(has(self.bmcRef) && has(self.connection))",message="bmcRef and connection are mutually exclusive"
type JobSpec struct {
	// BMCRef represents the BMC object that will be used for connection details when executing the Job.
	// All the operations in the Job are executed for the same BMC.
	// Mutually exclusive with Connection.
	// +optional
	BMCRef SimpleReference `json:"bmcRef,omitempty"`

	// Connection contains connection details that will be used for executing the Job.
	// Mutually exclusive with BMCRef.
	// +optional
	Connection Connection `json:"connection,omitempty"`

	// Operations represents a list of baseboard management actions to be executed.
	// The operations are executed sequentially. Controller waits for one operation to complete before executing the next.
	// If a single operation fails, job execution stops and sets condition Failed.
	// Condition Completed is set only if all the operations were successful.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:UniqueItems=false
	Operations []Operations `json:"operations"`
}

// JobStatus defines the observed state of Job.
type JobStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`

	// StartTime represents time when the Job controller started processing a job.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime represents time when the job was completed.
	// The completion time is only set when the job finishes successfully.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// SimpleReference is used to reference an object.
type SimpleReference struct {
	// Name of the object.
	Name string `json:"name"`

	// Namespace in which the object resides.
	Namespace string `json:"namespace"`
}
