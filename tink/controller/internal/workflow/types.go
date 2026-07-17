package workflow

// Workflow represents a workflow to be executed.
type Workflow struct {
	Version       string `yaml:"version"`
	Name          string `yaml:"name"`
	ID            string `yaml:"id"`
	GlobalTimeout int    `yaml:"global_timeout"`
	Tasks         []Task `yaml:"tasks"`
}

// Task represents a task to be executed as part of a workflow.
type Task struct {
	Name        string            `yaml:"name"`
	WorkerAddr  string            `yaml:"worker"`
	Actions     []Action          `yaml:"actions"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
}

// Action is the basic executional unit for a workflow.
type Action struct {
	Name        string            `yaml:"name"`
	Image       string            `yaml:"image"`
	Timeout     int64             `yaml:"timeout"`
	Command     []string          `yaml:"command,omitempty"`
	OnTimeout   []string          `yaml:"on-timeout,omitempty"`
	OnFailure   []string          `yaml:"on-failure,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Pid         string            `yaml:"pid,omitempty"`
	Namespaces  ActionNamespace   `yaml:"namespaces,omitempty"`
}

// ActionNamespace defines the Linux namespaces an action container runs in.
// This mirrors the v1alpha2 API spec.
type ActionNamespace struct {
	// Network is the network namespace the action container runs in. Passed
	// through to the container runtime as-is; set to "host" to share the host's
	// network namespace.
	Network string `yaml:"network,omitempty"`
	// PID is the PID namespace the action container runs in. Passed through to
	// the container runtime as-is; takes precedence over the deprecated
	// top-level pid field when set.
	PID string `yaml:"pid,omitempty"`
}
