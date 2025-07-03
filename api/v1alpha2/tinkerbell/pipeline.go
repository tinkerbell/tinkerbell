package tinkerbell

import (
	"github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell/bmc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type (
	WorkflowState         string
	WorkflowConditionType string
	TemplateRendering     string
	BootMode              string
)

const (
	WorkflowStatePreparing = WorkflowState("PREPARING")
	WorkflowStatePending   = WorkflowState("PENDING")
	WorkflowStateRunning   = WorkflowState("RUNNING")
	WorkflowStatePost      = WorkflowState("POST")
	WorkflowStateSuccess   = WorkflowState("SUCCESS")
	WorkflowStateFailed    = WorkflowState("FAILED")
	WorkflowStateTimeout   = WorkflowState("TIMEOUT")

	BootJobFailed           WorkflowConditionType = "BootJobFailed"
	BootJobComplete         WorkflowConditionType = "BootJobComplete"
	BootJobRunning          WorkflowConditionType = "BootJobRunning"
	BootJobSetupFailed      WorkflowConditionType = "BootJobSetupFailed"
	BootJobSetupComplete    WorkflowConditionType = "BootJobSetupComplete"
	ToggleAllowNetbootTrue  WorkflowConditionType = "AllowNetbootTrue"
	ToggleAllowNetbootFalse WorkflowConditionType = "AllowNetbootFalse"
	TemplateRenderedSuccess WorkflowConditionType = "TemplateRenderedSuccess"

	TemplateRenderingSuccessful TemplateRendering = "successful"
	TemplateRenderingFailed     TemplateRendering = "failed"

	BootModeNetboot    BootMode = "netboot"
	BootModeISO        BootMode = "iso"
	BootModeIsoboot    BootMode = "isoboot"
	BootModeCustomboot BootMode = "customboot"
)

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=pipelines,scope=Namespaced,categories=tinkerbell,shortName=pl,singular=pipeline
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".spec.templateRef",name=Template,type=string
// +kubebuilder:printcolumn:JSONPath=".status.state",name=State,type=string
// +kubebuilder:printcolumn:JSONPath=".status.currentState.taskName",name=Task,type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.currentState.actionName",name=Action,type=string
// +kubebuilder:printcolumn:JSONPath=".status.currentState.agentID",name=Agent,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.hardwareRef",name=Hardware,type=string
// +kubebuilder:printcolumn:JSONPath=".status.templateRendering",name=Template-Rendering,type=string,priority=1

// Pipeline is the Schema for the Pipeline API.
type Pipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PipelineSpec   `json:"spec,omitempty"`
	Status PipelineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PipelineList contains a list of Pipelines.
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pipeline `json:"items"`
}

// PipelineSpec defines Workflows and associated options.
type PipelineSpec struct {
	// Disabled indicates whether the Pipeline will be processed or not.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// Globals apply to all Workflows in the Pipeline.
	Globals *Globals `json:"globals,omitempty"`

	// Workflows that are run as part of the Pipeline.
	Workflows []WorkflowConfig `json:"workflows,omitempty"`
}

