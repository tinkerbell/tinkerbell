package agent

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/docker/docker/client"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/attribute"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/runtime/containerd"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/runtime/docker"
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
	TransportWriter TransportWriter
}

func (c *Config) Run(ctx context.Context, log logr.Logger) {
	// All steps are synchronous and blocking
	// 1. get an action from the input transport
	// 3. send running/starting event to the output transport
	// 4. send the action to the runtime for execution
	// 5. send the result event to the output transport
	// 6. go to step 1

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		action, err := c.TransportReader.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Info("error reading/retrieving action", "error", err)
			continue
		}

		log.Info("received action", "action", action)
		if err := c.TransportWriter.Write(ctx, spec.Event{Action: action, Message: "running action", State: spec.StateRunning}); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Info("error writing event", "error", err)
			continue
		}
		log.Info("reported action status", "action", action, "state", spec.StateRunning)

		state := spec.StateSuccess
		// TODO(jacobweinstock): Add a retry count that comes from a CLI flag. It should only take precedence if the action has a retry count of 0.
		retries := ternary(action.Retries == 0, 1, action.Retries)

		responseEvent := spec.Event{}
		action.ExecutionStart = time.Now().UTC()
		timeoutCtx, timeoutDone := context.WithTimeout(ctx, time.Duration(action.TimeoutSeconds)*time.Second)
		for i := 1; i <= retries; i++ {
			if err := c.RuntimeExecutor.Execute(timeoutCtx, action); err != nil {
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

		if err := c.TransportWriter.Write(ctx, responseEvent); err != nil {
			log.Info("error writing event", "error", err)
			continue
		}
		log.Info("reported action status", "action", action, "state", state)
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
}

type Transport struct {
	GRPC GRPCTransport
	File FileTransport
	NATS NATSTransport
}

type Runtime struct {
	Docker     DockerRuntime
	Containerd ContainerdRuntime
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
	ServerAddrPort netip.AddrPort
	TLSEnabled     bool
	TLSInsecure    bool
	RetryInterval  time.Duration
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
}

func (o *Options) ConfigureAndRun(ctx context.Context, log logr.Logger, id string) error {
	// instantiate the implementation for the transport reader
	// instantiate the implementation for the transport writer
	// instantiate the implementation for the runtime executor
	// instantiate the agent
	// run the agent
	eg, ectx := errgroup.WithContext(ctx)
	ctx = ectx
	var tr TransportReader
	var tw TransportWriter
	switch o.TransportSelected {
	case FileTransportType:
		readWriter := &file.Config{
			Log:     log,
			Actions: make(chan spec.Action),
			FileLoc: "./example/file_template.yaml",
		}
		eg.Go(func() error {
			return readWriter.Start(ctx)
		})
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
		eg.Go(func() error {
			return readWriter.Start(ctx)
		})
		tr = readWriter
		tw = readWriter
	default:
		conn, err := grpc.NewClientConn(o.Transport.GRPC.ServerAddrPort.String(), o.Transport.GRPC.TLSEnabled, o.Transport.GRPC.TLSInsecure)
		if err != nil {
			return fmt.Errorf("unable to create gRPC client: %w", err)
		}
		readWriter := &grpc.Config{
			Log:              log,
			TinkServerClient: proto.NewWorkflowServiceClient(conn),
			WorkerID:         id,
			RetryInterval:    time.Second * 5,
			Actions:          make(chan spec.Action),
		}
		if o.AttributeDetectionEnabled {
			readWriter.Attributes = grpc.ToProto(attribute.DiscoverAll())
		}
		log.Info("starting gRPC transport", "server", o.Transport.GRPC.ServerAddrPort)
		tr = readWriter
		tw = readWriter
	}

	var re RuntimeExecutor
	switch o.RuntimeSelected {
	case ContainerdRuntimeType:
		opts := []containerd.Opt{}
		if o.Runtime.Containerd.Namespace != "" {
			opts = append(opts, containerd.WithNamespace(o.Runtime.Containerd.Namespace))
		}
		if o.Runtime.Containerd.SocketPath != "" {
			opts = append(opts, containerd.WithSocketPath(o.Runtime.Containerd.SocketPath))
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
			client.WithAPIVersionNegotiation(),
		}
		if o.Runtime.Docker.SocketPath != "" {
			opts = append(opts, client.WithHost(fmt.Sprintf("unix://%s", o.Runtime.Docker.SocketPath)))
		}
		dclient, err := client.NewClientWithOpts(opts...)
		if err != nil {
			return fmt.Errorf("unable to create Docker client: %w", err)
		}
		// TODO(jacobweinstock): handle auth
		dockerExecutor := &docker.Config{
			Client: dclient,
			Log:    log,
		}
		re = dockerExecutor
		log.Info("using Docker runtime")
	}

	a := &Config{
		TransportReader: tr,
		RuntimeExecutor: re,
		TransportWriter: tw,
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
	case DockerRuntimeType.String(), ContainerdRuntimeType.String():
		*r = RuntimeType(s)
		return nil
	default:
		return fmt.Errorf("invalid Runtime type: %q, must be one of [%s, %s, %s]", s, GRPCTransportType, NATSTransportType, FileTransportType)
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
	hours := nanos / (time.Hour.Nanoseconds())
	remainingAfterHours := nanos % (time.Hour.Nanoseconds())
	minutes := remainingAfterHours / (time.Minute.Nanoseconds())
	remainingAfterMinutes := remainingAfterHours % (time.Minute.Nanoseconds())
	seconds := remainingAfterMinutes / (time.Second.Nanoseconds())
	remainingAfterSeconds := remainingAfterMinutes % (time.Second.Nanoseconds())
	milliseconds := remainingAfterSeconds / (time.Millisecond.Nanoseconds())
	remainingAfterMillis := remainingAfterSeconds % (time.Millisecond.Nanoseconds())
	microseconds := remainingAfterMillis / (time.Microsecond.Nanoseconds())
	nanoseconds := remainingAfterMillis % (time.Microsecond.Nanoseconds())

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
