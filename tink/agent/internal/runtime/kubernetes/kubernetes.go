// Package kubernetes implements a tink-agent RuntimeExecutor that runs Actions as Kubernetes
// Jobs via the Kubernetes API instead of a local Docker/containerd socket. It is intended for a
// standing Agent running as a pod inside a Kubernetes cluster, not for the bare-metal
// CaptainOS/HookOS provisioning path.
package kubernetes

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	actionIDLabel = "tinkerbell.org/action-id"
	// actionContainerName is the name given to the Action's container in jobFor. podTerminated
	// and exitError match on this name specifically so an unrelated sidecar container injected
	// by a mutating admission webhook (Istio, Linkerd, Vault Agent Injector, etc.) can never be
	// mistaken for the Action's own outcome.
	actionContainerName = "action"
	// logStreamTimeout bounds how long streamLogs may block fetching a terminated Pod's logs.
	// It intentionally does not reuse Execute's ctx: logs are worth fetching even if the Action's
	// own timeout already fired, but the fetch itself must still be bounded, or a stalled kubelet
	// log endpoint would hang Execute (and therefore the single-threaded agent.Config.Run loop)
	// forever.
	logStreamTimeout = 30 * time.Second
)

// Config is a RuntimeExecutor that runs Actions as Kubernetes Jobs.
type Config struct {
	Log       logr.Logger
	Client    kubernetes.Interface // interface, not *kubernetes.Clientset, so it can be faked in tests
	Namespace string               // fixed namespace Jobs/Pods are created in
}

// Opt configures a Config returned by NewConfig.
type Opt func(*Config)

// WithClient overrides the Kubernetes client used, for example with a fake clientset in tests.
func WithClient(c kubernetes.Interface) Opt {
	return func(cfg *Config) { cfg.Client = c }
}

// NewConfig builds a Config. If kubeconfig is empty, it uses the in-cluster config, which is the
// expected deployment (the Agent running as a pod in the cluster it's creating Jobs in).
func NewConfig(log logr.Logger, namespace, kubeconfig string, opts ...Opt) (*Config, error) {
	cfg := &Config{Log: log, Namespace: namespace}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.Client != nil {
		return cfg, nil
	}

	restCfg, err := restConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: %w", err)
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes: %w", err)
	}
	cfg.Client = client

	return cfg, nil
}

func restConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

// Execute creates a Job for the Action, waits for its Pod to terminate, streams the Pod's logs,
// and returns nil on a zero exit code or a descriptive error otherwise. The Job (and its Pod) are
// always deleted before Execute returns.
func (c *Config) Execute(ctx context.Context, a spec.Action) error {
	job, err := c.jobFor(a)
	if err != nil {
		return err
	}

	jobs := c.Client.BatchV1().Jobs(c.Namespace)
	created, err := jobs.Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("kubernetes: creating job: %w", err)
	}

	defer func() {
		// Always clean up, regardless of outcome. Use a background context since ctx may
		// already be cancelled/expired, and foreground propagation so the Pod is gone too.
		policy := metav1.DeletePropagationForeground
		if err := jobs.Delete(context.Background(), created.Name, metav1.DeleteOptions{PropagationPolicy: &policy}); err != nil {
			c.Log.Info("failed to delete job", "job", created.Name, "error", err)
		}
	}()

	pod, err := c.waitForPod(ctx, a.ID)
	if err != nil {
		return err
	}

	c.streamLogs(pod.Name)

	return exitError(pod)
}

// jobFor builds the Kubernetes Job for an Action. It fails fast on Action fields that have no
// safe Kubernetes equivalent rather than silently ignoring them.
func (c *Config) jobFor(a spec.Action) (*batchv1.Job, error) {
	if a.Namespaces.PID != "" || a.Namespaces.Network != "" {
		return nil, fmt.Errorf("kubernetes runtime: host PID/network namespaces are not supported")
	}
	if len(a.Volumes) > 0 {
		return nil, fmt.Errorf("kubernetes runtime: volumes are not supported")
	}

	backoffLimit := int32(0) // retries are handled by (*agent.Config).Run, not the Job

	container := corev1.Container{
		Name:  actionContainerName,
		Image: a.Image,
		Env:   convEnv(a.Env),
	}
	if a.Cmd != "" {
		container.Command = []string{a.Cmd}
	}
	if len(a.Args) > 0 {
		container.Args = a.Args
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName(a.ID, a.Name),
			Namespace: c.Namespace,
			Labels:    map[string]string{actionIDLabel: a.ID},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{actionIDLabel: a.ID},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers:    []corev1.Container{container},
					// No ServiceAccountName override: inherits the executing Agent pod's own
					// ServiceAccount, including whatever imagePullSecrets it already carries.
					// No SecurityContext/Privileged: deliberately the Kubernetes default
					// (unprivileged), unlike docker.go/containerd.go which both set Privileged
					// unconditionally for the bare-metal case this runtime isn't used for.
				},
			},
		},
	}, nil
}

