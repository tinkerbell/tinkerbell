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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error) {
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
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error) {
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
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error) {
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
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error) {
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
				ListWorkflowRuleSetsFunc: func(_ context.Context, _ data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error) {
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
	ListWorkflowRuleSetsFunc func(ctx context.Context, opts data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error)
	CreateWorkflowFunc       func(ctx context.Context, wf *tinkerbell.Workflow) error
}

func (m *mockAutoCapabilities) ListWorkflowRuleSets(ctx context.Context, opts data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error) {
	return m.ListWorkflowRuleSetsFunc(ctx, opts)
}

func (m *mockAutoCapabilities) CreateWorkflow(ctx context.Context, wf *tinkerbell.Workflow) error {
	return m.CreateWorkflowFunc(ctx, wf)
}

func TestEnrollWorkflowNamespace(t *testing.T) {
	tinkHardware := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "hw", Namespace: "tink"},
		Spec:       tinkerbell.HardwareSpec{Auto: tinkerbell.AutoCapabilities{EnrollmentEnabled: true}},
	}
	tests := map[string]struct {
		workflowNamespace string
		hardware          *tinkerbell.Hardware
		// betterElsewhere adds a rule set in namespace "elsewhere" that matches more rules.
		betterElsewhere bool
		want            string
	}{
		"empty uses the rule set's namespace":  {want: "tink"},
		"same namespace":                       {workflowNamespace: "tink", want: "tink"},
		"other namespace skips the rule set":   {workflowNamespace: "other"},
		"hardware in the rule set's namespace": {hardware: tinkHardware, want: "tink"},
		"hardware in another namespace skips the rule set": {
			hardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{Name: "hw", Namespace: "other"},
				Spec:       tinkerbell.HardwareSpec{Auto: tinkerbell.AutoCapabilities{EnrollmentEnabled: true}},
			},
		},
		"without hardware the best match in any namespace wins": {betterElsewhere: true, want: "elsewhere"},
		"hardware's namespace wins over a better match elsewhere": {
			hardware: tinkHardware, betterElsewhere: true, want: "tink",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got string
			caps := &mockAutoCapabilities{
				ListWorkflowRuleSetsFunc: func(context.Context, data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error) {
					rs := []tinkerbell.WorkflowRuleSet{{
						ObjectMeta: metav1.ObjectMeta{Name: "rs", Namespace: "tink"},
						Spec: tinkerbell.WorkflowRuleSetSpec{
							Rules:    []string{`{"chassis": {"serial": ["12345"]}}`},
							Workflow: tinkerbell.WorkflowRuleSetWorkflow{Namespace: tt.workflowNamespace},
						},
					}}
					if tt.betterElsewhere {
						rs = append(rs, tinkerbell.WorkflowRuleSet{
							ObjectMeta: metav1.ObjectMeta{Name: "rs", Namespace: "elsewhere"},
							Spec: tinkerbell.WorkflowRuleSetSpec{Rules: []string{
								`{"chassis": {"serial": ["12345"]}}`,
								`{"chassis": {"serial": [{"prefix": "123"}]}}`,
							}},
						})
					}
					return rs, nil
				},
				CreateWorkflowFunc: func(_ context.Context, wf *tinkerbell.Workflow) error {
					got = wf.Namespace
					return nil
				},
			}
			handler := &Handler{
				AutoCapabilities: AutoCapabilities{Enrollment: AutoEnrollment{Enabled: true, WorkflowRuleSetLister: caps, WorkflowCreator: caps}},
				Backend:          &mockBackendReadWriter{},
				RetryOptions:     []backoff.RetryOption{backoff.WithMaxTries(1)},
			}
			attr := convert(&proto.AgentAttributes{Chassis: &proto.Chassis{Serial: toPtr("12345")}})
			_, _ = handler.enroll(context.Background(), "worker-123", attr, tt.hardware)
			if got != tt.want {
				t.Fatalf("Workflow created in %q, want %q", got, tt.want)
			}
		})
	}
}

