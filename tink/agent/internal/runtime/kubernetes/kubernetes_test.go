package kubernetes

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestJobFor(t *testing.T) {
	tests := map[string]struct {
		action  spec.Action
		want    *batchv1.Job
		wantErr bool
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
							RestartPolicy: corev1.RestartPolicyNever,
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
							RestartPolicy: corev1.RestartPolicyNever,
							Containers:    []corev1.Container{{Name: "action", Image: "img"}},
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
			c := &Config{Namespace: "tinkerbell"}
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
