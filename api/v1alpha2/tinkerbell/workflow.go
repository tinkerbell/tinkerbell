package tinkerbell

import (
	"github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell/bmc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=workflows,scope=Namespaced,categories=tinkerbell,shortName=wf,singular=workflow
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".status.metadata.workflow.state",name="Workflow State",type=string
// +kubebuilder:printcolumn:JSONPath=".status.metadata.task.name",name="Task Name",type=string
// +kubebuilder:printcolumn:JSONPath=".status.metadata.action.name",name="Action Name",type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name="Age",type="date"
// +kubebuilder:printcolumn:JSONPath=".status.metadata.task.state",name="Task State",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.metadata.task.agentID",name="Task Agent",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.metadata.task.hardware",name="Task Hardware",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".status.conditions[?(@.type=='TemplateRenderedSuccess')].status",name="Task Rendering",type=string,priority=1
// +kubebuilder:printcolumn:JSONPath=".metadata.annotations['tinkerbell.org/disabled']",name="Disabled",type=string,priority=1

// Workflow is the Schema for the Workflow API.
// A Workflow orchestrates one or more Tasks with boot options, hardware references, and templating for provisioning Hardware.
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

	Items []Workflow `json:"items"`
}

// WorkflowSpec defines Tasks and associated options.
type WorkflowSpec struct {
	// Globals are extra configuration that is applied to all Tasks in the Workflow.
	Globals *Extra `json:"globals,omitempty"`

	// TimeoutSeconds is the duration before a Workflow time out is reached.
	// A zero or nil value means no timeout.
	// +optional
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`

	// Tasks that are run as part of the Workflow.
	Tasks []WorkflowTask `json:"tasks,omitempty"`
}

type Extra struct {
	// EnvVars defined here are additive to any existing environment variables.
	// +optional
	EnvVars []EnvVar `json:"envVars,omitempty"`

	// TemplateMap is a mapping of key/values that will be used when templating a Task.
	// +optional
	TemplateMap map[string]string `json:"templateMap,omitempty"`

	// Volumes defined here are additive to any existing volumes.
	// +optional
	Volumes []Volume `json:"volumes,omitempty"`
}

// WorkflowTask defines a Task entry within a Workflow, binding a Task definition to an Agent and Hardware.
type WorkflowTask struct {
	// AgentID is the ID of the Agent that is to run this Task.
	AgentID string `json:"agentID,omitempty"`

	// Extra configuration that is applied to this specific Task.
	// +optional
	Extra *Extra `json:"extra,omitempty"`

	// Hardware is the Hardware and options associated with this Task.
	// +optional
	Hardware *WorkflowHardware `json:"hardware,omitempty"`

	// TimeoutSeconds is the duration before a Task time out is reached.
	// A zero or nil value means no timeout.
	// +optional
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`

	// TaskRef is the Task associated with this Workflow.
	TaskRef SimpleReference `json:"taskRef,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="!(has(self.hardwareRef) && has(self.bmcRef))",message="hardwareRef and bmcRef are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="has(self.hardwareRef) || has(self.bmcRef)",message="at least one of hardwareRef or bmcRef must be specified"
type WorkflowHardware struct {
	// BootOptions are options that control the booting of Hardware.
	// These are only applicable when a HardwareRef or a BMCRef is provided.
	BootOptions BootOptions `json:"bootOptions,omitempty,omitzero"`

	// HardwareRef is the Hardware object associated with this Task.
	// This is used if the Task has templating that requires Hardware information.
	// When specified, the BootOptions will be applied using the BMCRef defined in the Hardware object.
	// Mutually exclusive with BMCRef.
	// +optional
	HardwareRef *SimpleReference `json:"hardwareRef,omitempty"`

	// BMCRef is the bmc.tinkerbell.org object associated with this Task.
	// When specified, the BootOptions will be applied using this bmc.tinkerbell.org object.
	// Mutually exclusive with HardwareRef.
	// +optional
	BMCRef *SimpleReference `json:"bmcRef,omitempty"`
}

// BootOptions are options that control the booting of Hardware.
type BootOptions struct {
	// ToggleNetboot indicates whether the the field in the associated Hardware for allowing network booting should
	// be enabled before the Task is executed and disabled after the Task has completed successfully.
	// A Hardware Reference must be provided.
	// +optional
	ToggleNetboot bool `json:"toggleNetboot,omitempty"`

	// ISOURL is the URL of the ISO that will be one-time booted.
	// When this field is set, a job.bmc.tinkerbell.org object will be created
	// for getting the associated Hardware into a CDROM booting state.
	// A Workflow Hardware Reference that contains a spec.reference.builtin.bmc must be provided.
	// BootMode must be set to "isoboot".
	// +optional
	// +kubebuilder:validation:Format=uri
	ISOURL string `json:"isoURL,omitempty"`

	// BootMode is the type of booting that will be done. One of "netboot", "isoboot", or "customboot".
	// +optional
	// +kubebuilder:validation:Enum=netboot;isoboot;customboot
	BootMode BootMode `json:"bootMode,omitempty"`

	// Customboot is the configuration for the "customboot" boot mode.
	// This allows users to define custom BMC Operations for pre and post a Task.
	Customboot *Customboot `json:"customboot,omitempty,omitzero"`
}

// Customboot defines the configuration for the customboot boot mode.
type Customboot struct {
	// PreOperations are the BMC operations that will be run before any Task Actions.
	// In most cases these operations should get a machine into a state where a Tink Agent is running.
	PreOperations []bmc.Operations `json:"preOperations,omitempty"`
	// PostOperations are the BMC operations that will be run after all Task Actions have completed.
	// In most cases these operations should get a machine into a state where it can be powered off or rebooted and remove any mounted virtual media.
	// These operations will be run only if all the associated Task Actions complete successfully.
	PostOperations []bmc.Operations `json:"postOperations,omitempty"`
}

func (b BootOptions) IsZero() bool {
	return b.ISOURL == "" && !b.ToggleNetboot && b.BootMode == "" && (b.Customboot == nil || b.Customboot.IsZero())
}

func (c Customboot) IsZero() bool {
	return len(c.PreOperations) == 0 && len(c.PostOperations) == 0
}
