package spec

import (
	"fmt"
	"time"
)

// Action holds the configuration used to create and run an Action container.
type Action struct {
	WorkerID   string `json:"worker_id" yaml:"worker_id"`
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
	Cmd string `json:"cmd,omitempty" yaml:"cmd,omitempty"`

	// Args are a set of arguments to be passed to the command executed by the container on
	// launch.
	// +optional
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`

	// Env defines environment variables that will be available inside an Action container.
	//+optional
	Env []Env `json:"env,omitempty" yaml:"env,omitempty"`

	// Volumes defines the volumes to mount into the container.
	// +optional
	Volumes []Volume `json:"volumes,omitempty" yaml:"volumes,omitempty"`

	// Namespaces defines the Linux namespaces this container should execute in.
	// +optional
	Namespaces     Namespaces `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
	Retries        int        `json:"retries" yaml:"retries"`
	TimeoutSeconds int        `json:"timeoutSeconds" yaml:"timeoutSeconds"`
	// StartedAt is the time the action was started.
	StartedAt time.Time `json:"created_at" yaml:"created_at"`
	// Duration is the time the action took to complete.
	Duration time.Duration `json:"duration" yaml:"duration"`
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
	Network string `json:"network,omitempty" yaml:"network,omitempty"`

	// PID defines the PID namespace
	// +optional
	PID string `json:"pid,omitempty" yaml:"pid,omitempty"`
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
