package kubernetes

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestJobFor(t *testing.T) {
	tests := map[string]struct {
		action             spec.Action
		serviceAccountName string
		want               *batchv1.Job
		wantErr            bool
	}{
		"basic": {
			action: spec.Action{
				ID:    "abc-123",
				Name:  "my-action",
				Image: "example.com/image:v1",
				Cmd:   "/bin/sh",
				Args:  []string{"-c", "echo hi"},
				Env:   []spec.Env{{Key: "FOO", Value: "bar"}},
			},
			want: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tinkerbell-my-action-abc-123",
					Namespace: "tinkerbell",
					Labels:    map[string]string{actionIDLabel: "abc-123"},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: int32Ptr(0),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{actionIDLabel: "abc-123"},
						},
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyNever,
							AutomountServiceAccountToken: boolPtr(false),
							Containers: []corev1.Container{
								{
									Name:    "action",
									Image:   "example.com/image:v1",
									Command: []string{"/bin/sh"},
									Args:    []string{"-c", "echo hi"},
									Env:     []corev1.EnvVar{{Name: "FOO", Value: "bar"}},
								},
							},
						},
					},
				},
			},
		},
		"no cmd or args": {
			action: spec.Action{ID: "id2", Name: "n2", Image: "img"},
			want: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tinkerbell-n2-id2",
					Namespace: "tinkerbell",
					Labels:    map[string]string{actionIDLabel: "id2"},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: int32Ptr(0),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{actionIDLabel: "id2"}},
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyNever,
							AutomountServiceAccountToken: boolPtr(false),
							Containers:                   []corev1.Container{{Name: "action", Image: "img"}},
						},
					},
				},
			},
		},
		"explicit service account name is set on the pod": {
			action:             spec.Action{ID: "id6", Name: "n6", Image: "img"},
			serviceAccountName: "tink-agent",
			want: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tinkerbell-n6-id6",
					Namespace: "tinkerbell",
					Labels:    map[string]string{actionIDLabel: "id6"},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: int32Ptr(0),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{actionIDLabel: "id6"}},
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyNever,
							ServiceAccountName:           "tink-agent",
							AutomountServiceAccountToken: boolPtr(false),
							Containers:                   []corev1.Container{{Name: "action", Image: "img"}},
						},
					},
				},
			},
		},
		"timeout sets ActiveDeadlineSeconds on the job": {
			action: spec.Action{ID: "id7", Name: "n7", Image: "img", TimeoutSeconds: 90},
			want: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tinkerbell-n7-id7",
					Namespace: "tinkerbell",
					Labels:    map[string]string{actionIDLabel: "id7"},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit:          int32Ptr(0),
					ActiveDeadlineSeconds: int64Ptr(90),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{actionIDLabel: "id7"}},
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyNever,
							AutomountServiceAccountToken: boolPtr(false),
							Containers:                   []corev1.Container{{Name: "action", Image: "img"}},
						},
					},
				},
			},
		},
		"host pid namespace rejected": {
			action:  spec.Action{ID: "id3", Namespaces: spec.Namespaces{PID: "host"}},
			wantErr: true,
		},
		"host network namespace rejected": {
			action:  spec.Action{ID: "id4", Namespaces: spec.Namespaces{Network: "host"}},
			wantErr: true,
		},
		"volumes rejected": {
			action:  spec.Action{ID: "id5", Volumes: []spec.Volume{"/etc/data:/data:ro"}},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Config{Namespace: "tinkerbell", ServiceAccountName: tt.serviceAccountName}
			got, err := c.jobFor(tt.action)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("unexpected job (-want +got):\n%s", diff)
			}
		})
	}
}

func TestJobNameTruncation(t *testing.T) {
	longName := "this-is-a-very-long-action-name-that-well-exceeds-the-limit-imposed-by-kubernetes"
	name := jobName("some-id", longName)
	if len(name) > 63 {
		t.Fatalf("job name %q exceeds 63 chars: %d", name, len(name))
	}
}

func TestExecute(t *testing.T) {
	tests := map[string]struct {
		pod     *corev1.Pod
		wantErr bool
	}{
		"success": {
			pod: podWithContainerState("action-1", "tinkerbell", corev1.PodSucceeded, &corev1.ContainerStateTerminated{ExitCode: 0}),
		},
		"non-zero exit code": {
			pod:     podWithContainerState("action-2", "tinkerbell", corev1.PodFailed, &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}),
			wantErr: true,
		},
		"failed with no container status": {
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-action-3",
					Namespace: "tinkerbell",
					Labels:    map[string]string{actionIDLabel: "action-3"},
				},
				Status: corev1.PodStatus{Phase: corev1.PodFailed},
			},
			wantErr: true,
		},
		"success despite a terminated sidecar container listed first": {
			pod: podWithContainers("action-5", "tinkerbell", corev1.PodSucceeded,
				corev1.ContainerStatus{Name: "istio-proxy", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}}},
				corev1.ContainerStatus{Name: actionContainerName, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
			),
		},
		"failure from the action container despite a healthy sidecar listed first": {
			pod: podWithContainers("action-6", "tinkerbell", corev1.PodFailed,
				corev1.ContainerStatus{Name: "istio-proxy", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
				corev1.ContainerStatus{Name: actionContainerName, State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}}},
			),
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			actionID := tt.pod.Labels[actionIDLabel]
			client := fake.NewSimpleClientset(tt.pod)
			c := &Config{Log: logr.Discard(), Client: client, Namespace: "tinkerbell"}

			err := c.Execute(context.Background(), spec.Action{ID: actionID, Name: "n", Image: "img"})
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// The Job must be cleaned up regardless of outcome.
			jobs, jerr := client.BatchV1().Jobs("tinkerbell").List(context.Background(), metav1.ListOptions{})
			if jerr != nil {
				t.Fatalf("listing jobs: %v", jerr)
			}
			if len(jobs.Items) != 0 {
				t.Fatalf("expected job to be deleted, found %d", len(jobs.Items))
			}
		})
	}
}

