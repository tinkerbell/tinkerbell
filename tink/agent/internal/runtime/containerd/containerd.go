package containerd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/avast/retry-go/v4"
	gocni "github.com/containerd/go-cni"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/content"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/core/remotes/docker"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containers/image/v5/pkg/shortnames"
	"github.com/containers/image/v5/types"
	"github.com/go-logr/logr"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
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
	// DataRoot is the on-disk root used for nerdctl-compatible per-container
	// state (the json-file logs that `nerdctl logs` reads).
	DataRoot string
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

func WithDataRoot(dataRoot string) Opt {
	return func(c *Config) {
		c.DataRoot = dataRoot
	}
}

func NewConfig(log logr.Logger, opts ...Opt) (*Config, error) {
	c := &Config{
		Log:        log,
		Namespace:  defaultNamespace,
		SocketPath: defaultSocketPath,
		DataRoot:   defaultDataRoot,
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
			log.Info("unable to check for IP forwarding, CNI bridge NAT may not work", "error", err)
		}
		if !enabled {
			log.Info("IP forwarding is disabled, CNI bridge NAT will not work. Container network may have no external connectivity.")
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
	image, err := c.Client.GetImage(ctx, imageName)
	if err != nil {
		// if the image isn't already in our namespaced context, then pull it
		pullImage := func() error {
			image, err = c.Client.Pull(ctx, imageName, containerd.WithPullUnpack, containerd.WithResolver(docker.NewResolver(docker.ResolverOptions{})))
			if err != nil {
				return fmt.Errorf("error pulling image: %w", err)
			}
			c.Log.V(1).Info("image pulled", "image", image.Name())

			return nil
		}
		err := retry.Do(pullImage, retry.Attempts(5), retry.Delay(2*time.Second), retry.MaxDelay(10*time.Second), retry.DelayType(retry.BackOffDelay))
		if err != nil {
			return err
		}
	}

	// Determine network mode.
	// Default to CNI bridge networking (isolated network namespace with NAT).
	// Use host networking only if explicitly requested via namespaces.network: "host"
	useHostNetwork := a.Namespaces.Network == "host"

	// Build DNS configuration files for the container.
	// Both host-network and isolated-network containers need DNS files because
	// containerd does not automatically mount /etc/resolv.conf, /etc/hosts, or
	// /etc/hostname into containers. For host-network containers, localhost
	// nameservers are preserved since they are reachable. For isolated-network
	// containers, localhost nameservers are filtered out (unreachable from a
	// separate network namespace) and replaced with public DNS fallbacks.
	df, err := prepareDNSFiles(useHostNetwork)
	if err != nil {
		return fmt.Errorf("failed to build DNS files: %w", err)
	}
	defer func() {
		if err := df.cleanup(); err != nil {
			c.Log.Info("failed to cleanup DNS files", "error", err)
		}
	}()

	// Compute the per-container log directory and json-file path. nerdctl
	// reads these locations directly (it does NOT consult the
	// nerdctl/log-uri label), so they must match its conventions exactly
	// for `nerdctl logs <id>` to work.
	dataStore, err := dataStoreDir(c.DataRoot, c.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to compute data store: %w", err)
	}

	// create a container
	tainer, hostname, err := c.createContainer(ctx, image, a, useHostNetwork, df, dataStore)
	if err != nil {
		return fmt.Errorf("error creating container: %w", err)
	}
	// success is flipped to true at the single happy-path return below.
	// The deferred cleanups below key off it: on failure we retain the
	// container (and its rootfs snapshot + on-disk log directory) so an
	// operator can run `nerdctl --namespace tinkerbell ps -a / inspect /
	// logs <id>` and then `nerdctl rm <id>` to reclaim everything. CNI and
	// DNS scratch state are always cleaned up because they leak host
	// resources rather than debug value.
	success := false
	defer func() {
		if !success {
			c.Log.Info("action failed, retaining container for debugging (clean up with `nerdctl rm`)",
				"container", tainer.ID(), "namespace", c.Namespace)
			return
		}
		dctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), c.Namespace), 10*time.Second)
		if err := tainer.Delete(dctx, containerd.WithSnapshotCleanup); err != nil {
			c.Log.Info("failed to delete container", "container", tainer.ID(), "error", err)
		}
		cancel()
	}()

	// Populate hosts and hostname files using the hostname computed during
	// container creation (host's real hostname for host-network, truncated
	// container ID for isolated-network).
	if err := df.setHostname(hostname, useHostNetwork); err != nil {
		c.Log.Error(err, "failed to set container hostname in DNS files")
	}

	containerID := tainer.ID()

	// Write log-config.json so `nerdctl logs` selects the json-file driver.
	logDir := containerLogDir(dataStore, c.Namespace, containerID)
	if err := writeLogConfig(logDir, c.SocketPath); err != nil {
		return fmt.Errorf("failed to write log-config.json: %w", err)
	}

	// Open the json-file log writer pair and tee container stdout/stderr
	// into both the json-file (for `nerdctl logs`) and tink-agent's own
	// stdout/stderr (which are forwarded via syslog to tink-server).
	logPair, err := newJSONLogPair(containerLogFile(dataStore, c.Namespace, containerID))
	if err != nil {
		return fmt.Errorf("failed to open container log file: %w", err)
	}
	// Closed below, after task.Delete, so io copy goroutines drain first.

	// On success, remove the per-container log directory so we don't
	// accumulate state on long-lived hosts. On failure (or context
	// cancellation) we deliberately keep the directory so operators can run
	// `nerdctl logs <id>` for post-mortem. Registered BEFORE the task defer
	// so it runs AFTER it (LIFO): the log file must be closed and flushed
	// before the directory is removed.
	defer func() {
		if success {
			if err := os.RemoveAll(logDir); err != nil {
				c.Log.Info("failed to remove container log directory", "dir", logDir, "error", err)
			}
		}
	}()

	// create the task
	task, err := tainer.NewTask(ctx, cio.NewCreator(cio.WithStreams(
		nil,
		io.MultiWriter(os.Stdout, logPair.Stdout),
		io.MultiWriter(os.Stderr, logPair.Stderr),
	)))
	if err != nil {
		_ = logPair.Close()
		return fmt.Errorf("error creating task: %w", err)
	}
	defer func() {
		dctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), c.Namespace), 10*time.Second)
		status, err := task.Delete(dctx)
		if err != nil {
			c.Log.Info("failed to delete task", "task", task.ID(), "error", err)
		} else {
			c.Log.V(1).Info("task deleted", "task", task.ID(), "status", status)
		}
		cancel()
		// Flush + close the log file AFTER task.Delete so the cio copy
		// goroutines have finished writing.
		if err := logPair.Close(); err != nil {
			c.Log.Info("failed to close container log file", "error", err)
		}
	}()

	// Setup CNI networking if not using host network
	c.Log.V(1).Info("network configuration", "useHostNetwork", useHostNetwork, "cniAvailable", c.CNI != nil, "networkNamespace", a.Namespaces.Network)
	if !useHostNetwork && c.CNI != nil {
		netns := fmt.Sprintf("/proc/%d/ns/net", task.Pid())
		_, cniSetupErr := c.CNI.Setup(ctx, containerID, netns)
		// Always attempt to remove the CNI network, even if setup returned an error,
		// to avoid leaking any partially configured resources.
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), c.Namespace), 10*time.Second)
			if err := c.CNI.Remove(cleanupCtx, containerID, netns); err != nil {
				c.Log.Info("failed to remove CNI network", "container", containerID, "error", err)
			}
			cancel()
		}()
		if cniSetupErr != nil {
			c.Log.Error(cniSetupErr, "failed to setup CNI network, container will have no network")
			// If CNI setup fails, we continue - the container will have no network
			// but this is better than failing completely. The error is logged for debugging.
		} else {
			c.Log.V(1).Info("CNI network setup complete", "container", containerID, "netns", netns)
		}
	} else if !useHostNetwork && c.CNI == nil {
		c.Log.Info("CNI not available, container will have isolated network with no connectivity")
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

	select {
	case exitStatus := <-statusC:
		if exitStatus.ExitCode() != 0 {
			return fmt.Errorf("task exited with non-zero code: %d, error: %w", exitStatus.ExitCode(), exitStatus.Error())
		}
		success = true
		return nil
	case <-ctx.Done():
		// Context was cancelled; kill the task and wait for it to exit.
		kctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), c.Namespace), 10*time.Second)
		if err := task.Kill(kctx, syscall.SIGKILL); err != nil {
			c.Log.Error(err, "failed to kill task after context cancellation")
		}
		cancel()
		// Drain the exit status channel so deferred cleanup can proceed.
		<-statusC
		return fmt.Errorf("context cancelled while waiting for task: %w", ctx.Err())
	}
}

