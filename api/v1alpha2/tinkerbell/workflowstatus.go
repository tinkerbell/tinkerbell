package tinkerbell

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// WorkflowStatus defines the observed state of a Workflow.
type WorkflowStatus struct {
	// BootOptions holds the state of any boot options.
	BootOptions BootOptionsStatus `json:"bootOptions,omitempty"`

	// Metadata tracks where the Workflow is in its execution.
	// It holds the current state of the Workflow as a whole,
	// the current Task, and the current Action.
	Metadata WorkflowMetadata `json:"metadata,omitempty"`

	// GlobalTimeout represents the max execution duration time.
	GlobalTimeout int64 `json:"globalTimeout,omitempty"`

	// GlobalExecutionStop represents the time when the Workflow should stop executing.
	// After this time, if the Workflow has not completed it will be marked as timed out.
	GlobalExecutionStop *metav1.Time `json:"globalExecutionStop,omitempty"`

	// RenderedTasks are the Tasks to be run by this Workflow.
	RenderedTasks []TaskWithMetadata `json:"renderedTasks,omitempty"`

	// Conditions are the latest available observations of an object's current state.
	//
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=atomic
	Conditions []WorkflowCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// TaskWithMetadata is a copy of TaskSpec with the addition of the ActionWithMetadata.
// Users don't need Metadata in the CRD Spec so we don't include it but we need
// it in the CRD Status.
type TaskWithMetadata struct {
	// Actions that the Task runs.
	Actions []ActionWithMetadata `json:"actions"`

	// Environment variables here are added to all Actions in the Task.
	// +optional
	Environment []EnvVar `json:"environment,omitempty"`

	// Name is a human readable name for the Task.
	Name string `json:"name"`

	// Volumes defined here are added to all Actions in the Task.
	// +optional
	Volumes []Volume `json:"volumes,omitempty"`

	Metadata Metadata `json:"taskMetadata,omitempty"`
}

// ActionWithMetadata
type ActionWithMetadata struct {
	Action `json:",inline"`

	Metadata Metadata `json:"actionMetadata,omitempty"`
}

// BootOptionsStatus holds the state of any boot options.
type BootOptionsStatus struct {
	// AllowNetboot holds the state of the the controller's interactions with the allowPXE field in a Hardware object.
	AllowNetboot AllowNetbootStatus `json:"allowNetboot,omitempty"`
	// Jobs holds the state of any job.bmc.tinkerbell.org objects created.
	Jobs map[string]JobStatus `json:"jobs,omitempty"`
}

type AllowNetbootStatus struct {
	ToggledTrue  bool `json:"toggledTrue,omitempty"`
	ToggledFalse bool `json:"toggledFalse,omitempty"`
}

// WorkflowMetadata tracks the execution progress at each level: Workflow, Task, and Action.
type WorkflowMetadata struct {
	Workflow Metadata `json:"workflow,omitempty"`
	Task     Metadata `json:"task,omitempty"`
	Action   Metadata `json:"action,omitempty"`
}

// Metadata tracks the state and timing of a single execution unit (Workflow, Task, or Action).
type Metadata struct {
	AgentID string `json:"agentID,omitempty"`

	ID types.UID `json:"id,omitempty"`

	Name string `json:"name,omitempty"`

	Hardware string `json:"hardware,omitempty"`

	State State `json:"state,omitempty"`

	StartTime *metav1.Time `json:"startTime,omitempty"`

	EndTime *metav1.Time `json:"endTime,omitempty"`

	ExecutionDuration string `json:"executionDuration,omitempty"`
}

// JobStatus holds the state of a specific job.bmc.tinkerbell.org object created.
type JobStatus struct {
	// UID is the UID of the job.bmc.tinkerbell.org object associated with this workflow.
	// This is used to uniquely identify the job.bmc.tinkerbell.org object, as
	// all objects for a specific Hardware/Machine.bmc.tinkerbell.org are created with the same name.
	UID types.UID `json:"uid,omitempty"`

	// Complete indicates whether the created job.bmc.tinkerbell.org has reported its conditions as complete.
	Complete bool `json:"complete,omitempty"`

	// ExistingJobDeleted indicates whether any existing job.bmc.tinkerbell.org was deleted.
	// The name of each job.bmc.tinkerbell.org object created by the controller is the same, so only one can exist at a time.
	// Using the same name was chosen so that there is only ever 1 job.bmc.tinkerbell.org per Hardware/Machine.bmc.tinkerbell.org.
	// This makes clean up easier and we dont just orphan jobs every time.
	ExistingJobDeleted bool `json:"existingJobDeleted,omitempty"`
}

// WorkflowCondition describes the current state of a Workflow condition.
type WorkflowCondition struct {
	// Type of condition.
	Type ConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=ConditionType"`
	// Status of the condition, one of True, False, Unknown.
	Status metav1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`
	// Reason is a (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,5,opt,name=reason"`
	// Message is a human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,6,opt,name=message"`
	// Time when the condition was created.
	// +optional
	Time *metav1.Time `json:"time,omitempty" protobuf:"bytes,7,opt,name=time"`
}

// HasCondition checks if the condition of type ct is present with status cs.
func (w *WorkflowStatus) HasCondition(ct ConditionType, cs metav1.ConditionStatus) bool {
	for _, c := range w.Conditions {
		if c.Type == ct {
			return c.Status == cs
		}
	}

	return false
}

// SetCondition updates conditions. If the condition already exists, it updates it.
// If the condition doesn't exist then it appends the new one (wc).
func (w *WorkflowStatus) SetCondition(wc WorkflowCondition) {
	index := -1
	for i, c := range w.Conditions {
		if c.Type == wc.Type {
			index = i
			break
		}
	}
	if index != -1 {
		w.Conditions[index] = wc
		return
	}

	w.Conditions = append(w.Conditions, wc)
}

// SetConditionIfDifferent updates the status with a condition, if:
//
// 1. the condition does not exist
//
// 2. the condition exists but any field except the .Time field is different
//
// This is needed so as to not overwhelm the kubernetes event system if failures grow.
// This limits the number of updates to the status so that we don't continually
// update the status with the same information and cause unnecessary Kubernetes events.
func (w *WorkflowStatus) SetConditionIfDifferent(wc WorkflowCondition) {
	index := -1
	for i, c := range w.Conditions {
		if c.Type == wc.Type {
			index = i
			break
		}
	}
	if index != -1 {
		if w.Conditions[index].Status == wc.Status && w.Conditions[index].Reason == wc.Reason && w.Conditions[index].Message == wc.Message {
			return
		}
		w.Conditions[index] = wc
		return
	}

	w.Conditions = append(w.Conditions, wc)
}

const (
	BootJobFailed        ConditionType = "BootJobFailed"
	BootJobComplete      ConditionType = "BootJobComplete"
	BootJobRunning       ConditionType = "BootJobRunning"
	BootJobSetupFailed   ConditionType = "BootJobSetupFailed"
	BootJobSetupComplete ConditionType = "BootJobSetupComplete"

	ToggleNetbootTrue  ConditionType = "AllowNetbootTrue"
	ToggleNetbootFalse ConditionType = "AllowNetbootFalse"

	TemplateRenderedSuccess ConditionType = "TemplateRenderedSuccess"

	DisabledCondition ConditionType = "Disabled"

	BootModeNetboot    BootMode = "netboot"
	BootModeIsoboot    BootMode = "isoboot"
	BootModeCustomboot BootMode = "customboot"
)

type State int
type (
	ConditionType string
	BootMode      string
)

const (
	StatePreparing State = iota + 1
	StatePending
	StateRunning
	StatePost
	StateSuccess
	StateFailed
	StateTimeout

	ActionStatePending = StatePending
	ActionStateRunning = StateRunning
	ActionStateSuccess = StateSuccess
	ActionStateFailed  = StateFailed
	ActionStateTimeout = StateTimeout

	TaskStatePreparing = StatePreparing
	TaskStatePending   = StatePending
	TaskStateRunning   = StateRunning
	TaskStatePost      = StatePost
	TaskStateSuccess   = StateSuccess
	TaskStateFailed    = StateFailed
	TaskStateTimeout   = StateTimeout

	WorkflowStatePending = StatePending
	WorkflowStateRunning = StateRunning
	WorkflowStateSuccess = StateSuccess
	WorkflowStateFailed  = StateFailed
	WorkflowStateTimeout = StateTimeout
)

func (s State) String() string {
	switch s {
	case StatePreparing:
		return "Preparing"
	case StatePending:
		return "Pending"
	case StateRunning:
		return "Running"
	case StatePost:
		return "Post"
	case StateSuccess:
		return "Success"
	case StateFailed:
		return "Failed"
	case StateTimeout:
		return "Timeout"
	}
	return fmt.Sprintf("State(%d)", s)
}
