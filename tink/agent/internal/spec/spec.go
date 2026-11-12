package spec

import (
	"fmt"
	"time"
)

// Action holds the configuration used to create and run an Action container.
//
// The yaml tags deliberately omit the omitzero flag that the json tags carry: the file and NATS
// transports decode this struct with gopkg.in/yaml.v3, which panics on flags it does not know, and
// omitzero is an encoding/json feature only.
type Action struct {
	AgentID    string `json:"agent_id" yaml:"agent_id"`
	TaskID     string `json:"task_id" yaml:"task_id"`
	WorkflowID string `json:"workflow_id" yaml:"workflow_id"`
	// ID is the unique identifier for the Action.
	ID string `json:"id" yaml:"id"`
	// Name is a name for the action.
	Name string `json:"name" yaml:"name"`

	// Image is an OCI image. Mutually exclusive with Run.
	// +optional
	Image string `json:"image,omitempty,omitzero" yaml:"image,omitempty"`

	// Run is an inline script executed directly on the host the Agent runs on, outside any
	// container runtime. The Agent writes the contents to a file and invokes it with Shell.
	// Mutually exclusive with Image.
	// +optional
	Run string `json:"run,omitempty,omitzero" yaml:"run,omitempty"`

	// Shell is the interpreter argv used to execute Run, e.g. ["python3", "-u"].
	// Defaults to ["bash", "-x", "-e"]. Only valid together with Run.
	// +optional
	Shell []string `json:"shell,omitempty,omitzero" yaml:"shell,omitempty"`

	// Background reports the Action successful before executing it, then runs it detached in
	// the background with no timeout. Useful for Actions that terminate the Agent itself, such
	// as kexec, reboot, or power off.
	// +optional
	Background bool `json:"background,omitempty,omitzero" yaml:"background,omitempty"`

	// Cmd defines the command to use when launching the image. It overrides the default command
	// of the action. It must be a unix path to an executable program.
	// +kubebuilder:validation:Pattern=`^(/[^/ ]*)+/?$`
	// +optional
	Cmd string `json:"cmd,omitempty,omitzero" yaml:"cmd,omitempty"`

	// Args are a set of arguments to be passed to the command executed by the container on
	// launch.
	// +optional
	Args []string `json:"args,omitempty,omitzero" yaml:"args,omitempty"`

	// Env defines environment variables that will be available inside an Action container.
	//+optional
	Env []Env `json:"env,omitempty,omitzero" yaml:"env,omitempty"`

	// Volumes defines the volumes to mount into the container.
	// +optional
	Volumes []Volume `json:"volumes,omitempty,omitzero" yaml:"volumes,omitempty"`

	// Namespaces defines the Linux namespaces this container should execute in.
	// +optional
	Namespaces     Namespaces `json:"namespaces,omitempty,omitzero" yaml:"namespaces,omitempty"`
	Retries        int        `json:"retries,omitempty,omitzero" yaml:"retries,omitempty"`
	TimeoutSeconds int        `json:"timeoutSeconds,omitempty,omitzero" yaml:"timeoutSeconds,omitempty"`
	// ExecutionStart is the time the action started executing.
	ExecutionStart time.Time `json:"executionStart,omitzero" yaml:"executionStart"`
	// ExecutionStop is the time the action stopped executing.
	ExecutionStop time.Time `json:"executionStop,omitzero" yaml:"executionStop"`
	// ExecutionDuration is the time the action took to complete.
	ExecutionDuration string `json:"executionDuration,omitempty,omitzero" yaml:"duration,omitempty"`
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
	Network string `json:"network,omitempty,omitzero" yaml:"network,omitempty"`

	// PID defines the PID namespace
	// +optional
	PID string `json:"pid,omitempty,omitzero" yaml:"pid,omitempty"`
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