type Globals struct {
	// Environment variables here are added to all Workflows in the Pipeline.
	Environment []Environment `json:"environment,omitempty"`

	// TemplateMap is a mapping of key/values that will be used when templating a Workflow.
	TemplateMap map[string]string `json:"templateMap,omitempty"`

	// TimeoutSeconds is the duration for the Pipeline before marking it as Timed out.
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`

	// Volumes defined here are added to all Workflows in the Pipeline.
	Volumes []Volume `json:"volumes,omitempty"`
}

type WorkflowConfig struct {
	// AgentID is the ID of the Agent that is to run this Workflow.
	AgentID string `json:"agentID,omitempty"`

	// Disabled indicates whether this Workflow will be processed or not.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// HardwareRef is the Hardware and options associated with this Workflow.
	// +optional
	Hardware *HardwareRef `json:"hardware,omitempty"`

	// TemplateMap is a mapping of key/values that will be used when templating a Workflow.
	TemplateMap map[string]string `json:"templateMap,omitempty"`

	// WorkflowRef is the Workflow associated with this Pipeline config.
	WorkflowRef SimpleReference `json:"workflowRef,omitempty"`
}

type HardwareRef struct {
	// BootOptions are options that control the booting of Hardware.
	// These are only applicable when a HardwareRef is provided.
	BootOptions BootOptions `json:"bootOptions,omitempty,omitzero"`

	// Reference is the Hardware object associated with this Workflow.
	Reference SimpleReference `json:"reference,omitempty"`
}

// BootOptions are options that control the booting of Hardware.
type BootOptions struct {
	// ToggleAllowNetboot indicates whether the controller should toggle the field in the associated hardware for allowing PXE booting.
	// This will be enabled before a Workflow is executed and disabled after the Workflow has completed successfully.
	// A HardwareRef must be provided.
	// +optional
	ToggleAllowNetboot bool `json:"toggleAllowNetboot,omitempty"`

	// ISOURL is the URL of the ISO that will be one-time booted. When this field is set,
	// the controller will create a job.bmc.tinkerbell.org object
	// for getting the associated hardware into a CDROM booting state.
	// A HardwareRef that contains a spec.BmcRef must be provided.
	// BootMode must be set to "isoboot".
	// +optional
	// +kubebuilder:validation:Format=url
	ISOURL string `json:"isoURL,omitempty"`

	// BootMode is the type of booting that will be done. One of "netboot", "isoboot", or "customboot".
	// +optional
	// +kubebuilder:validation:Enum=netboot;isoboot;iso;customboot
	BootMode BootMode `json:"bootMode,omitempty"`

	// CustombootConfig is the configuration for the "customboot" boot mode.
	// This allows users to define custom BMC Actions.
	CustombootConfig CustombootConfig `json:"custombootConfig,omitempty,omitzero"`
}

// CustombootConfig defines the configuration for the customboot boot mode.
type CustombootConfig struct {
	// PreparingActions are the BMC Actions that will be run before any Workflow Actions.
	// In most cases these Actions should get a Machine into a state where a Tink Agent is running.
	PreparingActions []bmc.Operations `json:"preparingActions,omitempty"`
	// PostActions are the BMC Actions that will be run after all Workflow Actions have completed.
	// In most cases these Actions should get a Machine into a state where it can be powered off or rebooted and remove any mounted virtual media.
	// These Actions will be run only if the main Workflow Actions complete successfully.
	PostActions []bmc.Operations `json:"postActions,omitempty"`
}

func (b BootOptions) IsZero() bool {
	return b.ISOURL == "" && !b.ToggleAllowNetboot && b.BootMode == ""
}

func (c CustombootConfig) IsZero() bool {
	return len(c.PreparingActions) == 0 && len(c.PostActions) == 0
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

type CurrentState struct {
	AgentID    string        `json:"agentID,omitempty"`
	TaskID     string        `json:"taskID,omitempty"`
	ActionID   string        `json:"actionID,omitempty"`
	State      WorkflowState `json:"state,omitempty"`
	ActionName string        `json:"actionName,omitempty"`
	TaskName   string        `json:"taskName,omitempty"`
}

// PipelineStatus defines the observed state of a Workflow.
type PipelineStatus struct {
	// AgentID is the ID of the Agent with which this Workflow is associated.
	AgentID string `json:"agentID,omitempty"`

	// State is the current overall state of the Workflow.
	State WorkflowState `json:"state,omitempty"`

	// BootOptions holds the state of any boot options.
	BootOptions BootOptionsStatus `json:"bootOptions,omitempty"`

	// TemplateRendering indicates whether the template was rendered successfully.
	// Possible values are "successful" or "failed" or "unknown".
	TemplateRendering TemplateRendering `json:"templateRendering,omitempty"`

	// GlobalTimeout represents the max execution time.
	GlobalTimeout int64 `json:"globalTimeout,omitempty"`

	// GlobalExecutionStop represents the time when the Workflow should stop executing.
	// After this time, the Workflow will be marked as TIMEOUT.
	GlobalExecutionStop *metav1.Time `json:"globalExecutionStop,omitempty"`

	// CurrentState tracks where the workflow is in its execution.
	CurrentState *CurrentState `json:"currentState,omitempty"`

	// Tasks are the tasks to be run by the Agent(s).
	Tasks []Task `json:"tasks,omitempty"`

	// Conditions are the latest available observations of an object's current state.
	//
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=atomic
	Conditions []PipelineCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
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

// Task represents a series of actions to be completed by an Agent.
type Task struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	AgentID     string            `json:"agentID"`
	Actions     []Action          `json:"actions"`
	Volumes     []string          `json:"volumes,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

// Action represents a workflow action.
type Action2 struct {
	ID                string            `json:"id"`
	Name              string            `json:"name,omitempty"`
	Image             string            `json:"image,omitempty"`
	Timeout           int64             `json:"timeout,omitempty"`
	Command           []string          `json:"command,omitempty"`
	Volumes           []string          `json:"volumes,omitempty"`
	Pid               string            `json:"pid,omitempty"`
	Environment       map[string]string `json:"environment,omitempty"`
	State             WorkflowState     `json:"state,omitempty"`
	ExecutionStart    *metav1.Time      `json:"executionStart,omitempty"`
	ExecutionStop     *metav1.Time      `json:"executionStop,omitempty"`
	ExecutionDuration string            `json:"executionDuration,omitempty"`
	Message           string            `json:"message,omitempty"`
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
