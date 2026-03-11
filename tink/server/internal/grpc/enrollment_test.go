package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/cenkalti/backoff/v5"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEnroll(t *testing.T) {
	tests := map[string]struct {
		workerID          string
		attributes        *proto.AgentAttributes
		mockCapabilities  *mockAutoCapabilities
		expectedErrorCode codes.Code
	}{
		"successful enrollment": {
			workerID: "worker-123",
			attributes: &proto.AgentAttributes{
				Chassis: &proto.Chassis{Serial: toPtr("12345")},
			},
			mockCapabilities: &mockAutoCapabilities{
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.ReadListOptions) ([]tinkerbell.WorkflowRuleSet, error) {
					return []tinkerbell.WorkflowRuleSet{
						{
							Spec: tinkerbell.WorkflowRuleSetSpec{
								Rules: []string{`{"chassis": {"serial": ["12345"]}}`},
								Workflow: tinkerbell.WorkflowRuleSetWorkflow{
									Namespace:     "default",
									AddAttributes: true,
								},
							},
						},
					}, nil
				},
				CreateWorkflowFunc: func(_ context.Context, _ *tinkerbell.Workflow) error {
					return nil
				},
			},
			expectedErrorCode: codes.NotFound,
		},
		"no matching workflow rule set": {
			workerID: "worker-123",
			attributes: &proto.AgentAttributes{
				Chassis: &proto.Chassis{Serial: toPtr("12345")},
			},
			mockCapabilities: &mockAutoCapabilities{
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.ReadListOptions) ([]tinkerbell.WorkflowRuleSet, error) {
					return nil, nil
				},
			},
			expectedErrorCode: codes.NotFound,
		},
		"error reading workflow rule sets": {
			workerID: "worker-123",
			attributes: &proto.AgentAttributes{
				Chassis: &proto.Chassis{Serial: toPtr("12345")},
			},
			mockCapabilities: &mockAutoCapabilities{
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.ReadListOptions) ([]tinkerbell.WorkflowRuleSet, error) {
					return nil, errors.New("failed to read workflow rule sets")
				},
			},
			expectedErrorCode: codes.Internal,
		},
		"error no patterns matched": {
			workerID: "worker-123",
			attributes: &proto.AgentAttributes{
				Chassis: &proto.Chassis{Serial: toPtr("12345")},
			},
			mockCapabilities: &mockAutoCapabilities{
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.ReadListOptions) ([]tinkerbell.WorkflowRuleSet, error) {
					return []tinkerbell.WorkflowRuleSet{
						{
							Spec: tinkerbell.WorkflowRuleSetSpec{
								Rules: []string{`{"chassis": {"serial": ["67890"]}}`},
							},
						},
					}, nil
				},
			},
			expectedErrorCode: codes.NotFound,
		},
		"error bad pattern": {
			workerID: "worker-123",
			attributes: &proto.AgentAttributes{
				Chassis: &proto.Chassis{Serial: toPtr("12345")},
			},
			mockCapabilities: &mockAutoCapabilities{
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.ReadListOptions) ([]tinkerbell.WorkflowRuleSet, error) {
					return []tinkerbell.WorkflowRuleSet{
						{
							Spec: tinkerbell.WorkflowRuleSetSpec{
								Rules: []string{`im a bad pattern`},
							},
						},
					}, nil
				},
			},
			expectedErrorCode: codes.NotFound,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &Handler{
				AutoCapabilities: AutoCapabilities{
					Enrollment: AutoEnrollment{
						Enabled:               true,
						WorkflowRuleSetLister: tt.mockCapabilities,
						WorkflowCreator:       tt.mockCapabilities,
					},
				},
				Backend: &mockBackendReadWriter{},
				RetryOptions: []backoff.RetryOption{
					backoff.WithMaxTries(1),
				},
			}

			_, err := handler.enroll(context.Background(), tt.workerID, convert(tt.attributes), nil)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got %v", err)
			}

			if st.Code() != tt.expectedErrorCode {
				t.Errorf("expected error code %v, got %v", tt.expectedErrorCode, st.Code())
			}
		})
	}
}

type mockAutoCapabilities struct {
	ListWorkflowRuleSetsFunc func(ctx context.Context, opts data.ReadListOptions) ([]tinkerbell.WorkflowRuleSet, error)
	CreateWorkflowFunc       func(ctx context.Context, wf *tinkerbell.Workflow) error
}

func (m *mockAutoCapabilities) ListWorkflowRuleSets(ctx context.Context, opts data.ReadListOptions) ([]tinkerbell.WorkflowRuleSet, error) {
	return m.ListWorkflowRuleSetsFunc(ctx, opts)
}

func (m *mockAutoCapabilities) CreateWorkflow(ctx context.Context, wf *tinkerbell.Workflow) error {
	return m.CreateWorkflowFunc(ctx, wf)
}
