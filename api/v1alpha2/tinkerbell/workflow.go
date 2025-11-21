package tinkerbell

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=workflows,scope=Namespaced,categories=tinkerbell,shortName=wf,singular=workflow
// +kubebuilder:storageversion

// Workflow is the Schema for the Workflow API.
type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec WorkflowSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// WorkflowList contains a list of Workflows.
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Workflow `json:"items"`
}

// WorkflowSpec defines the desired state of Workflow.
type WorkflowSpec struct {
	// Actions that the Workflow runs.
	Actions []Action `json:"actions"`

	// Environment variables here are added to all Action in the Workflow.
	// +optional
	Environment []Environment `json:"environment,omitempty"`

	// Name is a human readable name for the Workflow.
	Name string `json:"name"`

	// Volumes defined here are added to all Actions in the Workfow.
	// +optional
	Volumes []Volume `json:"volumes,omitempty"`
}

// Action represents a Workflow Action.
type Action struct {
	// Name is a human readable name for the Action.
	Name string `json:"name" yaml:"name"`

	// Image is an OCI image. Should generally be a fully qualified OCI image reference.
	// For example, quay.io/tinkerbell/actions/image2disk:v1.0.0
	Image string `json:"image" yaml:"image"`

	// Command defines the command to use when launching the image. It overrides the default command
	// of the Action image. It must be a unix path to an executable program. When omitted, the image's
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
}

// Environment defines environmental variables.
type Environment struct {
	// Key is the name of the environmental variable.
	Key string `json:"key" yaml:"key"`
	// Value is the value of the environmental variable.
	Value string `json:"value" yaml:"value"`
}

// Volume is a specification for mounting a location on a Host into an Action container.
// Volumes take the form {SRC-VOLUME-NAME | SRC-HOST-DIR}:TGT-CONTAINER-DIR:OPTIONS.
// When specifying a VOLUME-NAME that does not exist it will be created for you.
// Examples:
//
// Read-only bind mount bound to /data
//
//	/etc/data:/data:ro
//
// Writable volume name bound to /data
//
//	shared_volume:/data
//
// See https://docs.docker.com/storage/volumes/ for additional details.
type Volume string

// Namespaces defines the Linux namespaces to use for the container.
// See https://man7.org/linux/man-pages/man7/namespaces.7.html.
type Namespaces struct {
	// Network defines the network namespace.
	// +optional
	Network string `json:"network,omitempty,omitzero" yaml:"network,omitempty,omitzero"`

	// PID defines the PID namespace
	// +optional
	PID string `json:"pid,omitempty,omitzero" yaml:"pid,omitempty,omitzero"`
}
