package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/cenkalti/backoff/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
	"google.golang.org/grpc"
)

type mockWorkflowServiceClient struct {
	GetActionFunc          func(ctx context.Context, req *proto.ActionRequest) (*proto.ActionResponse, error)
	ReportActionStatusFunc func(ctx context.Context, req *proto.ActionStatusRequest) (*proto.ActionStatusResponse, error)
}

func (m *mockWorkflowServiceClient) GetAction(ctx context.Context, req *proto.ActionRequest, _ ...grpc.CallOption) (*proto.ActionResponse, error) {
	return m.GetActionFunc(ctx, req)
}

func (m *mockWorkflowServiceClient) ReportActionStatus(ctx context.Context, req *proto.ActionStatusRequest, _ ...grpc.CallOption) (*proto.ActionStatusResponse, error) {
	return m.ReportActionStatusFunc(ctx, req)
}

var errTest = errors.New("failed to get action")

func TestRead(t *testing.T) {
	tests := map[string]struct {
		expectedSpec  spec.Action
		protoResponse *proto.ActionResponse
		expectedError error
		readError     error
	}{
		"Success": {
			expectedSpec: spec.Action{
				WorkerID:   "123",
				TaskID:     "456",
				WorkflowID: "789",
				ID:         "0123",
				Name:       "first action",
				Image:      "alpine",
				Args:       []string{"sleep", "5"},
				Env: []spec.Env{
					{
						Key:   "ENV_VAR",
						Value: "value",
					},
					{
						Key:   "UNSET_VAR",
						Value: "",
					},
				},
				Volumes:        []spec.Volume{"/var/lib:/var/lib"},
				Namespaces:     spec.Namespaces{},
				Retries:        0,
				TimeoutSeconds: 60,
			},
			protoResponse: &proto.ActionResponse{
				WorkflowId: toPtr("789"),
				TaskId:     toPtr("456"),
				WorkerId:   toPtr("123"),
				ActionId:   toPtr("0123"),
				Name:       toPtr("first action"),
				Image:      toPtr("alpine"),
				Timeout:    toPtr(int64(60)),
				Command:    []string{"sleep", "5"},
				Volumes:    []string{"/var/lib:/var/lib"},
				Environment: []string{
					"ENV_VAR=value",
					"UNSET_VAR",
				},
			},
		},
		"Error": {
			expectedSpec:  spec.Action{},
			protoResponse: nil,
			readError:     errTest,
			expectedError: errTest,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockClient := &mockWorkflowServiceClient{
				GetActionFunc: func(_ context.Context, _ *proto.ActionRequest) (*proto.ActionResponse, error) {
					return test.protoResponse, test.readError
				},
			}

			config := &Config{
				TinkServerClient: mockClient,
				WorkerID:         "worker-123",
				RetryOptions: []backoff.RetryOption{
					backoff.WithMaxTries(1),
				},
			}

			ctx := context.Background()
			got, err := config.Read(ctx)
			if test.expectedError != nil {
				if !errors.Is(err, test.expectedError) {
					t.Fatalf("expected error: %v, got: %v", test.expectedError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				if diff := cmp.Diff(got, test.expectedSpec); diff != "" {
					t.Errorf("unexpected difference:\n%v", diff)
				}
			}
		})
	}
}

func TestWrite(t *testing.T) {
	tests := map[string]struct {
		expectedError error
	}{
		"Success": {
			expectedError: nil,
		},
		"Error": {
			expectedError: errors.New("failed to report action"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockClient := &mockWorkflowServiceClient{
				ReportActionStatusFunc: func(_ context.Context, _ *proto.ActionStatusRequest) (*proto.ActionStatusResponse, error) {
					return nil, test.expectedError
				},
			}

			config := &Config{
				TinkServerClient: mockClient,
				WorkerID:         "worker-123",
				RetryOptions: []backoff.RetryOption{
					backoff.WithMaxTries(1),
				},
			}

			ctx := context.Background()
			err := config.Write(ctx, spec.Event{State: spec.StateRunning})
			if test.expectedError != nil {
				if !errors.Is(err, test.expectedError) {
					t.Fatalf("expected error: %v, got: %v", test.expectedError, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestNewClientConn(t *testing.T) {
	tests := map[string]struct {
		address string
		wantErr bool
	}{
		"no error": {
			address: "localhost:8080",
			wantErr: false,
		},
		"error": {
			address: "",
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			conn, err := NewClientConn(test.address, false, false)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				if conn == nil {
					t.Fatalf("expected non-nil connection")
				}
			}
		})
	}
}
