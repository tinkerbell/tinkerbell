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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
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
	// jobDeleteTimeout bounds how long the deferred cleanup in Execute waits for a deleted Job to
	// actually disappear (foreground-propagation Delete only marks it for deletion; the garbage
	// collector removes it asynchronously). Like logStreamTimeout, it uses its own bounded context
	// rather than Execute's ctx, which may already be cancelled/expired by this point.
	jobDeleteTimeout = 30 * time.Second
)

// Config is a RuntimeExecutor that runs Actions as Kubernetes Jobs.
type Config struct {
	Log       logr.Logger
	Client    kubernetes.Interface // interface, not *kubernetes.Clientset, so it can be faked in tests
	Namespace string               // fixed namespace Jobs/Pods are created in
	// ServiceAccountName is the ServiceAccount Job Pods run as. Kubernetes does not propagate the
	// identity of the caller that creates an object to that object, so leaving this empty does NOT
	// mean the Job inherits the Agent's own ServiceAccount: it means the Job Pod runs as the
	// namespace's "default" ServiceAccount instead, which may not carry the imagePullSecrets a
	// private image needs. Set this to the Agent's own ServiceAccount name to reuse its
	// imagePullSecrets.
	ServiceAccountName string
}

// Opt configures a Config returned by NewConfig.
type Opt func(*Config)

// WithClient overrides the Kubernetes client used, for example with a fake clientset in tests.
func WithClient(c kubernetes.Interface) Opt {
	return func(cfg *Config) { cfg.Client = c }
}

// WithServiceAccountName sets the ServiceAccount Job Pods run as. See Config.ServiceAccountName.
func WithServiceAccountName(name string) Opt {
	return func(cfg *Config) { cfg.ServiceAccountName = name }
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
// always deleted, and confirmed gone, before Execute returns, so a retried Action (same ID, same
// deterministic Job name) never races its own predecessor's still-terminating Job. If a Job with
// that name already exists — a leftover from a previous Agent process that crashed or restarted
// before its own cleanup ran — Execute adopts it via adoptExistingJob instead of failing outright.
func (c *Config) Execute(ctx context.Context, a spec.Action) error {
	job, err := c.jobFor(a)
	if err != nil {
		return err
	}

	jobs := c.Client.BatchV1().Jobs(c.Namespace)
	created, err := jobs.Create(ctx, job, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		created, err = c.adoptExistingJob(ctx, jobs, job.Name, a.ID, err)
	}
	if err != nil {
		return fmt.Errorf("kubernetes: creating job: %w", err)
	}

	defer func() {
		// Always clean up, regardless of outcome. Use a background context since ctx may
		// already be cancelled/expired.
		delCtx, cancel := context.WithTimeout(context.Background(), jobDeleteTimeout)
		defer cancel()
		c.deleteJob(delCtx, jobs, created.Name)
	}()

	pod, err := c.waitForPod(ctx, a.ID)
	if err != nil {
		return err
	}

	c.streamLogs(pod.Name)

	return exitError(pod)
}

// adoptExistingJob recovers from a Jobs.Create AlreadyExists conflict by checking whether the
// existing Job belongs to this same Action (its action-id label matches). If so, it's a leftover
// from a previous Agent process that crashed or restarted before its deferred cleanup could run:
// Execute can safely observe it instead of failing the Action permanently just because its
// deterministic name (see jobName) is already taken. If the existing Job belongs to a different
// Action — a rare name-truncation collision — or can't be fetched, createErr is returned unchanged
// so the caller never touches a Job it doesn't own.
func (c *Config) adoptExistingJob(ctx context.Context, jobs batchv1client.JobInterface, name, actionID string, createErr error) (*batchv1.Job, error) {
	existing, err := jobs.Get(ctx, name, metav1.GetOptions{})
	if err != nil || existing.Labels[actionIDLabel] != actionID {
		return nil, createErr
	}
	c.Log.Info("adopting existing job left over from a previous attempt", "job", existing.Name)
	return existing, nil
}

// deleteJob issues a foreground-propagation delete and then waits for the Job to actually
// disappear (foreground propagation only marks it for deletion; the garbage collector removes it
// asynchronously), so the caller can rely on the Job name being free again once this returns.
// Errors are logged, not returned: cleanup is best-effort and must never fail an Action that
// otherwise already succeeded or failed on its own terms.
func (c *Config) deleteJob(ctx context.Context, jobs batchv1client.JobInterface, name string) {
	policy := metav1.DeletePropagationForeground
	if err := jobs.Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &policy}); err != nil && !apierrors.IsNotFound(err) {
		c.Log.Info("failed to delete job", "job", name, "error", err)
		return
	}

	err := wait.PollUntilContextCancel(ctx, 500*time.Millisecond, true, func(ctx context.Context) (bool, error) {
		_, err := jobs.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		c.Log.Info("timed out waiting for job deletion to complete", "job", name, "error", err)
	}
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
			// A backstop independent of the Agent's own liveness: if the Agent pod is OOM-killed,
			// crashes, or restarts mid-Execute, this Job would otherwise be orphaned with nothing
			// enforcing the Action's timeout or cleaning it up. nil (unset) if the Action has no
			// timeout, matching the Agent's own ctx-based behavior.
			ActiveDeadlineSeconds: activeDeadlineSeconds(a),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{actionIDLabel: a.ID},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers:         []corev1.Container{container},
					ServiceAccountName: c.ServiceAccountName,
					// Action containers never need Kubernetes API access themselves (only the
					// Agent does), so don't hand them an API token regardless of which
					// ServiceAccount they run as.
					AutomountServiceAccountToken: boolPtr(false),
					// No SecurityContext/Privileged: deliberately the Kubernetes default
					// (unprivileged), unlike docker.go/containerd.go which both set Privileged
					// unconditionally for the bare-metal case this runtime isn't used for.
				},
			},
		},
	}, nil
}

