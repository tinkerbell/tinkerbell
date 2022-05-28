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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BMCJobConditionType represents the condition of the BMC Job.
type BMCJobConditionType string

const (
	// JobCompleted represents successful completion of the BMC Job tasks.
	JobCompleted BMCJobConditionType = "Completed"
	// JobFailed represents failure in BMC job execution.
	JobFailed BMCJobConditionType = "Failed"
	// JobRunning represents a currently executing BMC job.
	JobRunning BMCJobConditionType = "Running"
)

// PowerControl represents the power control operation on the baseboard management.
type PowerControl string

const (
	PowerOn      PowerControl = "on"
	HardPowerOff PowerControl = "off"
	SoftPowerOff PowerControl = "soft"
	Cycle        PowerControl = "cycle"
	Reset        PowerControl = "reset"
	Status       PowerControl = "status"
)

// BMCJobSpec defines the desired state of BMCJob
type BMCJobSpec struct {
	// BaseboardManagementRef represents the BaseboardManagement resource to execute the job.
	// All the tasks in the job are executed for the same BaseboardManagement.
	BaseboardManagementRef BaseboardManagementRef `json:"baseboardManagementRef"`

	// Tasks represents a list of baseboard management actions to be executed.
	// The tasks are executed sequentially. Controller waits for one task to complete before executing the next.
	// If a single task fails, job execution stops and sets condition Failed.
	// Condition Completed is set only if all the tasks were successful.
	Tasks []Task `json:"tasks"`
}

// BMCJobStatus defines the observed state of BMCJob
type BMCJobStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	Conditions []BMCJobCondition `json:"conditions,omitempty"`

	// StartTime represents time when the BMCJob controller started processing a job.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime represents time when the job was completed.
	// The completion time is only set when the job finishes successfully.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

type BMCJobCondition struct {
	// Type of the BMCJob condition.
	Type BMCJobConditionType `json:"type"`

	// Status is the status of the BMCJob condition.
	// Can be True or False.
	Status ConditionStatus `json:"status"`

	// Message represents human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:generate=false
type BMCJobSetConditionOption func(*BMCJobCondition)

// SetCondition applies the cType condition to bmj. If the condition already exists,
// it is updated.
func (bmj *BMCJob) SetCondition(cType BMCJobConditionType, status ConditionStatus, opts ...BMCJobSetConditionOption) {
	var condition *BMCJobCondition

	// Check if there's an existing condition.
	for i, c := range bmj.Status.Conditions {
		if c.Type == cType {
			condition = &bmj.Status.Conditions[i]
			break
		}
	}

	// We didn't find an existing condition so create a new one and append it.
	if condition == nil {
		bmj.Status.Conditions = append(bmj.Status.Conditions, BMCJobCondition{
			Type: cType,
		})
		condition = &bmj.Status.Conditions[len(bmj.Status.Conditions)-1]
	}

	condition.Status = status
	for _, opt := range opts {
		opt(condition)
	}
}

// WithJobConditionMessage sets message m to the BMCJobCondition.
func WithJobConditionMessage(m string) BMCJobSetConditionOption {
	return func(c *BMCJobCondition) {
		c.Message = m
	}
}

// HasCondition checks if the cType condition is present with status cStatus on a bmj.
func (bmj *BMCJob) HasCondition(cType BMCJobConditionType, cStatus ConditionStatus) bool {
	for _, c := range bmj.Status.Conditions {
		if c.Type == cType {
			return c.Status == cStatus
		}
	}

	return false
}

// FormatTaskName returns a BMCTask name based on BMCJob name.
func FormatTaskName(job BMCJob, n int) string {
	return fmt.Sprintf("%s-task-%d", job.Name, n)
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:path=bmcjobs,scope=Namespaced,categories=tinkerbell,singular=bmcjob,shortName=bmj

// BMCJob is the Schema for the bmcjobs API
type BMCJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCJobSpec   `json:"spec,omitempty"`
	Status BMCJobStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BMCJobList contains a list of BMCJob
type BMCJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMCJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BMCJob{}, &BMCJobList{})
}
