package agent

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/attribute"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/runtime/containerd"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/runtime/docker"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/runtime/kubernetes"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/runtime/script"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/transport/file"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/transport/grpc"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/transport/nats"
	"golang.org/x/sync/errgroup"
)

// TransportReader provides a method to read an action.
type TransportReader interface {
	// Read blocks until an action is available or an error occurs
	Read(ctx context.Context) (spec.Action, error)
}

// RuntimeExecutor provides a method to execute an action.
type RuntimeExecutor interface {
	// Execute blocks until the action is completed or an error occurs
	Execute(ctx context.Context, action spec.Action) error
}

// TransportWriter provides a method to write an event.
type TransportWriter interface {
	// Write blocks until the event is written or an error occurs
	Write(ctx context.Context, event spec.Event) error
}

type Config struct {
	TransportReader TransportReader
	RuntimeExecutor RuntimeExecutor
	// ScriptExecutor runs actions that carry an inline script instead of an image. It is
	// selected per action, independently of which container runtime RuntimeExecutor is.
	ScriptExecutor  RuntimeExecutor
	TransportWriter TransportWriter
	Retry           Retry
}

func (c *Config) Run(ctx context.Context, log logr.Logger) {
	// All steps are synchronous and blocking
	// 1. get an action from the input transport
	// 2. send running/starting event to the output transport
	// 3. send the action to the runtime for execution
	// 4. send the result event to the output transport
	// 5. go to step 1
	rt := newRetrier(c.Retry)
	for {
		if ctx.Err() != nil {
			return
		}
		action, err := c.TransportReader.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			d := rt.next(err)
			if !isNoAction(err) {
				log.Info("error reading/retrieving action", "error", err, "retryIn", d.String())
			}
			if !sleep(ctx, d) {
				return
			}
			continue
		}

		log.Info("received action", "action", action)
		if err := c.TransportWriter.Write(ctx, spec.Event{Action: action, Message: "running action", State: spec.StateRunning}); err != nil {
			if ctx.Err() != nil {
				return
			}
			// The server re-serves the Action on the next read, so there is nothing to lose by going back to polling.
			d := rt.next(err)
			log.Info("error writing event", "error", err, "retryIn", d.String())
			if !sleep(ctx, d) {
				return
			}
			continue
		}
		rt.next(nil)
		log.Info("reported action status", "action", action, "state", spec.StateRunning)

		// An action carrying an inline script runs directly on this host; everything else goes to
		// the configured container runtime. Dispatching per action, rather than per agent, lets a
		// single workflow mix the two.
		executor := c.RuntimeExecutor
		if action.Run != "" {
			executor = c.ScriptExecutor
		}
		if executor == nil {
			// Only reachable if the agent was assembled without the executor this action needs.
			// Report the failure rather than panicking, so the workflow fails visibly instead of
			// taking the agent down and stalling.
			log.Info("no executor available for action", "action", action)
			action.ExecutionStart = time.Now().UTC()
			action.ExecutionStop = action.ExecutionStart
			if err := c.report(ctx, log, rt, spec.Event{
				Action:  action,
				Message: "no executor available for this action",
				State:   spec.StateFailure,
			}); err != nil {
				if ctx.Err() != nil {
					return
				}
				d := rt.next(err)
				log.Info("action report rejected, returning to polling", "error", err, "retryIn", d.String())
				if !sleep(ctx, d) {
					return
				}
			}
			continue
		}

		// A background action is reported successful before it runs, then detached. This is what
		// makes actions that take the agent down with them - reboot, kexec, power off - usable:
		// the workflow reaches a completed state first, instead of hanging on a report that will
		// never arrive. The trade-off is that the action's outcome is invisible to the workflow.
		if action.Background {
			// The action is reported as taking no time, because from the workflow's point of
			// view it did not run at all: it was handed off, not executed.
			action.ExecutionStart = time.Now().UTC()
			action.ExecutionStop = action.ExecutionStart
			if err := c.report(ctx, log, rt, spec.Event{
				Action:  action,
				Message: "action started in the background",
				State:   spec.StateSuccess,
			}); err != nil {
				if ctx.Err() != nil {
					return
				}
				d := rt.next(err)
				log.Info("action report rejected, returning to polling", "error", err, "retryIn", d.String())
				if !sleep(ctx, d) {
					return
				}
				continue
			}
			log.Info("reported action status", "action", action, "state", spec.StateSuccess)

			// Detach from the loop's context: the point of a background action is to outlive the
			// agent's attention to it, and no timeout applies.
			bgCtx := context.WithoutCancel(ctx)
			bgAction := action
			bgExecutor := executor
			go func() {
				if err := bgExecutor.Execute(bgCtx, bgAction); err != nil {
					log.Info("background action failed", "action", bgAction, "error", err)
					return
				}
				log.Info("background action completed", "action", bgAction)
			}()

			continue
		}

		state := spec.StateSuccess
		// TODO(jacobweinstock): Add a retry count that comes from a CLI flag. It should only take precedence if the action has a retry count of 0.
		retries := ternary(action.Retries == 0, 1, action.Retries)

		responseEvent := spec.Event{}
		action.ExecutionStart = time.Now().UTC()
		// A zero timeout means no timeout. Wrapping unconditionally would build an
		// already-expired context, failing every action that omits the field.
		timeoutCtx := ctx
		timeoutDone := func() {}
		if action.TimeoutSeconds > 0 {
			timeoutCtx, timeoutDone = context.WithTimeout(ctx, time.Duration(action.TimeoutSeconds)*time.Second)
		}
		for i := 1; i <= retries; i++ {
			if err := executor.Execute(timeoutCtx, action); err != nil {
				log.Info("error executing action", "error", err, "maxRetries", retries, "currentTry", i)
				state = spec.StateFailure
				if errors.Is(err, context.DeadlineExceeded) {
					state = spec.StateTimeout
					timeoutDone()
					break
				}
				if i == retries {
					timeoutDone()
					break
				}
				continue
			}
			state = spec.StateSuccess
			log.Info("executed action", "action", action)
			timeoutDone()
			break
		}
		timeoutDone()

		action.ExecutionStop = time.Now().UTC()
		action.ExecutionDuration = humanDuration(action.ExecutionStop.Sub(action.ExecutionStart), 2)
		responseEvent.Action = action
		responseEvent.Message = "action completed"
		responseEvent.State = state

		if err := c.report(ctx, log, rt, responseEvent); err != nil {
			if ctx.Err() != nil {
				return
			}
			d := rt.next(err)
			log.Info("action report rejected, returning to polling", "error", err, "retryIn", d.String())
			if !sleep(ctx, d) {
				return
			}
			continue
		}
		log.Info("reported action status", "action", action, "state", state)
	}
}