// waitForPod blocks until the Action's Job Pod reaches a terminal state (a container has
// terminated, or the Pod itself failed/succeeded without ever reporting a container status, e.g.
// ImagePullBackOff) and returns it.
func (c *Config) waitForPod(ctx context.Context, actionID string) (*corev1.Pod, error) {
	pods := c.Client.CoreV1().Pods(c.Namespace)
	selector := fmt.Sprintf("%s=%s", actionIDLabel, actionID)

	// Check first in case the Pod already reached a terminal state before the watch below is
	// established (e.g. a very fast-exiting container).
	list, err := pods.List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: listing pods: %w", err)
	}
	for i := range list.Items {
		if podTerminated(&list.Items[i]) {
			return &list.Items[i], nil
		}
	}

	w, err := pods.Watch(ctx, metav1.ListOptions{LabelSelector: selector, ResourceVersion: list.ResourceVersion})
	if err != nil {
		return nil, fmt.Errorf("kubernetes: watching pods: %w", err)
	}
	defer w.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case event, ok := <-w.ResultChan():
			if !ok {
				return nil, fmt.Errorf("kubernetes: watch closed before pod for action %s terminated", actionID)
			}
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			if podTerminated(pod) {
				return pod, nil
			}
		}
	}
}

func podTerminated(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		return true
	}
	return actionContainerStatus(pod).Terminated != nil
}

// exitError returns nil if the Action container exited 0, and a descriptive error otherwise. This
// is the fix over tinklet's kube.go, whose wait condition returns success as soon as a container
// has any terminated state without ever inspecting the exit code.
func exitError(pod *corev1.Pod) error {
	if t := actionContainerStatus(pod).Terminated; t != nil {
		if t.ExitCode == 0 {
			return nil
		}
		return fmt.Errorf("kubernetes: container exited %d: %s", t.ExitCode, t.Reason)
	}
	return fmt.Errorf("kubernetes: pod %s ended in phase %s with no terminated container status", pod.Name, pod.Status.Phase)
}

// actionContainerStatus returns the status of the Action's own container, identified by name, so
// an unrelated sidecar container (e.g. injected by a mesh/vault mutating webhook) is never
// mistaken for the Action's outcome. It returns the zero value if the container isn't found yet.
func actionContainerStatus(pod *corev1.Pod) corev1.ContainerState {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == actionContainerName {
			return cs.State
		}
	}
	return corev1.ContainerState{}
}

// streamLogs is best-effort: it's read after the Pod has already terminated and the Job/Pod are
// about to be deleted, so failures here shouldn't affect the Action's outcome. It deliberately
// uses its own bounded timeout rather than Execute's ctx (which may already be done) or an
// unbounded context.Background() (which could hang Execute, and therefore the single-threaded
// agent.Config.Run loop, forever if the log endpoint stalls).
func (c *Config) streamLogs(podName string) {
	ctx, cancel := context.WithTimeout(context.Background(), logStreamTimeout)
	defer cancel()

	rc, err := c.Client.CoreV1().Pods(c.Namespace).GetLogs(podName, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		c.Log.Info("failed to fetch pod logs", "pod", podName, "error", err)
		return
	}
	defer rc.Close()

	buf := make([]byte, 4096)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			c.Log.Info(string(buf[:n]), "pod", podName)
		}
		if err != nil {
			if err != io.EOF {
				c.Log.Error(err, "error reading pod logs", "pod", podName)
			}
			return
		}
	}
}

func convEnv(envs []spec.Env) []corev1.EnvVar {
	var out []corev1.EnvVar
	for _, e := range envs {
		out = append(out, corev1.EnvVar{Name: e.Key, Value: e.Value})
	}
	return out
}

var invalidJobNameChars = regexp.MustCompile(`[^a-z0-9-]`)

// jobName builds a DNS-1123 compliant Job name from an Action's ID and name. Truncation can
// produce name collisions across Actions, but that's cosmetic, not a correctness issue: Execute
// finds a Job's Pod back via the action-id label, not the name.
func jobName(actionID, name string) string {
	const maxLen = 63

	base := strings.ToLower(fmt.Sprintf("tinkerbell-%s-%s", name, actionID))
	base = invalidJobNameChars.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if len(base) > maxLen {
		base = strings.Trim(base[:maxLen], "-")
	}
	if base == "" {
		base = "tinkerbell-action"
	}

	return base
}
