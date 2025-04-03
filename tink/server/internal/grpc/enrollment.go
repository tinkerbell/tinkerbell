package grpc

import (
	"context"
	"errors"
	"fmt"

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

func (h *Handler) enroll(ctx context.Context, workerID string, attr *proto.WorkerAttributes) (*proto.ActionResponse, error) {
	log := h.Logger.WithValues("workerID", workerID)
	name, err := makeValidName(workerID, "enrollment-")
	if err != nil {
		log.Info("debugging", "error making valid", true, "error", err)
		return nil, status.Errorf(codes.Internal, "error making workerID a valid Kubernetes name: %v", err)
	}
	log = log.WithValues("workflowName", name)
	// Get all WorkflowRuleSets and check if there is a match to the WorkerID or the Attributes (if Attributes are provided by request)
	// using github.com/timbray/quamina
	// If there is a match, create a Workflow for the WorkerID.
	wrs, err := h.AutoCapabilities.Enrollment.ReadCreator.ReadAllWorkflowRuleSets(ctx)
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
		if w, err := h.BackendReadWriter.Read(ctx, name, wr.Spec.WorkflowNamespace); (err != nil && !apierrors.IsNotFound(err)) || w != nil {
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
		// Create a Workflow for the WorkerID
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
			awf.Labels = flattenAttributes(attr)
		}
		if awf.Spec.HardwareMap == nil {
			awf.Spec.HardwareMap = make(map[string]string)
		}
		awf.Spec.HardwareMap[final.wrs.Spec.WorkerTemplateName] = workerID
		// TODO: if the awf.Spec.HardwareRef is an empty string, then query for a Hardware object with some corresponding value from the attributes.
		// If a Hardware object is found add it to the awf.Spec.HardwareRef.
		if err := h.AutoCapabilities.Enrollment.ReadCreator.Create(ctx, awf); err != nil {
			log.Info("debugging", "error creating enrollment workflow", true, "error", err)
			return nil, errors.Join(errBackendWrite, status.Errorf(codes.Internal, "error creating enrollment workflow: %v", err))
		}
		log.Info("debugging", "enrollmentWorkflowCreated", true)
		st := status.New(codes.Aborted, "enrollment workflow created, please try again")

		ds, err := st.WithDetails(&epb.PreconditionFailure{
			Violations: []*epb.PreconditionFailure_Violation{{
				Type:        proto.PreconditionFailureViolation_PRECONDITION_FAILURE_VIOLATION_ENROLLMENT_WORKFLOW_CREATED.String(),
				Subject:     fmt.Sprintf("name:%s", name),
				Description: "enrollment workflow created, please try again",
			}},
		})
		if err != nil {
			log.Info("debugging", "error creating status with details", true, "error", err)
			return nil, st.Err()
		}

		return nil, backoff.Permanent(ds.Err())
	}
	// If there is no match, return an error.
	log.Info("debugging", "noWorkflowRuleSetMatch", true)
	return nil, status.Errorf(codes.NotFound, "no Workflow Rule Sets found or matched for worker %s", workerID)
}
