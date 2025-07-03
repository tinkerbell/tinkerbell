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
	metav1.TypeMeta   `json:""`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JobSpec   `json:"spec,omitempty"`
	Status JobStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// JobList contains a list of Job.
type JobList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Job `json:"items"`
}

// JobSpec defines the desired state of Job.
type JobSpec struct {
	// MachineRef represents the Machine resource to execute the job.
	// All the tasks in the job are executed for the same Machine.
	MachineRef MachineRef `json:"machineRef"`

	// Tasks represents a list of baseboard management actions to be executed.
	// The tasks are executed sequentially. Controller waits for one task to complete before executing the next.
	// If a single task fails, job execution stops and sets condition Failed.
	// Condition Completed is set only if all the tasks were successful.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:UniqueItems=false
	Tasks []Operations `json:"tasks"`
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

// MachineRef is used to reference a Machine object.
type MachineRef struct {
	// Name of the Machine.
	Name string `json:"name"`

	// Namespace the Machine resides in.
	Namespace string `json:"namespace"`
}