type notFoundError struct{}

func (notFoundError) Error() string  { return "not found" }
func (notFoundError) NotFound() bool { return true }

type multipleFoundError struct{}

func (multipleFoundError) Error() string       { return "multiple found" }
func (multipleFoundError) MultipleFound() bool { return true }

type createHardwareFunc func(context.Context, *tinkerbell.Hardware) error

func (f createHardwareFunc) CreateHardware(ctx context.Context, hw *tinkerbell.Hardware) error {
	return f(ctx, hw)
}

func TestGetActionEnrollmentHardwareLookupError(t *testing.T) {
	tests := map[string]struct {
		discovery   bool
		hardwareErr error
		createErr   error
		wantCode    codes.Code
		wantCreated bool
		wantTries   int
	}{
		"not found enrolls without Hardware": {hardwareErr: notFoundError{}, wantCreated: true},
		"multiple found is not retried":      {hardwareErr: multipleFoundError{}, wantCode: codes.FailedPrecondition, wantTries: 1},
		"read error is retried":              {hardwareErr: errors.New("boom"), wantCode: codes.Unavailable, wantTries: 3},
		"discovery multiple found":           {discovery: true, hardwareErr: multipleFoundError{}, wantCode: codes.FailedPrecondition, wantTries: 1},
		"discovery read error":               {discovery: true, hardwareErr: errors.New("boom"), wantCode: codes.Unavailable, wantTries: 3},
		"discovery create not found": {
			discovery: true, hardwareErr: notFoundError{},
			createErr: apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "missing"),
			wantCode:  codes.Unavailable, wantTries: 3,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var created bool
			caps := &mockAutoCapabilities{
				ListWorkflowRuleSetsFunc: func(context.Context, data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error) {
					return []tinkerbell.WorkflowRuleSet{{
						ObjectMeta: metav1.ObjectMeta{Name: "rs", Namespace: "tink"},
						Spec:       tinkerbell.WorkflowRuleSetSpec{Rules: []string{`{"chassis": {"serial": ["12345"]}}`}},
					}}, nil
				},
				CreateWorkflowFunc: func(context.Context, *tinkerbell.Workflow) error {
					created = true
					return nil
				},
			}
			backend := &mockBackendReadWriter{hardwareErr: tt.hardwareErr}
			handler := &Handler{
				AutoCapabilities: AutoCapabilities{
					Enrollment: AutoEnrollment{Enabled: true, WorkflowRuleSetLister: caps, WorkflowCreator: caps},
					Discovery: AutoDiscovery{
						Enabled: tt.discovery, Namespace: "discovery", HardwareFilterer: backend,
						HardwareCreator: createHardwareFunc(func(context.Context, *tinkerbell.Hardware) error { return tt.createErr }),
					},
				},
				Backend:      backend,
				RetryOptions: []backoff.RetryOption{backoff.WithMaxTries(3), backoff.WithBackOff(&backoff.ZeroBackOff{})},
			}
			req := &proto.ActionRequest{
				AgentId:         toPtr("worker-123"),
				AgentAttributes: &proto.AgentAttributes{Chassis: &proto.Chassis{Serial: toPtr("12345")}},
			}
			_, err := handler.GetAction(context.Background(), req)
			if ns := backend.hardwareFilter.InNamespace; ns != "" {
				t.Fatalf("Hardware lookup limited to namespace %q, want all namespaces", ns)
			}
			if created != tt.wantCreated {
				t.Fatalf("Workflow created = %v, want %v", created, tt.wantCreated)
			}
			if tt.wantCreated {
				return
			}
			if got := status.Code(err); got != tt.wantCode {
				t.Fatalf("code = %v, want %v (err: %v)", got, tt.wantCode, err)
			}
			if backend.workflowLists != tt.wantTries {
				t.Fatalf("tries = %d, want %d", backend.workflowLists, tt.wantTries)
			}
		})
	}
}
