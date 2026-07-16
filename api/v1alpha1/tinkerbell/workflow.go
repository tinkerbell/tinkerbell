package tinkerbell

import (
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	corev1 "k8s.io/api/core/v1"
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
// +kubebuilder:resource:path=workflows,scope=Namespaced,categories=tinkerbell,shortName=wf,singular=workflow
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".spec.templateRef",name=Template,type=string
// +kubebuilder:printcolumn:JSONPath=".status.state",name=State,type=string
// +kubebuilder:printcolumn:JSONPath=".status.currentState.taskName",name=Task,type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.currentState.actionName",name=Action,type=string
// +kubebuilder:printcolumn:JSONPath=".status.currentState.agentID",name=Agent,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.hardwareRef",name=Hardware,type=string
// +kubebuilder:printcolumn:JSONPath=".status.templateRendering",name=Template-Rendering,type=string,priority=1

// Workflow is the Schema for the Workflows API.
type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowSpec   `json:"spec,omitempty"`
	Status WorkflowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkflowList contains a list of Workflows.
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workflow `json:"items"`
}

// WorkflowSpec defines the desired state of Workflow.
type WorkflowSpec struct {
	// Disabled indicates whether the Workflow will be processed or not.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`
	// Name of the Template associated with this workflow.
	TemplateRef string `json:"templateRef,omitempty"`

	// Name of the Hardware associated with this workflow.
	// +optional
	HardwareRef string `json:"hardwareRef,omitempty"`

	// A mapping of template devices to hardware mac addresses.
	HardwareMap map[string]string `json:"hardwareMap,omitempty"`

	// BootOptions are options that control the booting of Hardware.
	// These are only applicable when a HardwareRef is provided.
	BootOptions BootOptions `json:"bootOptions,omitempty,omitzero"`
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
	// +kubebuilder:validation:Format=uri
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
	// PreparingActions are the BMC actions and/or webhook calls that will be run, in order,
	// before any Workflow Actions. In most cases these should get a Machine into a state where
	// a Tink Agent is running. A pure-BMC-actions list (no webhook entries) behaves exactly as
	// it always has; a webhook entry interleaved among them makes an HTTP call instead of a
	// BMC action at that point in the sequence.
	// +kubebuilder:validation:MaxItems=50
	PreparingActions []CustombootAction `json:"preparingActions,omitempty"`

	// PostActions are the BMC actions and/or webhook calls that will be run, in order, after
	// all Workflow Actions have completed. In most cases these should get a Machine into a
	// state where it can be powered off or rebooted and remove any mounted virtual media.
	// These will be run only if the main Workflow Actions complete successfully.
	// +kubebuilder:validation:MaxItems=50
	PostActions []CustombootAction `json:"postActions,omitempty"`
}

// WebhookAction represents a single HTTP call made directly by the workflow controller.
type WebhookAction struct {
	// URL is the target of the HTTP request. Supports the same Go-template/sprig syntax
	// as bmc.VirtualMediaAction.MediaURL (see templateActions), e.g.
	// "https://inventory.example.com/hosts/{{ (index .Hardware.Interfaces 0).DHCP.MAC }}/provisioning-started".
	URL string `json:"url"`

	// Method defaults to POST if empty.
	// +kubebuilder:validation:Enum=GET;POST;PUT;PATCH;DELETE
	Method string `json:"method,omitempty"`

	// Headers are sent with the request, in order. Each is either a static (still templated)
	// Value or sourced directly from a Secret via ValueFrom.
	Headers []WebhookHeader `json:"headers,omitempty"`

	// BasicAuth, if set, adds an HTTP Basic Authorization header built from a Secret
	// containing "username" and "password" keys. Takes precedence over any Headers entry
	// named "Authorization".
	BasicAuth *corev1.SecretReference `json:"basicAuth,omitempty"`

	// Body is the request body, templated the same way as URL. Empty for GET/DELETE.
	Body string `json:"body,omitempty"`

	// Timeout in seconds for this call. Defaults to 30 if unset. Matches the existing
	// container-task Action.Timeout int64-seconds convention, not metav1.Duration.
	Timeout int64 `json:"timeout,omitempty"`

	// ExpectStatus, if set, is the exact HTTP status code that counts as success.
	// If unset, any 2xx response counts as success.
	ExpectStatus int `json:"expectStatus,omitempty"`
}

// WebhookHeader is one HTTP header for a WebhookAction. Exactly one of Value or ValueFrom
// must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.value) ? 1 : 0) + (has(self.valueFrom) ? 1 : 0) == 1",message="exactly one of value or valueFrom must be specified"
type WebhookHeader struct {
	Name string `json:"name"`

	// Value is a static header value, templated the same way as URL/Body.
	Value string `json:"value,omitempty"`

	// ValueFrom sources this header's value directly from a Secret key. Never templated —
	// unlike Value, it goes straight from Secret.Data[key] to the HTTP request, so a
	// credential can't leak into a body/URL that gets logged on failure.
	ValueFrom *corev1.SecretKeySelector `json:"valueFrom,omitempty"`
}

// CustombootAction is one element of PreparingActions/PostActions, executed in list order.
// bmc.Action is embedded (not nested under a "bmcAction" key) so a pure-BMC entry is written
// exactly as it always has been, e.g. "- powerAction: off" — only an entry that's a webhook call
// needs the extra Webhook field. Exactly one of bmc.Action's own fields or Webhook must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.powerAction) ? 1 : 0) + (has(self.oneTimeBootDeviceAction) ? 1 : 0) + (has(self.bootDevice) ? 1 : 0) + (has(self.virtualMediaAction) ? 1 : 0) + (has(self.webhook) ? 1 : 0) == 1",message="exactly one of powerAction, bootDevice, virtualMediaAction, oneTimeBootDeviceAction, or webhook must be specified"
type CustombootAction struct {
	bmc.Action `json:",inline"`

	// Webhook, if set, makes this entry an HTTP call instead of a BMC action.
	Webhook *WebhookAction `json:"webhook,omitempty"`
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
	// Actions tracks per-phase progress through PreparingActions/PostActions,
	// e.g. "customboot-preparing".
	Actions map[string]ActionListStatus `json:"actions,omitempty"`
}

// ActionListStatus holds progress through an ordered PreparingActions/PostActions list.
type ActionListStatus struct {
	// Completed is the number of entries in the list that have fully succeeded, in order.
	// The entry at index Completed is the one currently in flight (if any). The batch Job
	// name for an in-flight BMC-action run is always derivable as "<phase>-<workflow>-
	// <Completed>" — it isn't tracked separately here.
	Completed int `json:"completed,omitempty"`
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

// WorkflowStatus defines the observed state of a Workflow.
type WorkflowStatus struct {
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
	Conditions []WorkflowCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
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
type WorkflowCondition struct {
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
type Action struct {
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
func (w *WorkflowStatus) HasCondition(wct WorkflowConditionType, cs metav1.ConditionStatus) bool {
	for _, c := range w.Conditions {
		if c.Type == wct {
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