func (c *Config) createContainer(ctx context.Context, image containerd.Image, action spec.Action, useHostNetwork bool, df *dnsFiles, dataStore string) (containerd.Container, string, error) {
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
		// Only entrypoint override: replace entrypoint but keep image CMD.
		// This mirrors Docker behavior where setting Entrypoint alone preserves CMD.
		specOpts = append(specOpts, withEntrypointOverride(image, action.Cmd))
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

	// Bind-mount generated DNS config files (resolv.conf, hosts, hostname).
	// Containerd does not do this automatically.
	specOpts = append(specOpts, dnsSpecOpts(df)...)
	c.Log.V(1).Info("DNS configuration files mounted")

	// Generate a unique container ID (64-character hex string from 32 random
	// bytes) matching nerdctl's approach. The human-readable name is used for
	// the snapshot so containers are easier to identify during debugging.
	containerID, err := generateID()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate container ID: %w", err)
	}
	// displayName is the human-readable name surfaced to operators (NAMES
	// column in `nerdctl ps`, .Name in `nerdctl inspect`). snapshotID is
	// the on-disk snapshot identifier, suffixed with the first 8 chars of
	// the container ID so retried actions (which reuse action.ID) don't
	// collide on the snapshot name when an earlier failed attempt's
	// container has been retained for debugging. Keeping these distinct
	// preserves a stable display name across retries.
	displayName := conv.ParseName(action.ID, action.Name)
	snapshotID := displayName + "-" + containerID[:8]
	newOpts = append(newOpts, containerd.WithNewSnapshot(snapshotID, image))

	// Compute the container hostname. For host-network containers, use the
	// host's real hostname; for isolated-network containers, use the first
	// 12 chars of the container ID (matching nerdctl/Docker convention).
	hostname := truncateHostname(containerID)
	if useHostNetwork {
		if h, err := os.Hostname(); err != nil {
			c.Log.Info("failed to get host hostname, using container ID", "error", err)
		} else {
			hostname = h
		}
	}

	// Set the OCI spec hostname so gethostname(2) / the hostname command
	// inside the container returns the correct value.
	specOpts = append(specOpts, oci.WithHostname(hostname))
	newOpts = append(newOpts, containerd.WithNewSpec(specOpts...))

	// Container labels. Verified against nerdctl main: pkg/cmd/container/
	// {logs,remove,inspect}.go, pkg/logging/log_viewer.go, pkg/ocihook/
	// ocihook.go, pkg/containerutil/container_network_manager.go.
	//
	// Required for `nerdctl logs`:
	//   - nerdctl/namespace: read into LogViewOptions.Namespace; an empty
	//     namespace fails Validate() with "log viewing options require a
	//     ContainerID and Namespace". The log driver itself is selected
	//     from log-config.json on disk (see writeLogConfig), NOT a label.
	//
	// Required for `nerdctl rm` to clean up our per-container log dir:
	//   - nerdctl/state-dir: nerdctl rm does os.RemoveAll(labels[state-dir]).
	//
	// Required because nerdctl json.Unmarshals it unconditionally:
	//   - nerdctl/extraHosts: ocihook and container_network_manager parse
	//     this as JSON; missing/empty would trip the OCI hook with a json
	//     error. "[]" is the safe minimum.
	//
	// Cosmetic (surfaced by `nerdctl ps` / `nerdctl inspect` only):
	//   - nerdctl/name:     NAMES column in `ps`; Name in `inspect`.
	//   - nerdctl/hostname: Config.Hostname in `inspect`.
	//   - nerdctl/log-uri:  HostConfig.LogConfig.LogURI in `inspect`. NOT
	//     read by `nerdctl logs` (driver comes from log-config.json).
	labels := map[string]string{
		"nerdctl/namespace":  c.Namespace,
		"nerdctl/name":       displayName,
		"nerdctl/extraHosts": "[]",
		"nerdctl/hostname":   hostname,
		"nerdctl/log-uri":    "json-file://" + containerLogFile(dataStore, c.Namespace, containerID),
		"nerdctl/state-dir":  containerLogDir(dataStore, c.Namespace, containerID),
	}
	newOpts = append(newOpts, containerd.WithContainerLabels(labels))

	ctr, err := c.Client.NewContainer(ctx, containerID, newOpts...)
	if err != nil {
		return nil, "", err
	}
	return ctr, hostname, nil
}

