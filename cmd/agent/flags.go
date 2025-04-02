package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/pkg/flag/netip"
	"github.com/tinkerbell/tinkerbell/tink/agent"
)

type config struct {
	AgentID  string
	LogLevel int
	Options  *agent.Options
}

func RegisterFlagsLegacy(c *config, fs *flag.FlagSet) {
	fs.StringVar(&c.Options.Registry.User, "registry-username", "", "Container image Registry user for authentication")
	fs.StringVar(&c.Options.Registry.Pass, "registry-password", "", "Container image Registry pass for authentication")
	fs.StringVar(&c.Options.Registry.Name, "docker-registry", "", "Container image Registry name to which to log in")
	fs.StringVar(&c.Options.Transport.GRPC.ServerAddrPort, "tinkerbell-grpc-authority", "", "Tink server GRPC address:port")
	fs.BoolVar(&c.Options.Transport.GRPC.TLSInsecure, "tinkerbell-insecure-tls", false, "Tink server GRPC insecure TLS")
	fs.BoolVar(&c.Options.Transport.GRPC.TLSEnabled, "tinkerbell-tls", false, "Tink server GRPC use TLS")
}

// SetFromEnvLegacy gets any legacy cli flags from the environment and sets them in the config.
func SetFromEnvLegacy(c *config) {
	envs := []string{"REGISTRY_USERNAME", "REGISTRY_PASSWORD", "DOCKER_REGISTRY", "TINKERBELL_GRPC_AUTHORITY", "TINKERBELL_TLS", "TINKERBELL_INSECURE_TLS", "ID", "WORKER_ID"}
	for _, env := range envs {
		if v := os.Getenv(env); v != "" {
			switch env {
			case "REGISTRY_USERNAME":
				c.Options.Registry.User = v
			case "REGISTRY_PASSWORD":
				c.Options.Registry.Pass = v
			case "DOCKER_REGISTRY":
				c.Options.Registry.Name = v
			case "TINKERBELL_GRPC_AUTHORITY":
				c.Options.Transport.GRPC.ServerAddrPort = v
			case "TINKERBELL_TLS":
				b, err := strconv.ParseBool(v)
				if err == nil {
					c.Options.Transport.GRPC.TLSEnabled = b
				}
			case "TINKERBELL_INSECURE_TLS":
				b, err := strconv.ParseBool(v)
				if err == nil {
					c.Options.Transport.GRPC.TLSInsecure = b
				}
			case "ID", "WORKER_ID":
				c.AgentID = v
			}
		}
	}
}

func RegisterAllFlags(c *config) *ff.FlagSet {
	fst := flag.NewFlagSet("general", flag.ContinueOnError)
	RegisterRootFlags(c, fst)
	fsTransport := ff.NewFlagSetFrom("general", fst)

	fscr := flag.NewFlagSet("container registry", flag.ContinueOnError)
	RegisterRepositoryFlags(c, fscr)
	fsContainerRegistry := ff.NewFlagSetFrom("container registry", fscr).SetParent(fsTransport)

	fsc := flag.NewFlagSet("containerd runtime", flag.ContinueOnError)
	RegisterContainerdRuntimeFlags(c, fsc)
	fsContainerd := ff.NewFlagSetFrom("containerd runtime", fsc).SetParent(fsContainerRegistry)

	fsd := flag.NewFlagSet("docker runtime", flag.ContinueOnError)
	RegisterDockerRuntimeFlags(c, fsd)
	fsDocker := ff.NewFlagSetFrom("docker runtime", fsd).SetParent(fsContainerd)

	fsg := flag.NewFlagSet("grpc transport", flag.ContinueOnError)
	RegisterGRPCTransportFlags(c, fsg)
	fsGrpc := ff.NewFlagSetFrom("grpc transport", fsg).SetParent(fsDocker)

	fsf := flag.NewFlagSet("file transport", flag.ContinueOnError)
	RegisterFileTransportFlags(c, fsf)
	fsFile := ff.NewFlagSetFrom("file transport", fsf).SetParent(fsGrpc)

	fsn := flag.NewFlagSet("nats transport", flag.ContinueOnError)
	RegisterNATSTransportFlags(c, fsn)
	fsNats := ff.NewFlagSetFrom("nats transport", fsn).SetParent(fsFile)

	fsLegacy := flag.NewFlagSet("legacy", flag.ContinueOnError)
	RegisterFlagsLegacy(c, fsLegacy)
	fsl := ff.NewFlagSetFrom("legacy", fsLegacy).SetParent(fsNats)

	return fsl
}

