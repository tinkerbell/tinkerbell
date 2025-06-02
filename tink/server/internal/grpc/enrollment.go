package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/cenkalti/backoff/v5"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"quamina.net/go/quamina"
)

const (
	workflowPrefix       = "enrollment-"
	attributesAnnotation = "tinkerbell.org/agent-attributes"
)

type match struct {
	wrs        tinkerbell.WorkflowRuleSet
	numMatches int
}

// enroll creates a Workflow for an agentID by matching the attr against WorkflowRuleSets.
// auto enrollment does not support Templates with multiple Agents defined.
func (h *Handler) enroll(ctx context.Context, agentID string, attr *data.AgentAttributes) (*proto.ActionResponse, error) {
	log := h.Logger.WithValues("agentID", agentID)
	// Get all WorkflowRuleSets and check if there is a match to the AgentID or the Attributes (if Attributes are provided by request)
	// using github.com/timbray/quamina
	// If there is a match, create a Workflow for the AgentID.
	wrs, err := h.AutoCapabilities.Enrollment.ReadCreator.ReadWorkflowRuleSets(ctx)
	if err != nil {
		journal.Log(ctx, "error getting workflow rules", "error", err)
		return nil, errors.Join(ErrBackendRead, status.Errorf(codes.Internal, "error getting workflow rules: %v", err))
	}
	name, err := makeValidName(agentID, workflowPrefix)
	if err != nil {
		journal.Log(ctx, "error making agentID a valid Kubernetes name", "error", err)
		return nil, status.Errorf(codes.Internal, "error making agentID a valid Kubernetes name: %v", err)
	}
	log = log.WithValues("workflowName", name)

	final := &match{}
	for _, wr := range wrs {
		m, err := findMatch(wr, attr, final.numMatches)
		if err != nil {
			journal.Log(ctx, "error matching pattern", "error", err)
			log.Error(err, "error matching pattern")
			continue
		}
		if m != nil {
			final.numMatches = m.numMatches
			final.wrs = m.wrs
		}
	}
	if final.numMatches > 0 { //nolint:nestif // TODO: look into this.
		// Create a Workflow for the AgentID
		awf := &tinkerbell.Workflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: final.wrs.Spec.Workflow.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: final.wrs.APIVersion,
						Kind:       final.wrs.Kind,
						Name:       final.wrs.Name,
						UID:        final.wrs.UID,
					},
				},
			},
			Spec: tinkerbell.WorkflowSpec{
				TemplateRef: final.wrs.Spec.Workflow.Template.Ref,
				Disabled:    final.wrs.Spec.Workflow.Disabled,
			},
		}

		if final.wrs.Spec.Workflow.AddAttributes {
			// Add attributes to the Workflow as an annotation.
			if awf.Annotations == nil {
				awf.Annotations = make(map[string]string)
			}
			if a, err := json.Marshal(attr); err == nil {
				awf.Annotations[attributesAnnotation] = string(a)
			} else {
				journal.Log(ctx, "error marshalling attributes to json", "error", err)
			}
		}

		if awf.Spec.HardwareMap == nil {
			awf.Spec.HardwareMap = make(map[string]string)
		}
		awf.Spec.HardwareMap[final.wrs.Spec.Workflow.Template.AgentValue] = agentID
		maps.Copy(awf.Spec.HardwareMap, final.wrs.Spec.Workflow.Template.KVs)
		// TODO: if the awf.Spec.HardwareRef is an empty string, then query for a Hardware object with some corresponding value from the attributes.
		// If a Hardware object is found add it to the awf.Spec.HardwareRef.
		if err := h.AutoCapabilities.Enrollment.ReadCreator.CreateWorkflow(ctx, awf); err != nil {
			if apierrors.IsAlreadyExists(err) {
				journal.Log(ctx, "workflow already exists", "workflow", name, "namespace", final.wrs.Spec.Workflow.Namespace)
				// if we get here, then we didn't find an existing Workflow above, but CreateWorkflow is reporting that there is.
				// So we treat this as a new Workflow creation and send the same error.
				// failed precondition and backoff permanent error so that the backoff retry loop stops and the Agent is signaled to try again immediately.
				return nil, status.Error(codes.FailedPrecondition, "existing workflow found")
			}
			journal.Log(ctx, "error creating enrollment workflow", "error", err)
			return nil, errors.Join(ErrBackendWrite, status.Errorf(codes.Internal, "error creating enrollment workflow: %v", err))
		}

		ar := &proto.ActionRequest{
			AgentId: &agentID,
		}
		journal.Log(ctx, "calling getActionNoAuto")
		// Now that we have created a Workflow, we need to call getActionNoAuto to serve an Action to the Agent.
		return h.getActionNoAuto(ctx, ar)
	}
	// If there is no match, return an error.
	return nil, status.Errorf(codes.NotFound, "no Workflow Rule Sets found or matched for Agent %s", agentID)
}

func findMatch(wr tinkerbell.WorkflowRuleSet, attr *data.AgentAttributes, curMatches int) (*match, error) {
	q, _ := quamina.New() // errors are ignored because they can only happen when passing in options.
	for idx, r := range wr.Spec.Rules {
		if err := q.AddPattern(fmt.Sprintf("pattern-%v", idx), r); err != nil {
			// TODO: pattern checking should be done before this. Maybe when a CRD is created.
			return nil, fmt.Errorf("error adding Workflow matching pattern: %v err: %w", fmt.Sprintf("pattern-%v", idx), err)
		}
	}

	var jsonEvent []byte
	if attr != nil {
		jsonBytes, err := json.Marshal(attr)
		if err != nil {
			return nil, fmt.Errorf("error marshalling attributes to json: %w", err)
		}
		jsonEvent = jsonBytes
	}
	matches, err := q.MatchesForEvent(jsonEvent)
	if err != nil {
		return nil, fmt.Errorf("error matching pattern: %w", err)
	}
	if len(matches) > curMatches {
		return &match{
			numMatches: len(matches),
			wrs:        wr,
		}, nil
	}

	return nil, nil
}

// getActionNoAuto calls to the doGetAction method with a retry mechanism that disables auto capabilities.
func (h *Handler) getActionNoAuto(ctx context.Context, req *proto.ActionRequest) (*proto.ActionResponse, error) {
	operation := func() (*proto.ActionResponse, error) {
		opts := &options{
			AutoCapabilities: AutoCapabilities{
				Enrollment: AutoEnrollment{
					Enabled: false,
				},
				Discovery: AutoDiscovery{
					Enabled: false,
				},
			},
		}
		return h.doGetAction(ctx, req, opts)
	}
	if len(h.RetryOptions) == 0 {
		h.RetryOptions = []backoff.RetryOption{
			// This needs to be sufficiently long to allow for the Tink Controller to reconcile the Workflow and any server side caches to pick up
			// the updated Workflow.
			backoff.WithMaxTries(10),
			backoff.WithBackOff(backoff.NewExponentialBackOff()),
		}
	}
	// We retry multiple times as we read-write to the Workflow Status and there can be caching and eventually consistent issues
	// that would cause the write to fail. A retry to get the latest Workflow resolves these types of issues.
	resp, err := backoff.Retry(ctx, operation, h.RetryOptions...)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