func TestExecute_ContextCancelled(t *testing.T) {
	client := fake.NewSimpleClientset()
	c := &Config{Log: logr.Discard(), Client: client, Namespace: "tinkerbell"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Execute(ctx, spec.Action{ID: "action-4", Name: "n", Image: "img"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	jobs, jerr := client.BatchV1().Jobs("tinkerbell").List(context.Background(), metav1.ListOptions{})
	if jerr != nil {
		t.Fatalf("listing jobs: %v", jerr)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("expected job to be deleted, found %d", len(jobs.Items))
	}
}

// TestWaitForPod_ReconnectsAfterWatchCloses verifies that a watch closing for a benign reason
// (API server watch timeout, resourceVersion expiry, transient network blip) is not treated as a
// fatal error: waitForPod must re-list and re-establish the watch instead of failing a
// long-running Action just because its watch aged out.
func TestWaitForPod_ReconnectsAfterWatchCloses(t *testing.T) {
	runningPod := podWithContainerState("action-7", "tinkerbell", corev1.PodRunning, nil)
	terminatedPod := podWithContainerState("action-7", "tinkerbell", corev1.PodSucceeded, &corev1.ContainerStateTerminated{ExitCode: 0})

	client := fake.NewSimpleClientset(runningPod)

	var watches atomic.Int32
	client.PrependWatchReactor("pods", func(_ clienttesting.Action) (bool, watch.Interface, error) {
		fw := watch.NewFake()
		if watches.Add(1) == 1 {
			// First watch: close immediately, simulating a benign disconnect.
			fw.Stop()
			return true, fw, nil
		}
		// Second watch: deliver the terminated pod.
		go fw.Modify(terminatedPod)
		return true, fw, nil
	})

	c := &Config{Log: logr.Discard(), Client: client, Namespace: "tinkerbell"}
	got, err := c.waitForPod(context.Background(), "action-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !podTerminated(got) {
		t.Fatalf("expected a terminated pod, got %+v", got)
	}
	if n := watches.Load(); n < 2 {
		t.Fatalf("expected waitForPod to reconnect after the first watch closed, got %d watch call(s)", n)
	}
}

// TestDeleteJob verifies that deleteJob blocks until the Job actually disappears rather than
// returning as soon as the Delete call is acknowledged (foreground-propagation Delete only marks
// the Job for deletion; the garbage collector removes it asynchronously). Without this, a retried
// Action reusing the same deterministic Job name could race its own predecessor's still-terminating
// Job with an AlreadyExists error.
func TestDeleteJob(t *testing.T) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "j1", Namespace: "tinkerbell"}}
	client := fake.NewSimpleClientset(job)

	var gets atomic.Int32
	client.PrependReactor("get", "jobs", func(_ clienttesting.Action) (bool, runtime.Object, error) {
		if gets.Add(1) == 1 {
			// First poll: still exists, so deleteJob must keep waiting.
			return true, job, nil
		}
		return true, nil, apierrors.NewNotFound(batchv1.Resource("jobs"), "j1")
	})

	c := &Config{Log: logr.Discard()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	c.deleteJob(ctx, client.BatchV1().Jobs("tinkerbell"), "j1")
	elapsed := time.Since(start)

	if gets.Load() < 2 {
		t.Fatalf("expected deleteJob to poll Get at least twice, got %d", gets.Load())
	}
	if elapsed < 400*time.Millisecond {
		t.Fatalf("deleteJob returned after %v, expected it to wait for the poll interval before the job disappeared", elapsed)
	}
}

func podWithContainerState(actionID, namespace string, phase corev1.PodPhase, terminated *corev1.ContainerStateTerminated) *corev1.Pod {
	return podWithContainers(actionID, namespace, phase,
		corev1.ContainerStatus{Name: actionContainerName, State: corev1.ContainerState{Terminated: terminated}},
	)
}

func podWithContainers(actionID, namespace string, phase corev1.PodPhase, statuses ...corev1.ContainerStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-" + actionID,
			Namespace: namespace,
			Labels:    map[string]string{actionIDLabel: actionID},
		},
		Status: corev1.PodStatus{
			Phase:             phase,
			ContainerStatuses: statuses,
		},
	}
}

func int32Ptr(v int32) *int32 { return &v }
