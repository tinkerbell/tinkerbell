package grpc

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/cenkalti/backoff/v5"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	epb "google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"quamina.net/go/quamina"
)

const (
	workflowPrefix = "enrollment-"
)

// wflowNamespace is a map of workflow names to their namespaces.
type wflowNamespace map[string]string

func (h *Handler) enroll(ctx context.Context, agentID string, attr *proto.AgentAttributes, allWflows wflowNamespace) (*proto.ActionResponse, error) {
	log := h.Logger.WithValues("agentID", agentID)
	name, err := makeValidName(agentID, workflowPrefix)
	if err != nil {
		log.Info("debugging", "error making valid", true, "error", err)
		return nil, status.Errorf(codes.Internal, "error making agentID a valid Kubernetes name: %v", err)
	}
	log = log.WithValues("workflowName", name)
	// Get all WorkflowRuleSets and check if there is a match to the AgentID or the Attributes (if Attributes are provided by request)
	// using github.com/timbray/quamina
	// If there is a match, create a Workflow for the AgentID.
	wrs, err := h.AutoCapabilities.Enrollment.ReadCreator.ReadWorkflowRuleSets(ctx)
	if err != nil {
		log.Info("debugging", "error getting workflow rules", true, "error", err)
		return nil, errors.Join(errBackendRead, status.Errorf(codes.Internal, "error getting workflow rules: %v", err))
	}
	type match struct {
		wrs        v1alpha1.WorkflowRuleSet
		numMatches int
	}
	final := &match{}
	for _, wr := range wrs {
		if ns, found := allWflows[name]; found && ns == wr.Spec.WorkflowNamespace {
			// log.Info("debugging", "existingWorkflowFound", true, "error", err, "workflowName", name)
			// Should this continue to the next WorkflowRuleSet?
			st := status.New(codes.FailedPrecondition, "existing workflow found")
			ds, err := st.WithDetails(&epb.PreconditionFailure{
				Violations: []*epb.PreconditionFailure_Violation{{
					Type:        proto.PreconditionFailureViolation_PRECONDITION_FAILURE_VIOLATION_ENROLLMENT_EXISTING_WORKFLOW.String(),
					Subject:     fmt.Sprintf("name:%s", name),
					Description: "existing workflow found",
				}},
			})
			if err != nil {
				log.Info("debugging", "error creating status with details", true, "error", err)
				return nil, st.Err()
			}
			return nil, ds.Err()
		}
		q, err := quamina.New()
		if err != nil {
			log.Info("debugging", "error preparing WorkflowRuleSet parser", true, "error", err)
			return nil, status.Errorf(codes.Internal, "error preparing WorkflowRuleSet parser: %v", err)
		}
		for idx, r := range wr.Spec.Rules {
			if err := q.AddPattern(fmt.Sprintf("pattern-%v", idx), r); err != nil {
				log.Info("debugging", "error with pattern in WorkflowRuleSet", true, "error", err)
				return nil, status.Errorf(codes.Internal, "error with pattern in WorkflowRuleSet: %v", err)
			}
		}

		var jsonEvent []byte
		if attr != nil {
			jsonBytes, err := protojson.Marshal(attr)
			if err != nil {
				log.Info("debugging", "error marshalling attributes to json", true, "error", err)
				return nil, status.Errorf(codes.Internal, "error marshalling attributes to json: %v", err)
			}
			// log.Info("debugging", "jsonEvent", string(jsonBytes))
			jsonEvent = jsonBytes
		}
		matches, err := q.MatchesForEvent(jsonEvent)
		if err != nil {
			log.Info("debugging", "error matching pattern", true, "error", err)
			return nil, status.Errorf(codes.Internal, "error matching pattern: %v", err)
		}
		if len(matches) > final.numMatches {
			final.numMatches = len(matches)
			final.wrs = wr
		}
	}
	if final.numMatches > 0 {
		// Create a Workflow for the AgentID
		awf := &v1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: final.wrs.Spec.WorkflowNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: final.wrs.APIVersion,
						Kind:       final.wrs.Kind,
						Name:       final.wrs.Name,
						UID:        final.wrs.UID,
					},
				},
			},
			Spec: final.wrs.Spec.Workflow,
		}
		if final.wrs.Spec.AddAttributesAsLabels {
			maps.Copy(awf.Labels, awf.Spec.HardwareMap)
		}
		if awf.Spec.HardwareMap == nil {
			awf.Spec.HardwareMap = make(map[string]string)
		}
		awf.Spec.HardwareMap[final.wrs.Spec.AgentTemplateValue] = agentID
		// TODO: if the awf.Spec.HardwareRef is an empty string, then query for a Hardware object with some corresponding value from the attributes.
		// If a Hardware object is found add it to the awf.Spec.HardwareRef.
		if err := h.AutoCapabilities.Enrollment.ReadCreator.CreateWorkflow(ctx, awf); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// if we get here, then we didn't find an existing Workflow above, but CreateWorkflow is reporting that there is.
				// So we treat this as a new Workflow creation and send the same error.
				// failed precondition and backoff permanent error so that the backoff retry loop stops and the Agent is signaled to try again immediately.
				log.Info("debugging", "existingWorkflowFound", true, "error", err)
				return nil, status.Error(codes.FailedPrecondition, "existing workflow found")
			}
			log.Info("debugging", "error creating enrollment workflow", true, "error", err)
			return nil, errors.Join(errBackendWrite, status.Errorf(codes.Internal, "error creating enrollment workflow: %v", err))
		}

		ar := &proto.ActionRequest{
			AgentId:         &agentID,
			AgentAttributes: attr,
		}
		return h.enrollRetry(ctx, ar)
	}
	// If there is no match, return an error.
	log.Info("debugging", "noWorkflowRuleSetMatch", true)
	return nil, status.Errorf(codes.NotFound, "no Workflow Rule Sets found or matched for Agent %s", agentID)
}

func (h *Handler) enrollRetry(ctx context.Context, req *proto.ActionRequest) (*proto.ActionResponse, error) {
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
			backoff.WithMaxTries(3),
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
