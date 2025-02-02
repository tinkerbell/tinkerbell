package grpc

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
)

func TestModifyWorkflowState(t *testing.T) {
	cases := []struct {
		name           string
		inputWf        *v1alpha1.Workflow
		inputWfContext *proto.WorkflowContext
		want           *v1alpha1.Workflow
		wantErr        error
	}{
		{
			name:           "no workflow",
			inputWf:        nil,
			inputWfContext: &proto.WorkflowContext{},
			want:           nil,
			wantErr:        errors.New("no workflow provided"),
		},
		{
			name:           "no context",
			inputWf:        &v1alpha1.Workflow{},
			inputWfContext: nil,
			want:           nil,
			wantErr:        errors.New("no workflow context provided"),
		},
		{
			name: "no task",
			inputWf: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_PENDING",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:    "stream",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 300,
									Status:  "STATE_PENDING",
								},
							},
						},
					},
				},
			},
			inputWfContext: &proto.WorkflowContext{
				WorkflowId:           "debian",
				CurrentWorker:        "machine-mac-1",
				CurrentTask:          "power-on",
				CurrentAction:        "power-on-bmc",
				CurrentActionIndex:   0,
				CurrentActionState:   proto.State_STATE_RUNNING,
				TotalNumberOfActions: 1,
			},
			want:    nil,
			wantErr: errors.New("task not found"),
		},
		{
			name: "no action found",
			inputWf: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_PENDING",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:    "stream",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 300,
									Status:  "STATE_PENDING",
								},
							},
						},
					},
				},
			},
			inputWfContext: &proto.WorkflowContext{
				CurrentWorker:        "machine-mac-1",
				CurrentTask:          "provision",
				CurrentAction:        "power-on-bmc",
				CurrentActionIndex:   0,
				CurrentActionState:   proto.State_STATE_RUNNING,
				TotalNumberOfActions: 1,
			},
			want:    nil,
			wantErr: errors.New("action not found"),
		},
		{
			name: "running task",
			inputWf: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_PENDING",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:    "stream",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 300,
									Status:  "STATE_PENDING",
								},
							},
						},
					},
				},
			},
			inputWfContext: &proto.WorkflowContext{
				CurrentWorker:        "machine-mac-1",
				CurrentTask:          "provision",
				CurrentAction:        "stream",
				CurrentActionIndex:   0,
				CurrentActionState:   proto.State_STATE_RUNNING,
				TotalNumberOfActions: 1,
			},
			want: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_RUNNING",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:      "stream",
									Image:     "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:   300,
									Status:    "STATE_RUNNING",
									StartedAt: nil,
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "timed out task",
			inputWf: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_RUNNING",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:      "stream",
									Image:     "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:   300,
									Status:    "STATE_RUNNING",
									StartedAt: nil,
								},
							},
						},
					},
				},
			},
			inputWfContext: &proto.WorkflowContext{
				CurrentWorker:        "machine-mac-1",
				CurrentTask:          "provision",
				CurrentAction:        "stream",
				CurrentActionIndex:   0,
				CurrentActionState:   proto.State_STATE_TIMEOUT,
				TotalNumberOfActions: 1,
			},
			want: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_TIMEOUT",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:      "stream",
									Image:     "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:   300,
									Status:    "STATE_TIMEOUT",
									StartedAt: nil,
									Seconds:   301,
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "failed task",
			inputWf: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_RUNNING",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:      "stream",
									Image:     "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:   300,
									Status:    "STATE_RUNNING",
									StartedAt: nil,
								},
								{
									Name:    "kexec",
									Image:   "quay.io/tinkerbell-actions/kexec:v1.0.0",
									Timeout: 5,
									Status:  "STATE_PENDING",
								},
							},
						},
					},
				},
			},
			inputWfContext: &proto.WorkflowContext{
				CurrentWorker:        "machine-mac-1",
				CurrentTask:          "provision",
				CurrentAction:        "stream",
				CurrentActionIndex:   0,
				CurrentActionState:   proto.State_STATE_FAILED,
				TotalNumberOfActions: 2,
			},
			want: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_FAILED",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:      "stream",
									Image:     "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:   300,
									Status:    "STATE_FAILED",
									StartedAt: nil,
									Seconds:   30,
								},
								{
									Name:    "kexec",
									Image:   "quay.io/tinkerbell-actions/kexec:v1.0.0",
									Timeout: 5,
									Status:  "STATE_PENDING",
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "successful task",
			inputWf: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_RUNNING",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:      "stream",
									Image:     "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:   300,
									Status:    "STATE_RUNNING",
									StartedAt: nil,
								},
								{
									Name:    "kexec",
									Image:   "quay.io/tinkerbell-actions/kexec:v1.0.0",
									Timeout: 5,
									Status:  "STATE_PENDING",
								},
							},
						},
					},
				},
			},
			inputWfContext: &proto.WorkflowContext{
				CurrentWorker:        "machine-mac-1",
				CurrentTask:          "provision",
				CurrentAction:        "stream",
				CurrentActionIndex:   0,
				CurrentActionState:   proto.State_STATE_SUCCESS,
				TotalNumberOfActions: 2,
			},
			want: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_RUNNING",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:      "stream",
									Image:     "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:   300,
									Status:    "STATE_SUCCESS",
									StartedAt: nil,
									Seconds:   30,
								},
								{
									Name:    "kexec",
									Image:   "quay.io/tinkerbell-actions/kexec:v1.0.0",
									Timeout: 5,
									Status:  "STATE_PENDING",
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "successful last task",
			inputWf: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_RUNNING",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:      "stream",
									Image:     "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:   300,
									Status:    "STATE_SUCCESS",
									StartedAt: nil,
									Seconds:   27,
								},
								{
									Name:    "kexec",
									Image:   "quay.io/tinkerbell-actions/kexec:v1.0.0",
									Timeout: 5,
									Status:  "STATE_RUNNING",
								},
							},
						},
					},
				},
			},
			inputWfContext: &proto.WorkflowContext{
				CurrentWorker:        "machine-mac-1",
				CurrentTask:          "provision",
				CurrentAction:        "kexec",
				CurrentActionIndex:   1,
				CurrentActionState:   proto.State_STATE_SUCCESS,
				TotalNumberOfActions: 2,
			},
			want: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State:         "STATE_POST",
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:      "stream",
									Image:     "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:   300,
									Status:    "STATE_SUCCESS",
									StartedAt: nil,
									Seconds:   27,
								},
								{
									Name:    "kexec",
									Image:   "quay.io/tinkerbell-actions/kexec:v1.0.0",
									Timeout: 5,
									Status:  "STATE_SUCCESS",
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &Handler{
				Logger:            logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, nil)),
				BackendReadWriter: &mockBackendReadWriter{},
				NowFunc:           func() time.Time { return time.Time{} },
			}
			gotErr := server.modifyWorkflowState(tc.inputWf, tc.inputWfContext)
			compareErrors(t, gotErr, tc.wantErr)
			if tc.want == nil {
				return
			}

			if diff := cmp.Diff(tc.inputWf, tc.want, cmpopts.IgnoreFields(v1alpha1.Action{}, "Seconds", "StartedAt")); diff != "" {
				t.Errorf("unexpected difference:\n%v", diff)
			}
		})
	}
}

// compareErrors is a helper function for comparing an error value and a desired error.
func compareErrors(t *testing.T, got, want error) {
	t.Helper()
	if got != nil {
		if want == nil {
			t.Fatalf(`Got unexpected error: %v"`, got)
		} else if got.Error() != want.Error() {
			t.Fatalf(`Got unexpected error: got "%v" wanted "%v"`, got, want)
		}
		return
	}
	if got == nil && want != nil {
		t.Fatalf("Missing expected error: %v", want)
	}
}

type mockBackendReadWriter struct{}

func (m *mockBackendReadWriter) Read(_ context.Context, _, _ string) (*v1alpha1.Workflow, error) {
	return nil, nil
}

func (m *mockBackendReadWriter) ReadAll(_ context.Context, _ string) ([]v1alpha1.Workflow, error) {
	return nil, nil
}

func (m *mockBackendReadWriter) Write(_ context.Context, _ *v1alpha1.Workflow) error {
	return nil
}
