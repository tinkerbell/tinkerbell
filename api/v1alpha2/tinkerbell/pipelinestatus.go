package tinkerbell

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// PipelineStatus defines the observed state of a Pipeline.
type PipelineStatus struct {
	// BootOptions holds the state of any boot options.
	BootOptions BootOptionsStatus `json:"bootOptions,omitempty"`

	// Metadata tracks where the Pipeline is in its execution.
	// It holds the current state of the Pipeline as a whole,
	// the current Workflow, and the current Action.
	Metadata PipelineMetadata `json:"metadata,omitempty"`

	// WorkflowRendering indicates whether the Workflows were all rendered successfully.
	// Possible values are "successful" or "failed" or "unknown".
	WorkflowRendering WorkflowRendering `json:"workflowRendering,omitempty"`

	// GlobalTimeout represents the max execution duration time.
	GlobalTimeout int64 `json:"globalTimeout,omitempty"`

	// GlobalExecutionStop represents the time when the Pipeline should stop executing.
	// After this time, if the Pipeline has not completed it will be marked as timed out.
	GlobalExecutionStop *metav1.Time `json:"globalExecutionStop,omitempty"`

	// RenderedPipeline are the Workflows to be run by this Pipeline.
	RenderedPipeline []WorkflowWithMetadata `json:"renderedPipeline,omitempty"`

	// Conditions are the latest available observations of an object's current state.
	//
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=atomic
	Conditions []PipelineCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// WorkflowWithMetadata is a copy of WorkflowSpec with the addition of Metadata.
// Users don't need Metadata in the CRD Spec so we don't include it but we need
// it in the CRD Status.
type WorkflowWithMetadata struct {
	// Actions that the Workflow runs.
	Actions []ActionWithMetadata `json:"actions"`

	// Environment variables here are added to all Action in the Workflow.
	// +optional
	Environment []Environment `json:"environment,omitempty"`

	// Name is a human readable name for the Workflow.
	Name string `json:"name"`

	// Volumes defined here are added to all Actions in the Workfow.
	// +optional
	Volumes []Volume `json:"volumes,omitempty"`

	Metadata Metadata `json:"workflowMetadata,omitempty"`
}

// ActionWithMetadata
type ActionWithMetadata struct {
	// Name is a human readable name for the Action.
	Name string `json:"name" yaml:"name"`

	// Image is an OCI image. Should generally be a fully qualified OCI image reference.
	// For example, quay.io/tinkerbell/actions/image2disk:v1.0.0
	Image string `json:"image" yaml:"image"`

	// Command defines the command to use when launching the image. It overrides the default command
	// of the Action image. It must be a unix path to an executable program. When omited, the image's
	// default command/entrypoint is used.
	// +kubebuilder:validation:Pattern=`^(/[^/ ]*)+/?$`
	// +optional
	Command string `json:"command,omitempty,omitzero" yaml:"command,omitempty,omitzero"`

	// Args are a set of arguments to be passed to the command executed by the container on launch.
	// +optional
	Args []string `json:"args,omitempty,omitzero" yaml:"args,omitempty,omitzero"`

	// Environment defines environment variables that will be available inside a container.
	//+optional
	Environment []Environment `json:"environment,omitempty,omitzero" yaml:"environment,omitempty,omitzero"`

	// Volumes defines the volumes that will be mounted into the container.
	// +optional
	Volumes []Volume `json:"volumes,omitempty,omitzero" yaml:"volumes,omitempty,omitzero"`

	// Namespaces defines the Linux namespaces with which the container should configured.
	// +optional
	Namespaces Namespaces `json:"namespaces,omitempty,omitzero" yaml:"namespaces,omitempty,omitzero"`

	// Retries is the number of times the Action should be run until completed successfully.
	Retries int `json:"retries,omitempty,omitzero" yaml:"retries,omitempty,omitzero"`

	// TimeoutSeconds is the total number of seconds the Action is allowed to run without completing
	// before marking it as timed out.
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty,omitzero" yaml:"timeoutSeconds,omitempty,omitzero"`

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

type PipelineMetadata struct {
	Pipeline Metadata `json:"pipeline,omitempty"`
	Workflow Metadata `json:"workflow,omitempty"`
	Action   Metadata `json:"action,omitempty"`
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

// JobCondition describes current state of a job.
type PipelineCondition struct {
	// Type of job condition, Complete or Failed.
	Type WorkflowConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=WorkflowConditionType"`
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

// HasCondition checks if the cType condition is present with status cStatus on a bmj.
func (w *PipelineStatus) HasCondition(wct WorkflowConditionType, cs metav1.ConditionStatus) bool {
	for _, c := range w.Conditions {
		if c.Type == wct {
			return c.Status == cs
		}
	}

	return false
}

// SetCondition updates conditions. If the condition already exists, it updates it.
// If the condition doesn't exist then it appends the new one (wc).
func (w *PipelineStatus) SetCondition(wc PipelineCondition) {
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
func (w *PipelineStatus) SetConditionIfDifferent(wc PipelineCondition) {
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
	BootJobFailed        WorkflowConditionType = "BootJobFailed"
	BootJobComplete      WorkflowConditionType = "BootJobComplete"
	BootJobRunning       WorkflowConditionType = "BootJobRunning"
	BootJobSetupFailed   WorkflowConditionType = "BootJobSetupFailed"
	BootJobSetupComplete WorkflowConditionType = "BootJobSetupComplete"

	ToggleNetbootTrue  WorkflowConditionType = "AllowNetbootTrue"
	ToggleNetbootFalse WorkflowConditionType = "AllowNetbootFalse"

	TemplateRenderedSuccess WorkflowConditionType = "TemplateRenderedSuccess"

	TemplateRenderingSuccessful WorkflowRendering = "successful"
	TemplateRenderingFailed     WorkflowRendering = "failed"

	BootModeNetboot    BootMode = "netboot"
	BootModeIsoboot    BootMode = "isoboot"
	BootModeCustomboot BootMode = "customboot"
)

type State int
type (
	WorkflowConditionType string
	WorkflowRendering     string
	BootMode              string
)

const (
	StatePreparing State = iota
	StatePending
	StateRunning
	StatePost
	StateSuccess
	StateFailed
	StateTimeout
	StateUndefined

	ActionStatePending = StatePending
	ActionStateRunning = StateRunning
	ActionStateSuccess = StateSuccess
	ActionStateFailed  = StateFailed
	ActionStateTimeout = StateTimeout

	WorkflowStatePreparing = StatePreparing
	WorkflowStatePending   = StatePending
	WorkflowStateRunning   = StateRunning
	WorkflowStatePost      = StatePost
	WorkflowStateSuccess   = StateSuccess
	WorkflowStateFailed    = StateFailed
	WorkflowStateTimeout   = StateTimeout

	PipelineStatePending = StatePending
	PipelineStateRunning = StateRunning
	PipelineStateSuccess = StateSuccess
	PipelineStateFailed  = StateFailed
	PipelineStateTimeout = StateTimeout
)

func (s State) String() string {
	switch s {
	case 0:
		return "Preparing"
	case 1:
		return "Pending"
	case 2:
		return "Running"
	case 3:
		return "Post"
	case 4:
		return "Success"
	case 5:
		return "Failed"
	case 6:
		return "Timeout"
	}
	return fmt.Sprintf("State(%d)", s)
}

func (c State) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

type Metadata struct {
	AgentID string `json:"agentID,omitempty"`

	ID types.UID `json:"id,omitempty"`

	Name string `json:"name,omitempty"`

	Hardware string `json:"hardware,omitempty"`

	State State `json:"state"`

	StartTime *metav1.Time `json:"startTime,omitempty"`

	EndTime *metav1.Time `json:"endTime,omitempty"`

	ExecutionDuration string `json:"executionDuration,omitempty"`
}
