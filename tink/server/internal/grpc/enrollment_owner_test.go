package grpc

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestEnrollOwnerReference checks the owner reference of an auto-enrollment Workflow against a
// WorkflowRuleSet without TypeMeta, which is how a typed client returns it unless it comes from a
// cache. The owner reference must still name the WorkflowRuleSet's kind and group version.
func TestEnrollOwnerReference(t *testing.T) {
	var created *tinkerbell.Workflow
	caps := &mockAutoCapabilities{
		ListWorkflowRuleSetsFunc: func(context.Context, data.WorkflowFilter) ([]tinkerbell.WorkflowRuleSet, error) {
			return []tinkerbell.WorkflowRuleSet{{
				ObjectMeta: metav1.ObjectMeta{Name: "rs", Namespace: "default", UID: "rs-uid"},
				Spec:       tinkerbell.WorkflowRuleSetSpec{Rules: []string{`{"chassis": {"serial": ["12345"]}}`}},
			}}, nil
		},
		CreateWorkflowFunc: func(_ context.Context, wf *tinkerbell.Workflow) error {
			created = wf
			return nil
		},
	}
	handler := &Handler{
		AutoCapabilities: AutoCapabilities{
			Enrollment: AutoEnrollment{Enabled: true, WorkflowRuleSetLister: caps, WorkflowCreator: caps},
		},
		Backend: &mockBackendReadWriter{},
	}

	// The error is expected: the new Workflow has no Action until the Tink Controller reconciles it.
	_ = handler.enroll(context.Background(), "worker-123", convert(&proto.AgentAttributes{Chassis: &proto.Chassis{Serial: toPtr("12345")}}), nil)

	if created == nil {
		t.Fatal("no Workflow created")
	}
	want := []metav1.OwnerReference{{
		APIVersion: tinkerbell.GroupVersion.String(),
		Kind:       "WorkflowRuleSet",
		Name:       "rs",
		UID:        "rs-uid",
	}}
	if diff := cmp.Diff(want, created.OwnerReferences); diff != "" {
		t.Errorf("unexpected owner references (-want +got):\n%s", diff)
	}
}
