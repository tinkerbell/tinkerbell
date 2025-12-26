package tinkerbell

import (
	"github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell/bmc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=pipelines,scope=Namespaced,categories=tinkerbell,shortName=pl,singular=pipeline
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".status.metadata.pipeline.state",name="Pipeline State",type=string
// +kubebuilder:printcolumn:JSONPath=".status.metadata.workflow.name",name="Workflow Name",type=string
// +kubebuilder:printcolumn:JSONPath=".status.metadata.action.name",name="Action Name",type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name="Age",type="date"
// +kubebuilder:printcolumn:JSONPath=".status.metadata.workflow.state",name="Workflow State",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.metadata.workflow.agentID",name="Workflow Agent",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.metadata.workflow.hardware",name=Workflow Hardware,type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.workflowRendering",name="Workflow Rendering",type=string,priority=1

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

	Items []Pipeline `json:"items"`
}

// PipelineSpec defines Workflows and associated options.
type PipelineSpec struct {
	// Disabled indicates whether the Pipeline will be processed or not.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// Globals are extra configuration that is applied to all Workflows in the Pipeline.
	Globals *Extra `json:"globals,omitempty"`

	// TimeoutSeconds is the duration before a timed out is reached.
	// A zero value means no timeout.
	// +optional
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`

	// Workflows that are run as part of the Pipeline.
	Workflows []PipelineWorkflow `json:"workflows,omitempty"`
}

type Extra struct {
	// EnvVar variables here are additive to any existing environment variables.
	// +optional
	EnvVar []EnvVar `json:"envVars,omitempty"`

	// TemplateMap is a mapping of key/values that will be used when templating a Workflow.
	// +optional
	TemplateMap map[string]string `json:"templateMap,omitempty"`

	// Volumes defined here are additive to any existing volumes.
	// +optional
	Volumes []Volume `json:"volumes,omitempty"`
}

type PipelineWorkflow struct {
	// AgentID is the ID of the Agent that is to run this Workflow.
	AgentID string `json:"agentID,omitempty"`

	// Disabled indicates whether this Workflow will be processed or not.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// Extra configuration that is applied to this specific Workflow.
	// +optional
	Extra *Extra `json:"extra,omitempty"`

	// Hardware is the Hardware and options associated with this Workflow.
	// +optional
	Hardware *PipelineHardware `json:"hardware,omitempty"`

	// TimeoutSeconds is the duration before a timed out is reached.
	// A zero value means no timeout.
	// +optional
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`

	// WorkflowRef is the Workflow associated with this Pipeline config.
	WorkflowRef SimpleReference `json:"workflowRef,omitempty"`
}

type PipelineHardware struct {
	// BootOptions are options that control the booting of Hardware.
	// These are only applicable when a Reference is provided.
	BootOptions BootOptions `json:"bootOptions,omitempty,omitzero"`

	// HardwareRef is the Hardware object associated with this Workflow.
	HardwareRef SimpleReference `json:"hardwareRef,omitempty"`
}

// BootOptions are options that control the booting of Hardware.
type BootOptions struct {
	// ToggleNetboot indicates whether the the field in the associated Hardware for allowing network booting should
	// be enabled before the Workflow is executed and disabled after the Workflow has completed successfully.
	// A Hardware Reference must be provided.
	// +optional
	ToggleNetboot bool `json:"toggleNetboot,omitempty"`

	// ISOURL is the URL of the ISO that will be one-time booted.
	// When this field is set, a job.bmc.tinkerbell.org object will be created
	// for getting the associated Hardware into a CDROM booting state.
	// A Pipeline Hardware Reference that contains a spec.reference.builtin.bmc must be provided.
	// BootMode must be set to "isoboot".
	// +optional
	// +kubebuilder:validation:Format=url
	ISOURL string `json:"isoURL,omitempty"`

	// BootMode is the type of booting that will be done. One of "netboot", "isoboot", or "customboot".
	// +optional
	// +kubebuilder:validation:Enum=netboot;isoboot;customboot
	BootMode BootMode `json:"bootMode,omitempty"`

	// Customboot is the configuration for the "customboot" boot mode.
	// This allows users to define custom BMC Operations for pre and post a Workflow.
	Customboot *Customboot `json:"customboot,omitempty,omitzero"`
}

// Customboot defines the configuration for the customboot boot mode.
type Customboot struct {
	// PreOperations are the BMC Actions that will be run before any Workflow Actions.
	// In most cases these Actions should get a Machine into a state where a Tink Agent is running.
	PreOperations []bmc.Operations `json:"preOperations,omitempty"`
	// PostOperations are the BMC Actions that will be run after all Workflow Actions have completed.
	// In most cases these Actions should get a Machine into a state where it can be powered off or rebooted and remove any mounted virtual media.
	// These Actions will be run only if the main Workflow Actions complete successfully.
	PostOperations []bmc.Operations `json:"postOperations,omitempty"`
}

func (b BootOptions) IsZero() bool {
	return b.ISOURL == "" && !b.ToggleNetboot && b.BootMode == ""
}

func (c Customboot) IsZero() bool {
	return len(c.PreOperations) == 0 && len(c.PostOperations) == 0
}
