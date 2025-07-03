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
	// Name is a human readable name for the Workflow.
	Name string `json:"name"`

	// Actions that the Workflow runs.
	Actions []Action `json:"actions"`

	// Volumes defined here are added to all Actions in the Workfow.
	Volumes []Volume `json:"volumes,omitempty"`

	// Environment variables here are added to all Action in the Workflow.
	Environment []Environment `json:"environment,omitempty"`

	// TimeoutSeconds applied to the Workflow.
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`
}

// Action represents a workflow action.
type Action struct {
	// Name is a name for the action.
	Name string `json:"name" yaml:"name"`

	// Image is an OCI image.
	Image string `json:"image" yaml:"image"`

	// Cmd defines the command to use when launching the image. It overrides the default command
	// of the Action image. It must be a unix path to an executable program.
	// +kubebuilder:validation:Pattern=`^(/[^/ ]*)+/?$`
	// +optional
	Command string `json:"command,omitempty,omitzero" yaml:"command,omitempty,omitzero"`

	// Args are a set of arguments to be passed to the command executed by the container on
	// launch.
	// +optional
	Args []string `json:"args,omitempty,omitzero" yaml:"args,omitempty,omitzero"`

	// Environment defines environment variables that will be available inside an Action container.
	//+optional
	Environment []Environment `json:"environment,omitempty,omitzero" yaml:"environment,omitempty,omitzero"`

	// Volumes defines the volumes to mount into the container.
	// +optional
	Volumes []Volume `json:"volumes,omitempty,omitzero" yaml:"volumes,omitempty,omitzero"`

	// Namespaces defines the Linux namespaces this container should execute in.
	// +optional
	Namespaces Namespaces `json:"namespaces,omitempty,omitzero" yaml:"namespaces,omitempty,omitzero"`

	// Retries is the number of times the Action should be run when completed unsuccessfully.
	Retries int `json:"retries,omitempty,omitzero" yaml:"retries,omitempty,omitzero"`

	// TimeoutSeconds in seconds for this Action to complete.
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

// WorkflowStatus defines the observed state of Workflow.
type WorkflowStatus struct{}
