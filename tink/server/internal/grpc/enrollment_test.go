package grpc

import (
	"context"
	"errors"
	"testing"

	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestEnroll(t *testing.T) {
	tests := []struct {
		name                  string
		workerID              string
		attributes            *proto.WorkerAttributes
		mockCapabilities      *mockAutoCapabilities
		mockBackendReadWriter *mockReadUpdater
		expectedErrorCode     codes.Code
	}{
		{
			name:     "successful enrollment",
			workerID: "worker-123",
			attributes: &proto.WorkerAttributes{
				Chassis: &proto.Chassis{Serial: toPtr("12345")},
			},
			mockCapabilities: &mockAutoCapabilities{
				ReadAllWorkflowRuleSetsFunc: func(ctx context.Context) ([]v1alpha1.WorkflowRuleSet, error) {
					return []v1alpha1.WorkflowRuleSet{
						{
							Spec: v1alpha1.WorkflowRuleSetSpec{
								Rules:                 []string{`{"chassis": {"serial": ["12345"]}}`},
								WorkflowNamespace:     "default",
								Workflow:              v1alpha1.WorkflowSpec{},
								AddAttributesAsLabels: true,
							},
						},
					}, nil
				},
				CreateFunc: func(ctx context.Context, wf *v1alpha1.Workflow) error {
					return nil
				},
			},
			mockBackendReadWriter: &mockReadUpdater{
				ReadFunc: func(ctx context.Context, workflowID, namespace string) (*v1alpha1.Workflow, error) {
					return nil, nil
				},
			},
			expectedErrorCode: codes.Aborted,
		},
		{
			name:     "no matching workflow rule set",
			workerID: "worker-123",
			attributes: &proto.WorkerAttributes{
				Chassis: &proto.Chassis{Serial: toPtr("12345")},
			},
			mockCapabilities: &mockAutoCapabilities{
				ReadAllWorkflowRuleSetsFunc: func(ctx context.Context) ([]v1alpha1.WorkflowRuleSet, error) {
					return nil, nil
				},
			},
			mockBackendReadWriter: &mockReadUpdater{
				ReadFunc: func(ctx context.Context, workflowID, namespace string) (*v1alpha1.Workflow, error) {
					return nil, nil
				},
			},
			expectedErrorCode: codes.NotFound,
		},
		{
			name:     "error reading workflow rule sets",
			workerID: "worker-123",
			attributes: &proto.WorkerAttributes{
				Chassis: &proto.Chassis{Serial: toPtr("12345")},
			},
			mockCapabilities: &mockAutoCapabilities{
				ReadAllWorkflowRuleSetsFunc: func(ctx context.Context) ([]v1alpha1.WorkflowRuleSet, error) {
					return nil, errors.New("failed to read workflow rule sets")
				},
			},
			mockBackendReadWriter: &mockReadUpdater{
				ReadFunc: func(ctx context.Context, workflowID, namespace string) (*v1alpha1.Workflow, error) {
					return nil, nil
				},
			},
			expectedErrorCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				AutoCapabilities:  AutoCapabilities{Enrollment: AutoEnrollment{Enabled: true, ReadCreator: tt.mockCapabilities}},
				BackendReadWriter: tt.mockBackendReadWriter,
			}

			_, err := handler.enroll(context.Background(), tt.workerID, tt.attributes)
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
	ReadAllWorkflowRuleSetsFunc func(ctx context.Context) ([]v1alpha1.WorkflowRuleSet, error)
	CreateFunc                  func(ctx context.Context, wf *v1alpha1.Workflow) error
}

func (m *mockAutoCapabilities) ReadAllWorkflowRuleSets(ctx context.Context) ([]v1alpha1.WorkflowRuleSet, error) {
	return m.ReadAllWorkflowRuleSetsFunc(ctx)
}

func (m *mockAutoCapabilities) Create(ctx context.Context, wf *v1alpha1.Workflow) error {
	return m.CreateFunc(ctx, wf)
}

type mockReadUpdater struct {
	ReadAllFunc func(ctx context.Context, workerID string) ([]v1alpha1.Workflow, error)
	ReadFunc    func(ctx context.Context, workflowID, namespace string) (*v1alpha1.Workflow, error)
	UpdateFunc  func(ctx context.Context, wf *v1alpha1.Workflow) error
}

func (m *mockReadUpdater) ReadAll(ctx context.Context, workerID string) ([]v1alpha1.Workflow, error) {
	return m.ReadAllFunc(ctx, workerID)
}

func (m *mockReadUpdater) Read(ctx context.Context, workflowID, namespace string) (*v1alpha1.Workflow, error) {
	return m.ReadFunc(ctx, workflowID, namespace)
}

func (m *mockReadUpdater) Update(ctx context.Context, wf *v1alpha1.Workflow) error {
	return m.UpdateFunc(ctx, wf)
}
