package containerd

import (
	"context"
	"fmt"
	"os"
	"time"

	gocni "github.com/containerd/go-cni"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containers/image/v5/pkg/shortnames"
	"github.com/containers/image/v5/types"
	"github.com/go-logr/logr"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/pkg/conv"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

const (
	// Default CNI paths used by HookOS
	defaultCNIBinDir  = "/opt/cni/bin"
	defaultCNIConfDir = "/etc/cni/net.d"

	defaultNamespace  = "tinkerbell"
	defaultSocketPath = "/run/containerd/containerd.sock"

	// Fallback bridge network configuration for tink-agent containers.
	// Only used if no CNI configs exist in /etc/cni/net.d/.
	// This follows the same pattern as nerdctl's default bridge network.
	fallbackBridgeConflist = `{
         "cniVersion": "1.0.0",
         "name": "bridge",
         "nerdctlID": "bridge",
         "nerdctlLabels": {
           "nerdctl/default-network": "true"
         },
         "plugins": [
           {
             "type": "bridge",
             "bridge": "cni0",
             "isGateway": true,
             "ipMasq": true,
             "hairpinMode": true,
             "ipam": {
               "type": "host-local",
               "ranges": [
                 [
                   {
                     "subnet": "172.17.0.0/23",
                     "gateway": "172.17.0.1"
                   }
                 ]
               ],
               "routes": [
                 {
                   "dst": "0.0.0.0/0"
                 }
               ]
             }
           },
           {
             "type": "portmap",
             "capabilities": {
               "portMappings": true
             }
           },
           {
             "type": "firewall",
             "ingressPolicy": "same-bridge"
           },
           {
             "type": "tuning"
           }
         ]
    }`
)

type Config struct {
	Namespace  string
	Client     *containerd.Client
	Log        logr.Logger
	SocketPath string
	CNI        gocni.CNI
}

type Opt func(*Config)

func WithNamespace(namespace string) Opt {
	return func(c *Config) {
		c.Namespace = namespace
	}
}

func WithClient(client *containerd.Client) Opt {
	return func(c *Config) {
		c.Client = client
	}
}

func WithSocketPath(socketPath string) Opt {
	return func(c *Config) {
		c.SocketPath = socketPath
	}
}

func NewConfig(log logr.Logger, opts ...Opt) (*Config, error) {
	c := &Config{
		Log:        log,
		Namespace:  defaultNamespace,
		SocketPath: defaultSocketPath,
	}
	for _, opt := range opts {
		opt(c)
	}

	if c.Client == nil {
		client, err := containerd.New(c.SocketPath)
		if err != nil {
			return nil, fmt.Errorf("error creating containerd client: %w", err)
		}
		c.Client = client
	}

	if c.CNI == nil {
		// Initialize CNI for bridge networking.
		// First, try to load existing CNI configs from the standard config directory.
		// This allows HookOS or users to provide their own CNI configuration with custom subnets.
		cni, err := gocni.New(
			gocni.WithPluginDir([]string{defaultCNIBinDir}),
			gocni.WithPluginConfDir(defaultCNIConfDir),
			gocni.WithDefaultConf,
		)
		if err != nil {
			// No existing CNI configs found, fall back to our embedded default bridge config.
			log.V(1).Info("CNI initialization from config directory failed, using fallback bridge config", "confDir", defaultCNIConfDir, "error", err)
			cni, err = gocni.New(
				gocni.WithPluginDir([]string{defaultCNIBinDir}),
				gocni.WithConfListBytes([]byte(fallbackBridgeConflist)),
			)
			if err != nil {
				log.V(1).Info("CNI initialization failed, bridge networking unavailable", "error", err)
			} else {
				c.CNI = cni
				log.V(1).Info("CNI initialized with fallback bridge config")
			}
		} else {
			c.CNI = cni
			log.V(1).Info("CNI initialized from config directory", "confDir", defaultCNIConfDir)
		}
	}

	// Check IP forwarding - required for CNI bridge NAT/masquerade to work.
	// Without this, traffic from containers won't be forwarded to external networks.
	if c.CNI != nil {
		enabled, err := isIPForwardingEnabled(log)
		if err != nil {
			log.V(1).Info("unable to check for IP forwarding, CNI bridge NAT may not work", "error", err)
		}
		if !enabled {
			log.V(1).Info("IP forwarding is disabled, CNI bridge NAT will not work. Container network may have no external connectivity.")
		}
	}

	return c, nil
}

func (c *Config) Execute(ctx context.Context, a spec.Action) error {
	ctx = namespaces.WithNamespace(ctx, c.Namespace)
	// Pull the image
	imageName := a.Image
	r, err := shortnames.Resolve(&types.SystemContext{PodmanOnlyShortNamesIgnoreRegistriesConfAndForceDockerHub: true}, imageName)
	if err != nil {
		c.Log.Info("unable to resolve image fully qualified name", "error", err)
	}
	if r != nil && len(r.PullCandidates) > 0 {
		imageName = r.PullCandidates[0].Value.String()
	}
	// set up a containerd namespace
	ctx = namespaces.WithNamespace(ctx, c.Namespace)
	image, err := c.Client.GetImage(ctx, imageName)
	if err != nil {
		// if the image isn't already in our namespaced context, then pull it
		image, err = c.Client.Pull(ctx, imageName, containerd.WithPullUnpack, containerd.WithResolver(docker.NewResolver(docker.ResolverOptions{})))
		if err != nil {
			return fmt.Errorf("error pulling image: %w", err)
		}
		c.Log.V(1).Info("image pulled", "image", image.Name())
	}

	// Determine network mode.
	// Default to CNI bridge networking (isolated network namespace with NAT).
	// Use host networking only if explicitly requested via namespaces.network: "host"
	useHostNetwork := a.Namespaces.Network == "host"

	// create a container
	tainer, err := c.createContainer(ctx, image, a, useHostNetwork)
	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}
	defer func() { _ = tainer.Delete(ctx, containerd.WithSnapshotCleanup) }()

	// create the task
	task, err := tainer.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("error creating task: %w", err)
	}
	defer func() { _, _ = task.Delete(ctx) }()

	// Setup CNI networking if not using host network
	containerID := tainer.ID()
	c.Log.V(1).Info("network configuration", "useHostNetwork", useHostNetwork, "cniAvailable", c.CNI != nil, "networkNamespace", a.Namespaces.Network)
	if !useHostNetwork && c.CNI != nil {
		netns := fmt.Sprintf("/proc/%d/ns/net", task.Pid())
		if _, err := c.CNI.Setup(ctx, containerID, netns); err != nil {
			c.Log.Error(err, "failed to setup CNI network, container will have no network")
			// If CNI setup fails, we continue - the container will have no network
			// but this is better than failing completely. The error is logged for debugging.
		} else {
			c.Log.V(1).Info("CNI network setup complete", "container", containerID, "netns", netns)
			defer func() {
				// Use a background context for cleanup so teardown completes
				// even if the parent context has been canceled.
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := c.CNI.Remove(cleanupCtx, containerID, netns); err != nil {
					c.Log.Error(err, "failed to remove CNI network", "container", containerID)
				}
			}()
		}
	} else if !useHostNetwork && c.CNI == nil {
		c.Log.Info("WARNING: CNI not available, container will have isolated network with no connectivity")
	}

	var statusC <-chan containerd.ExitStatus
	statusC, err = task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("error waiting on task: %w", err)
	}

	// start the task
	if err := task.Start(ctx); err != nil {
		_, _ = task.Delete(ctx)
		return fmt.Errorf("error starting task: %w", err)
	}

	exitStatus := <-statusC
	if exitStatus.ExitCode() != 0 {
		return fmt.Errorf("task exited with non-zero code: %d, error: %w", exitStatus.ExitCode(), exitStatus.Error())
	}
	return nil
}