// activeDeadlineSeconds returns nil if the Action has no timeout, so the Job never gets an
// ActiveDeadlineSeconds it wasn't asked for.
func activeDeadlineSeconds(a spec.Action) *int64 {
	if a.TimeoutSeconds <= 0 {
		return nil
	}
	return int64Ptr(int64(a.TimeoutSeconds))
}

// watchReconnectBackoff bounds how long waitForPod waits before re-listing and re-establishing a
// watch after one closes for a benign reason, so a persistent connectivity problem can't turn into
// a tight List/Watch loop hammering the API server.
const watchReconnectBackoff = time.Second

// waitForPod blocks until the Action's Job Pod reaches a terminal state (a container has
// terminated, or the Pod itself failed/succeeded without ever reporting a container status, e.g.
// ImagePullBackOff) and returns it. A watch closing on its own — API server watch timeouts,
// resourceVersion expiry, and transient network interruptions all do this routinely — is not
// treated as fatal: waitForPod re-lists and re-establishes the watch instead, so a long-running
// Action can't fail spuriously just because its watch aged out.
func (c *Config) waitForPod(ctx context.Context, actionID string) (*corev1.Pod, error) {
	pods := c.Client.CoreV1().Pods(c.Namespace)
	selector := fmt.Sprintf("%s=%s", actionIDLabel, actionID)

	for {
		// Check first in case the Pod already reached a terminal state before the watch below is
		// (re-)established (e.g. a very fast-exiting container).
		list, err := pods.List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return nil, fmt.Errorf("kubernetes: listing pods: %w", err)
		}
		for i := range list.Items {
			if podTerminated(&list.Items[i]) {
				return &list.Items[i], nil
			}
		}

		pod, closed, err := c.watchForTerminatedPod(ctx, pods, selector, list.ResourceVersion)
		if err != nil {
			return nil, err
		}
		if !closed {
			return pod, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(watchReconnectBackoff):
		}
	}
}

// watchForTerminatedPod watches until the Action's Pod terminates or the watch itself closes.
// closed is true only in the latter case, in which case pod and err are both nil. A watch.Error
// event (e.g. a 410 Gone from a resourceVersion that aged out) is treated the same as a closed
// channel rather than silently ignored: waitForPod re-lists and re-establishes the watch after a
// short backoff, instead of waiting out the rest of ctx for no reason.
func (c *Config) watchForTerminatedPod(ctx context.Context, pods corev1client.PodInterface, selector, resourceVersion string) (pod *corev1.Pod, closed bool, err error) {
	w, err := pods.Watch(ctx, metav1.ListOptions{LabelSelector: selector, ResourceVersion: resourceVersion})
	if err != nil {
		return nil, false, fmt.Errorf("kubernetes: watching pods: %w", err)
	}
	defer w.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case event, ok := <-w.ResultChan():
			if !ok {
				return nil, true, nil
			}
			if event.Type == watch.Error {
				c.Log.Info("pod watch reported an error, reconnecting", "error", apierrors.FromObject(event.Object))
				return nil, true, nil
			}
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			if podTerminated(pod) {
				return pod, false, nil
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

	rc, err := c.Client.CoreV1().Pods(c.Namespace).GetLogs(podName, &corev1.PodLogOptions{Container: actionContainerName}).Stream(ctx)
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

func boolPtr(v bool) *bool { return &v }

func int64Ptr(v int64) *int64 { return &v }

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
