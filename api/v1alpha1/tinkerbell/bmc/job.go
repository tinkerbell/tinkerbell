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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobConditionType represents the condition of the BMC Job.
type JobConditionType string

const (
	// JobCompleted represents successful completion of the BMC Job tasks.
	JobCompleted JobConditionType = "Completed"
	// JobFailed represents failure in BMC job execution.
	JobFailed JobConditionType = "Failed"
	// JobRunning represents a currently executing BMC job.
	JobRunning JobConditionType = "Running"
)

// MachineRef is used to reference a Machine object.
type MachineRef struct {
	// Name of the Machine.
	Name string `json:"name"`

	// Namespace the Machine resides in.
	Namespace string `json:"namespace"`
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
	Tasks []Action `json:"tasks"`
}

// JobStatus defines the observed state of Job.
type JobStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	Conditions []JobCondition `json:"conditions,omitempty"`

	// StartTime represents time when the Job controller started processing a job.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime represents time when the job was completed.
	// The completion time is only set when the job finishes successfully.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

type JobCondition struct {
	// Type of the Job condition.
	Type JobConditionType `json:"type"`

	// Status is the status of the Job condition.
	// Can be True or False.
	Status ConditionStatus `json:"status"`

	// Message represents human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:generate=false
type JobSetConditionOption func(*JobCondition)

// SetCondition applies the cType condition to bmj. If the condition already exists,
// it is updated.
func (j *Job) SetCondition(cType JobConditionType, status ConditionStatus, opts ...JobSetConditionOption) {
	var condition *JobCondition

	// Check if there's an existing condition.
	for i, c := range j.Status.Conditions {
		if c.Type == cType {
			condition = &j.Status.Conditions[i]
			break
		}
	}

	// We didn't find an existing condition so create a new one and append it.
	if condition == nil {
		j.Status.Conditions = append(j.Status.Conditions, JobCondition{
			Type: cType,
		})
		condition = &j.Status.Conditions[len(j.Status.Conditions)-1]
	}

	condition.Status = status
	for _, opt := range opts {
		opt(condition)
	}
}

// WithJobConditionMessage sets message m to the JobCondition.
func WithJobConditionMessage(m string) JobSetConditionOption {
	return func(c *JobCondition) {
		c.Message = m
	}
}

// HasCondition checks if the cType condition is present with status cStatus on a bmj.
func (j *Job) HasCondition(cType JobConditionType, cStatus ConditionStatus) bool {
	for _, c := range j.Status.Conditions {
		if c.Type == cType {
			return c.Status == cStatus
		}
	}

	return false
}

// FormatTaskName returns a Task name based on Job name.
func FormatTaskName(job Job, n int) string {
	return fmt.Sprintf("%s-task-%d", job.Name, n)
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
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
