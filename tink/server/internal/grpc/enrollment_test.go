package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/cenkalti/backoff/v5"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEnroll(t *testing.T) {
	tests := map[string]struct {
		workerID              string
		attributes            *proto.AgentAttributes
		mockCapabilities      *mockAutoCapabilities
		mockBackendReadWriter *mockReadUpdater
		expectedErrorCode     codes.Code
	}{
		"successful enrollment": {
			workerID: "worker-123",
			attributes: &proto.AgentAttributes{
				Chassis: &proto.Chassis{Serial: toPtr("12345")},
			},
			mockCapabilities: &mockAutoCapabilities{
				ReadAllWorkflowRuleSetsFunc: func(_ context.Context) ([]tinkerbell.WorkflowRuleSet, error) {
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
				CreateFunc: func(_ context.Context, _ *tinkerbell.Workflow) error {
					return nil
				},
			},
			mockBackendReadWriter: &mockReadUpdater{
				ReadAllFunc: func(_ context.Context, _ string) ([]tinkerbell.Workflow, error) {
					return []tinkerbell.Workflow{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "worker-123",
								Namespace: "default",
							},
							Spec: tinkerbell.WorkflowSpec{},
						},
					}, nil
				},
				ReadFunc: func(_ context.Context, _, _ string) (*tinkerbell.Workflow, error) {
					return &tinkerbell.Workflow{}, nil
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
				ReadAllWorkflowRuleSetsFunc: func(_ context.Context) ([]tinkerbell.WorkflowRuleSet, error) {
					return nil, nil
				},
			},
			mockBackendReadWriter: &mockReadUpdater{
				ReadFunc: func(_ context.Context, _, _ string) (*tinkerbell.Workflow, error) {
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
				ReadAllWorkflowRuleSetsFunc: func(_ context.Context) ([]tinkerbell.WorkflowRuleSet, error) {
					return nil, errors.New("failed to read workflow rule sets")
				},
			},
			mockBackendReadWriter: &mockReadUpdater{
				ReadFunc: func(_ context.Context, _, _ string) (*tinkerbell.Workflow, error) {
					return nil, nil
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
				ReadAllWorkflowRuleSetsFunc: func(_ context.Context) ([]tinkerbell.WorkflowRuleSet, error) {
					return []tinkerbell.WorkflowRuleSet{
						{
							Spec: tinkerbell.WorkflowRuleSetSpec{
								Rules: []string{`{"chassis": {"serial": ["67890"]}}`},
							},
						},
					}, nil
				},
			},
			mockBackendReadWriter: &mockReadUpdater{
				ReadFunc: func(_ context.Context, _, _ string) (*tinkerbell.Workflow, error) {
					return nil, nil
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
				ReadAllWorkflowRuleSetsFunc: func(_ context.Context) ([]tinkerbell.WorkflowRuleSet, error) {
					return []tinkerbell.WorkflowRuleSet{
						{
							Spec: tinkerbell.WorkflowRuleSetSpec{
								Rules: []string{`im a bad pattern`},
							},
						},
					}, nil
				},
			},
			mockBackendReadWriter: &mockReadUpdater{
				ReadFunc: func(_ context.Context, _, _ string) (*tinkerbell.Workflow, error) {
					return nil, nil
				},
			},
			expectedErrorCode: codes.NotFound,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &Handler{
				AutoCapabilities:  AutoCapabilities{Enrollment: AutoEnrollment{Enabled: true, ReadCreator: tt.mockCapabilities}},
				BackendReadWriter: tt.mockBackendReadWriter,
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
	ReadAllWorkflowRuleSetsFunc func(ctx context.Context) ([]tinkerbell.WorkflowRuleSet, error)
	CreateFunc                  func(ctx context.Context, wf *tinkerbell.Workflow) error
}

func (m *mockAutoCapabilities) ReadWorkflowRuleSets(ctx context.Context) ([]tinkerbell.WorkflowRuleSet, error) {
	return m.ReadAllWorkflowRuleSetsFunc(ctx)
}

func (m *mockAutoCapabilities) CreateWorkflow(ctx context.Context, wf *tinkerbell.Workflow) error {
	return m.CreateFunc(ctx, wf)
}

type mockReadUpdater struct {
	ReadAllFunc func(ctx context.Context, _ string) ([]tinkerbell.Workflow, error)
	ReadFunc    func(ctx context.Context, workflowID, namespace string) (*tinkerbell.Workflow, error)
	UpdateFunc  func(ctx context.Context, wf *tinkerbell.Workflow) error
}

func (m *mockReadUpdater) ReadAll(ctx context.Context, _ string) ([]tinkerbell.Workflow, error) {
	return m.ReadAllFunc(ctx, "")
}

func (m *mockReadUpdater) Read(ctx context.Context, workflowID, namespace string) (*tinkerbell.Workflow, error) {
	return m.ReadFunc(ctx, workflowID, namespace)
}

func (m *mockReadUpdater) Update(ctx context.Context, wf *tinkerbell.Workflow) error {
	return m.UpdateFunc(ctx, wf)
}