// report sends the Action's final status until it is accepted, rejected as permanent, or ctx is done.
// The server will not re-serve an Action once the Agent has moved past execution, so
// giving up on a transient error would leave the Workflow stuck.
func (c *Config) report(ctx context.Context, log logr.Logger, rt *retrier, event spec.Event) error {
	for {
		err := c.TransportWriter.Write(ctx, event)
		if err == nil {
			rt.next(nil)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if isPermanent(err) {
			return err
		}
		d := rt.next(err)
		log.Info("error reporting action, retrying", "error", err, "retryIn", d.String())
		if !sleep(ctx, d) {
			return ctx.Err()
		}
	}
}

func ternary[T any](condition bool, valueIfTrue, valueIfFalse T) T {
	if condition {
		return valueIfTrue
	}
	return valueIfFalse
}

const (
	GRPCTransportType TransportType = "grpc"
	FileTransportType TransportType = "file"
	NATSTransportType TransportType = "nats"

	DockerRuntimeType     RuntimeType = "docker"
	ContainerdRuntimeType RuntimeType = "containerd"
	KubernetesRuntimeType RuntimeType = "kubernetes"
)

type TransportType string

type RuntimeType string

type Options struct {
	Transport                 Transport
	Runtime                   Runtime
	Registry                  Registry
	Proxy                     Proxy
	TransportSelected         TransportType
	RuntimeSelected           RuntimeType
	AttributeDetectionEnabled bool
	// LLDPTimeout bounds how long attribute detection waits for LLDP
	// neighbors. Detection finishes before any Action runs because an Action
	// can power-cycle the machine. Zero disables LLDP discovery.
	LLDPTimeout time.Duration
	Retry       Retry
}

type Transport struct {
	GRPC GRPCTransport
	File FileTransport
	NATS NATSTransport
}

type Runtime struct {
	Docker     DockerRuntime
	Containerd ContainerdRuntime
	Kubernetes KubernetesRuntime
}

type Registry struct {
	Name string
	User string
	Pass string
}

type Proxy struct {
	HTTPProxy  []string
	HTTPSProxy []string
	NoProxy    []string
}

type GRPCTransport struct {
	ServerAddrPort string
	TLSEnabled     bool
	TLSInsecure    bool
}
type FileTransport struct {
	WorkflowPath string
}
type NATSTransport struct {
	ServerAddrPort netip.AddrPort
	StreamName     string
	EventsSubject  string
	ActionsSubject string
}

type DockerRuntime struct {
	SocketPath string
}
type ContainerdRuntime struct {
	Namespace  string
	SocketPath string
	DataRoot   string
	// RegistryConfigPath is the root directory containing per-registry hosts.toml and certificate configuration.
	RegistryConfigPath string
}
type KubernetesRuntime struct {
	Namespace          string
	Kubeconfig         string
	ServiceAccountName string
}

func (o *Options) ConfigureAndRun(inctx context.Context, log logr.Logger, id string) error {
	// instantiate the implementation for the transport reader
	// instantiate the implementation for the transport writer
	// instantiate the implementation for the runtime executor
	// instantiate the agent
	// run the agent
	var tr TransportReader
	var tw TransportWriter
	// Held back until the runtime executor is built: starting it here would leave
	// the transport connected when a runtime error returns without waiting.
	var startTransport func(context.Context) error
	switch o.TransportSelected {
	case FileTransportType:
		readWriter := &file.Config{
			Log:     log,
			Actions: make(chan spec.Action),
			FileLoc: "./example/file_template.yaml",
		}
		startTransport = readWriter.Start
		tr = readWriter
		tw = readWriter
	case NATSTransportType:
		readWriter := &nats.Config{
			StreamName:     o.Transport.NATS.StreamName,
			EventsSubject:  o.Transport.NATS.EventsSubject,
			ActionsSubject: o.Transport.NATS.ActionsSubject,
			IPPort:         o.Transport.NATS.ServerAddrPort,
			Log:            log,
			AgentID:        id,
			Actions:        make(chan spec.Action),
		}
		log.Info("starting NATS transport", "server", o.Transport.NATS.ServerAddrPort)
		startTransport = readWriter.Start
		tr = readWriter
		tw = readWriter
	default:
		conn, err := grpc.NewClientConn(o.Transport.GRPC.ServerAddrPort, o.Transport.GRPC.TLSEnabled, o.Transport.GRPC.TLSInsecure)
		if err != nil {
			return fmt.Errorf("unable to create gRPC client: %w", err)
		}
		readWriter := &grpc.Config{
			Log:              log,
			TinkServerClient: proto.NewWorkflowServiceClient(conn),
			AgentID:          id,
			Actions:          make(chan spec.Action),
		}
		if o.AttributeDetectionEnabled {
			readWriter.Attributes = attribute.DiscoverAll(inctx, log, o.LLDPTimeout)
		}
		log.Info("starting gRPC transport", "server", o.Transport.GRPC.ServerAddrPort, "attributes", readWriter.Attributes)
		tr = readWriter
		tw = readWriter
	}

	var re RuntimeExecutor
	switch o.RuntimeSelected {
	case KubernetesRuntimeType:
		kn, err := kubernetes.NewConfig(log, o.Runtime.Kubernetes.Namespace, o.Runtime.Kubernetes.Kubeconfig,
			kubernetes.WithServiceAccountName(o.Runtime.Kubernetes.ServiceAccountName))
		if err != nil {
			return fmt.Errorf("unable to create Kubernetes config: %w", err)
		}
		re = kn
		log.Info("using Kubernetes runtime")
	case ContainerdRuntimeType:
		opts := []containerd.Opt{}
		if o.Runtime.Containerd.Namespace != "" {
			opts = append(opts, containerd.WithNamespace(o.Runtime.Containerd.Namespace))
		}
		if o.Runtime.Containerd.SocketPath != "" {
			opts = append(opts, containerd.WithSocketPath(o.Runtime.Containerd.SocketPath))
		}
		if o.Runtime.Containerd.DataRoot != "" {
			opts = append(opts, containerd.WithDataRoot(o.Runtime.Containerd.DataRoot))
		}
		if o.Runtime.Containerd.RegistryConfigPath != "" {
			opts = append(opts, containerd.WithRegistryConfigPath(o.Runtime.Containerd.RegistryConfigPath))
		}
		cd, err := containerd.NewConfig(log, opts...)
		if err != nil {
			return fmt.Errorf("unable to create Containerd config: %w", err)
		}
		re = cd
		log.Info("using Containerd runtime")
	default:
		opts := []client.Opt{
			client.FromEnv,
		}
		if o.Runtime.Docker.SocketPath != "" {
			opts = append(opts, client.WithHost(fmt.Sprintf("unix://%s", o.Runtime.Docker.SocketPath)))
		}
		dclient, err := client.New(opts...)
		if err != nil {
			return fmt.Errorf("unable to create Docker client: %w", err)
		}
		dockerExecutor := &docker.Config{
			Client: dclient,
			Log:    log,
			RegistryAuth: &registry.AuthConfig{
				Username:      o.Registry.User,
				Password:      o.Registry.Pass,
				ServerAddress: o.Registry.Name,
			},
		}
		re = dockerExecutor
		log.Info("using Docker runtime")
	}

	a := &Config{
		TransportReader: tr,
		RuntimeExecutor: re,
		// Always available, regardless of the selected container runtime: an action opts into it
		// by carrying a run script instead of an image.
		ScriptExecutor:  &script.Config{Log: log},
		TransportWriter: tw,
		Retry:           o.Retry,
	}

	eg, ctx := errgroup.WithContext(inctx)
	if startTransport != nil {
		eg.Go(func() error {
			return startTransport(ctx)
		})
	}

	eg.Go(func() error {
		a.Run(ctx, log)
		return nil
	})

	if err := eg.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func (t TransportType) String() string {
	return string(t)
}

func (r RuntimeType) String() string {
	return string(r)
}

func (t *TransportType) Set(s string) error {
	switch strings.ToLower(s) {
	case GRPCTransportType.String(), NATSTransportType.String(), FileTransportType.String():
		*t = TransportType(s)
		return nil
	default:
		return fmt.Errorf("invalid Transport type: %q, must be one of [%s, %s, %s]", s, GRPCTransportType, NATSTransportType, FileTransportType)
	}
}

func (t *TransportType) Type() string {
	return "transport-type"
}

func (r *RuntimeType) Set(s string) error {
	switch strings.ToLower(s) {
	case DockerRuntimeType.String(), ContainerdRuntimeType.String(), KubernetesRuntimeType.String():
		*r = RuntimeType(strings.ToLower(s))
		return nil
	default:
		return fmt.Errorf("invalid Runtime type: %q, must be one of [%s, %s, %s]", s, DockerRuntimeType, ContainerdRuntimeType, KubernetesRuntimeType)
	}
}

func (r *RuntimeType) Type() string {
	return "runtime-type"
}

// humanDuration prints human readable units that have non-zero values. The precision is the number of units to print.
// For example, if the duration is 1 hour, 2 minutes, and 3 seconds, and the precision is 2, it will print "1h2m".
func humanDuration(d time.Duration, precision int) string {
	// Convert everything to nanoseconds for precise calculations
	nanos := d.Nanoseconds()

	// Calculate each unit from nanoseconds
	hours := nanos / time.Hour.Nanoseconds()
	remainingAfterHours := nanos % time.Hour.Nanoseconds()
	minutes := remainingAfterHours / time.Minute.Nanoseconds()
	remainingAfterMinutes := remainingAfterHours % time.Minute.Nanoseconds()
	seconds := remainingAfterMinutes / time.Second.Nanoseconds()
	remainingAfterSeconds := remainingAfterMinutes % time.Second.Nanoseconds()
	milliseconds := remainingAfterSeconds / time.Millisecond.Nanoseconds()
	remainingAfterMillis := remainingAfterSeconds % time.Millisecond.Nanoseconds()
	microseconds := remainingAfterMillis / time.Microsecond.Nanoseconds()
	nanoseconds := remainingAfterMillis % time.Microsecond.Nanoseconds()

	// Build the duration string with only non-zero units
	parts := []string{}
	if hours > 0 && len(parts) < precision {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 && len(parts) < precision {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 && len(parts) < precision {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	if milliseconds > 0 && len(parts) < precision {
		parts = append(parts, fmt.Sprintf("%dms", milliseconds))
	}
	if microseconds > 0 && len(parts) < precision {
		parts = append(parts, fmt.Sprintf("%dus", microseconds))
	}
	if nanoseconds > 0 && len(parts) < precision {
		parts = append(parts, fmt.Sprintf("%dns", nanoseconds))
	}

	// Join all parts with spaces
	return strings.Join(parts, "")
}