func RegisterRootFlags(c *config, fs *flag.FlagSet) {
	fs.StringVar(&c.AgentID, "id", "", "ID of the agent")
	fs.IntVar(&c.LogLevel, "log-level", 0, "Log level")
	fs.Var(&c.Options.RuntimeSelected, "runtime", fmt.Sprintf("Container runtime used to run Actions, must be one of [%s, %s]", agent.DockerRuntimeType, agent.ContainerdRuntimeType))
	fs.Var(&c.Options.TransportSelected, "transport", fmt.Sprintf("Transport used to receive Workflows/Actions and to send results, must be one of [%s, %s, %s]", agent.GRPCTransportType, agent.NATSTransportType, agent.FileTransportType))
}

func RegisterRepositoryFlags(c *config, fs *flag.FlagSet) {
	fs.StringVar(&c.Options.Registry.Name, "registry-name", "", "Container image Registry name to which to log in")
	fs.StringVar(&c.Options.Registry.User, "registry-user", "", "Container image Registry user for authentication")
	fs.StringVar(&c.Options.Registry.Pass, "registry-pass", "", "Container image Registry pass for authentication")
}

func RegisterGRPCTransportFlags(c *config, fs *flag.FlagSet) {
	fs.StringVar(&c.Options.Transport.GRPC.ServerAddrPort, "grpc-server", "", "gRPC server address:port")
	fs.BoolVar(&c.Options.Transport.GRPC.TLSEnabled, "grpc-tls", false, "gRPC TLS enabled")
	fs.BoolVar(&c.Options.Transport.GRPC.TLSInsecure, "grpc-insecure-tls", false, "gRPC insecure TLS")
	fs.Var(ffval.NewValueDefault(&c.Options.Transport.GRPC.RetryInterval, 5*time.Second), "grpc-retry-interval", "gRPC retry interval in Seconds")
}

func RegisterFileTransportFlags(c *config, fs *flag.FlagSet) {
	fs.StringVar(&c.Options.Transport.File.WorkflowPath, "workflow-path", "", "Workflow file path")
}

func RegisterNATSTransportFlags(c *config, fs *flag.FlagSet) {
	fs.Var(&netip.AddrPort{AddrPort: &c.Options.Transport.NATS.ServerAddrPort}, "nats-server", "NATS server address:port")
	fs.StringVar(&c.Options.Transport.NATS.StreamName, "nats-stream", "tinkerbell", "NATS stream name")
	fs.StringVar(&c.Options.Transport.NATS.EventsSubject, "nats-events", "workflow_status", "NATS events subject")
	fs.StringVar(&c.Options.Transport.NATS.ActionsSubject, "nats-actions", "workflow_actions", "NATS actions subject")
}

func RegisterDockerRuntimeFlags(c *config, fs *flag.FlagSet) {
	fs.StringVar(&c.Options.Runtime.Docker.SocketPath, "docker-socket", "/var/run/docker.sock", "Docker socket path")
}

func RegisterContainerdRuntimeFlags(c *config, fs *flag.FlagSet) {
	fs.StringVar(&c.Options.Runtime.Containerd.Namespace, "containerd-namespace", "tinkerbell", "Containerd namespace")
	fs.StringVar(&c.Options.Runtime.Containerd.SocketPath, "containerd-socket", "/run/containerd/containerd.sock", "Containerd socket path")
}