// withEntrypointOverride returns an oci.SpecOpts that overrides the image's ENTRYPOINT
// with the given command while preserving the image's CMD. This mirrors Docker's behavior
// where setting only the Entrypoint preserves the existing CMD from the image config.
func withEntrypointOverride(image containerd.Image, cmd string) oci.SpecOpts {
	return func(ctx context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		ic, err := image.Config(ctx)
		if err != nil {
			return fmt.Errorf("failed to get image config: %w", err)
		}
		if !images.IsConfigType(ic.MediaType) {
			return fmt.Errorf("unknown image config media type %s", ic.MediaType)
		}

		var ociimage v1.Image
		imageConfigBytes, err := content.ReadBlob(ctx, image.ContentStore(), ic)
		if err != nil {
			return fmt.Errorf("failed to read image config: %w", err)
		}
		if err := json.Unmarshal(imageConfigBytes, &ociimage); err != nil {
			return fmt.Errorf("failed to unmarshal image config: %w", err)
		}

		// Replace entrypoint with action.Cmd, keep image CMD
		if s.Process == nil {
			s.Process = &specs.Process{}
		}
		s.Process.Args = append([]string{cmd}, ociimage.Config.Cmd...)

		return nil
	}
}

// generateID creates a random container ID as a 64-character hex string
// (32 random bytes), matching nerdctl's approach to container ID generation.
func generateID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random ID: %w", err)
	}
	return hex.EncodeToString(b), nil
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
