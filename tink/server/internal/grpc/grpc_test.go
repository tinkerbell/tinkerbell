package grpc

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetAction(t *testing.T) {
	cases := map[string]struct {
		workflow *tinkerbell.Workflow
		request  *proto.ActionRequest
		want     *proto.ActionResponse
		wantErr  error
	}{
		"successful second Action in Task": {
			request: &proto.ActionRequest{
				AgentId: toPtr("machine-mac-1"),
			},
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Status: tinkerbell.WorkflowStatus{
					State: tinkerbell.WorkflowStateRunning,
					CurrentState: &tinkerbell.CurrentState{
						AgentID:    "machine-mac-1",
						TaskID:     "provision",
						ActionID:   "stream",
						State:      tinkerbell.WorkflowStateSuccess,
						ActionName: "stream",
					},
					GlobalTimeout: 600,
					Tasks: []tinkerbell.Task{
						{
							Name:    "provision",
							AgentID: "machine-mac-1",
							ID:      "provision",
							Actions: []tinkerbell.Action{
								{
									Name:              "stream",
									Image:             "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:           300,
									State:             tinkerbell.WorkflowStateSuccess,
									ExecutionStart:    nil,
									ExecutionDuration: "30s",
									ID:                "stream",
								},
								{
									Name:    "kexec",
									Image:   "quay.io/tinkerbell-actions/kexec:v1.0.0",
									Timeout: 5,
									State:   tinkerbell.WorkflowStatePending,
									ID:      "kexec",
								},
							},
						},
					},
				},
			},
			want: &proto.ActionResponse{
				WorkflowId:  toPtr("default/machine1"),
				AgentId:     toPtr("machine-mac-1"),
				TaskId:      toPtr("provision"),
				ActionId:    toPtr("kexec"),
				Name:        toPtr("kexec"),
				Image:       toPtr("quay.io/tinkerbell-actions/kexec:v1.0.0"),
				Timeout:     toPtr(int64(5)),
				Environment: []string{},
				Pid:         new(string),
			},
			wantErr: nil,
		},
		"successful first Action in Task": {
			request: &proto.ActionRequest{
				AgentId: toPtr("machine-mac-1"),
			},
			want: &proto.ActionResponse{
				WorkflowId:  toPtr("default/machine1"),
				AgentId:     toPtr("machine-mac-1"),
				TaskId:      new(string),
				ActionId:    new(string),
				Name:        toPtr("stream"),
				Image:       toPtr("quay.io/tinkerbell-actions/image2disk:v1.0.0"),
				Timeout:     toPtr(int64(300)),
				Environment: []string{},
				Pid:         new(string),
			},
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Status: tinkerbell.WorkflowStatus{
					State:         tinkerbell.WorkflowStateRunning,
					GlobalTimeout: 600,
					Tasks: []tinkerbell.Task{
						{
							Name:    "provision",
							AgentID: "machine-mac-1",
							Actions: []tinkerbell.Action{
								{
									Name:              "stream",
									Image:             "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:           300,
									State:             tinkerbell.WorkflowStatePending,
									ExecutionStart:    nil,
									ExecutionDuration: "30s",
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		"workflow with no Tasks": {
			request: &proto.ActionRequest{
				AgentId: toPtr("machine-mac-1"),
			},
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Status: tinkerbell.WorkflowStatus{
					State:         tinkerbell.WorkflowStateRunning,
					GlobalTimeout: 600,
					Tasks:         []tinkerbell.Task{},
				},
			},
			wantErr: status.Errorf(codes.NotFound, "no Tasks found in Workflow"),
		},
		"no workflows found": {
			request: &proto.ActionRequest{
				AgentId: toPtr("machine-mac-1"),
			},
			wantErr: status.Errorf(codes.NotFound, "no Workflows found"),
		},
		"no agent id": {
			request: &proto.ActionRequest{},
			wantErr: status.Errorf(codes.InvalidArgument, "invalid Agent ID"),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := &Handler{
				Logger:            logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, nil)),
				BackendReadWriter: &mockBackendReadWriter{workflow: tc.workflow},
				NowFunc:           func() time.Time { return time.Time{} },
				RetryOptions:      []backoff.RetryOption{backoff.WithMaxTries(1)},
			}

			resp, gotErr := server.GetAction(context.Background(), tc.request)
			compareErrors(t, gotErr, tc.wantErr)
			if tc.want == nil {
				return
			}

			if diff := cmp.Diff(resp, tc.want, cmpopts.IgnoreUnexported(proto.ActionResponse{})); diff != "" {
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
	if want != nil {
		t.Fatalf("Missing expected error: %v", want)
	}
}

type mockBackendReadWriter struct {
	workflow *tinkerbell.Workflow
}

func (m *mockBackendReadWriter) Read(_ context.Context, _, _ string) (*tinkerbell.Workflow, error) {
	return m.workflow, nil
}

func (m *mockBackendReadWriter) ReadAll(_ context.Context, _ string) ([]tinkerbell.Workflow, error) {
	if m.workflow != nil {
		return []tinkerbell.Workflow{*m.workflow}, nil
	}
	return []tinkerbell.Workflow{}, nil
}

func (m *mockBackendReadWriter) Update(_ context.Context, _ *tinkerbell.Workflow) error {
	return nil
}

type mockBackendReadWriterForReport struct {
	workflow *tinkerbell.Workflow
	writeErr error
}

func (m *mockBackendReadWriterForReport) Read(_ context.Context, _, _ string) (*tinkerbell.Workflow, error) {
	if m.workflow == nil {
		return nil, errors.New("workflow not found")
	}
	return m.workflow, nil
}

func (m *mockBackendReadWriterForReport) ReadAll(_ context.Context, _ string) ([]tinkerbell.Workflow, error) {
	return nil, nil
}

func (m *mockBackendReadWriterForReport) Update(_ context.Context, _ *tinkerbell.Workflow) error {
	return m.writeErr
}

func TestReportActionStatus(t *testing.T) {
	tests := map[string]struct {
		request      *proto.ActionStatusRequest
		workflow     *tinkerbell.Workflow
		writeErr     error
		expectedResp *proto.ActionStatusResponse
		expectedErr  error
	}{
		"success": {
			request: &proto.ActionStatusRequest{
				WorkflowId:        toPtr("default/workflow1"),
				TaskId:            toPtr("task1"),
				ActionId:          toPtr("action1"),
				ActionState:       toPtr(proto.ActionStatusRequest_SUCCESS),
				ExecutionStart:    timestamppb.New(time.Now()),
				ExecutionDuration: toPtr("30s"),
				Message: &proto.ActionMessage{
					Message: toPtr("Action completed successfully"),
				},
			},
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow1",
					Namespace: "default",
				},
				Status: tinkerbell.WorkflowStatus{
					Tasks: []tinkerbell.Task{
						{
							ID: "task1",
							Actions: []tinkerbell.Action{
								{
									ID:    "action1",
									State: tinkerbell.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			writeErr:     nil,
			expectedErr:  nil,
			expectedResp: &proto.ActionStatusResponse{},
		},
		"write error": {
			request: &proto.ActionStatusRequest{
				WorkflowId:        toPtr("default/workflow6"),
				TaskId:            toPtr("task1"),
				ActionId:          toPtr("action1"),
				ActionState:       toPtr(proto.ActionStatusRequest_SUCCESS),
				ExecutionStart:    timestamppb.New(time.Now()),
				ExecutionDuration: toPtr("30s"),
				Message: &proto.ActionMessage{
					Message: toPtr("Action completed successfully"),
				},
			},
			workflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow1",
					Namespace: "default",
				},
				Status: tinkerbell.WorkflowStatus{
					Tasks: []tinkerbell.Task{
						{
							ID: "task1",
							Actions: []tinkerbell.Action{
								{
									ID:    "action1",
									State: tinkerbell.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			writeErr:    errors.New("write error"),
			expectedErr: status.Errorf(codes.Internal, "error writing report status: write error"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &Handler{
				BackendReadWriter: &mockBackendReadWriterForReport{
					workflow: tc.workflow,
					writeErr: tc.writeErr,
				},
				RetryOptions: []backoff.RetryOption{backoff.WithMaxTries(1)},
			}

			resp, err := handler.ReportActionStatus(context.Background(), tc.request)

			if diff := cmp.Diff(tc.expectedResp, resp, protocmp.Transform()); diff != "" {
				t.Errorf("unexpected response (-want +got):\n%s", diff)
			}

			if tc.expectedErr != nil {
				if err == nil || err.Error() != tc.expectedErr.Error() {
					t.Errorf("unexpected error: \ngot:  %v\nwant: %v", err, tc.expectedErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
