package spec

import (
	"fmt"
	"time"
)

// Action holds the configuration used to create and run an Action container.
type Action struct {
	AgentID    string `json:"agent_id" yaml:"agent_id"`
	TaskID     string `json:"task_id" yaml:"task_id"`
	WorkflowID string `json:"workflow_id" yaml:"workflow_id"`
	// ID is the unique identifier for the Action.
	ID string `json:"id" yaml:"id"`
	// Name is a name for the action.
	Name string `json:"name" yaml:"name"`

	// Image is an OCI image.
	Image string `json:"image" yaml:"image"`

	// Cmd defines the command to use when launching the image. It overrides the default command
	// of the action. It must be a unix path to an executable program.
	// +kubebuilder:validation:Pattern=`^(/[^/ ]*)+/?$`
	// +optional
	Cmd string `json:"cmd,omitempty,omitzero" yaml:"cmd,omitempty,omitzero"`

	// Args are a set of arguments to be passed to the command executed by the container on
	// launch.
	// +optional
	Args []string `json:"args,omitempty,omitzero" yaml:"args,omitempty,omitzero"`

	// Env defines environment variables that will be available inside an Action container.
	//+optional
	Env []Env `json:"env,omitempty,omitzero" yaml:"env,omitempty,omitzero"`

	// Volumes defines the volumes to mount into the container.
	// +optional
	Volumes []Volume `json:"volumes,omitempty,omitzero" yaml:"volumes,omitempty,omitzero"`

	// Namespaces defines the Linux namespaces this container should execute in.
	// +optional
	Namespaces     Namespaces `json:"namespaces,omitempty,omitzero" yaml:"namespaces,omitempty,omitzero"`
	Retries        int        `json:"retries,omitempty,omitzero" yaml:"retries,omitempty,omitzero"`
	TimeoutSeconds int        `json:"timeoutSeconds,omitempty,omitzero" yaml:"timeoutSeconds,omitempty,omitzero"`
	// ExecutionStart is the time the action started executing.
	ExecutionStart time.Time `json:"executionStart,omitzero" yaml:"executionStart,omitzero"`
	// ExecutionStop is the time the action stopped executing.
	ExecutionStop time.Time `json:"executionStop,omitzero" yaml:"executionStop,omitzero"`
	// ExecutionDuration is the time the action took to complete.
	ExecutionDuration string `json:"executionDuration,omitempty,omitzero" yaml:"duration,omitempty,omitzero"`
}

type Env struct {
	Key   string `json:"key" yaml:"key"`
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

type Event struct {
	Action  Action
	Message string
	State   State
}

type State string

const (
	StateSuccess State = "success"
	StateFailure State = "failure"
	StateRunning State = "running"
	StateTimeout State = "timeout"
	StateUnknown State = "unknown"
)

func (e Event) String() string {
	return fmt.Sprintf("action: %v, message: %v, state: %v", e.Action, e.Message, e.State)
}