func (c *Config) createContainer(ctx context.Context, image containerd.Image, action spec.Action, useHostNetwork bool) (containerd.Container, error) {
	newOpts := []containerd.NewContainerOpts{}
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image), // Loads ENTRYPOINT and CMD from image
		oci.WithPrivileged,
		oci.WithAllDevicesAllowed, // Allow access to all devices via cgroup rules
		oci.WithHostDevices,       // Mount all host devices into the container
		oci.WithEnv(conv.ParseEnv(action.Env)),
	}

	// Replicate Docker's Entrypoint/Cmd semantics:
	// - action.Cmd maps to Docker's Entrypoint (the binary to run)
	// - action.Args maps to Docker's Cmd (arguments to the entrypoint)
	// In OCI spec, Process.Args = Entrypoint + Cmd combined.
	switch {
	case action.Cmd != "" && len(action.Args) > 0:
		// Both specified: override entrypoint and cmd
		specOpts = append(specOpts, oci.WithProcessArgs(append([]string{action.Cmd}, action.Args...)...))
	case action.Cmd != "":
		// Only entrypoint override: use Cmd as the sole process arg
		specOpts = append(specOpts, oci.WithProcessArgs(action.Cmd))
	case len(action.Args) > 0:
		// Only args override: keep image ENTRYPOINT, replace CMD
		specOpts = append(specOpts, oci.WithImageConfigArgs(image, action.Args))
	}

	// Add volume mounts
	if len(action.Volumes) > 0 {
		mounts := parseVolumes(c.Log, action.Volumes)
		if len(mounts) > 0 {
			specOpts = append(specOpts, oci.WithMounts(mounts))
			c.Log.V(1).Info("volume mounts configured", "count", len(mounts))
		}
	}

	if action.Namespaces.PID == "host" {
		specOpts = append(specOpts, oci.WithHostNamespace(specs.PIDNamespace))
	}

	// Configure network namespace
	if useHostNetwork {
		specOpts = append(specOpts, oci.WithHostNamespace(specs.NetworkNamespace))
		c.Log.V(1).Info("using host network namespace")
	}
	// If not using host network, leave the network namespace isolated - CNI will be setup
	// after the task is created in Execute()

	name := conv.ParseName(action.ID, action.Name)
	newOpts = append(newOpts, containerd.WithNewSnapshot(name, image))
	newOpts = append(newOpts, containerd.WithNewSpec(specOpts...))

	return c.Client.NewContainer(ctx, name, newOpts...)
}

// isIPForwardingEnabled reports whether IPv4 forwarding is enabled.
//
// IPv4 forwarding is required for CNI bridge
// networking with NAT/masquerade to work properly.
func isIPForwardingEnabled(log logr.Logger) (bool, error) {
	const ipForwardPath = "/proc/sys/net/ipv4/ip_forward"

	// Check current value
	current, err := os.ReadFile(ipForwardPath)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", ipForwardPath, err)
	}

	if len(current) > 0 && current[0] == '1' {
		log.V(1).Info("IP forwarding already enabled")
		return true, nil
	}

	log.V(1).Info("IP forwarding is disabled")
	return false, nil
}
