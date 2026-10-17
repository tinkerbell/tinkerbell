package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"quamina.net/go/quamina"
)

const (
	workflowPrefix = "enrollment-"
)

type match struct {
	wrs        tinkerbell.WorkflowRuleSet
	numMatches int
}

// enroll creates a Workflow for an agentID by matching the attr against WorkflowRuleSets.
// It never serves an Action: the new Workflow has none until the Tink Controller reconciles it,
// so the returned status tells the Agent to poll again.
// auto enrollment does not support Templates with multiple Agents defined.
func (h *Handler) enroll(ctx context.Context, agentID string, attr *data.AgentAttributes, hardware *tinkerbell.Hardware) error {
	log := h.Logger.WithValues("agentID", agentID)
	// If auto enrollment is not enabled, then we do not create a Workflow for the AgentID.
	if hardware != nil && !hardware.Spec.Auto.EnrollmentEnabled {
		journal.Log(ctx, "auto enrollment is disabled for this hardware", "hardware", hardware.Name)
		return status.Errorf(codes.FailedPrecondition, "auto enrollment is disabled for this hardware")
	}

	// Get all WorkflowRuleSets and check if there is a match to the AgentID or the Attributes (if Attributes are provided by request)
	// using github.com/timbray/quamina
	// If there is a match, create a Workflow for the AgentID.
	wrs, err := h.AutoCapabilities.Enrollment.ListWorkflowRuleSets(ctx, data.WorkflowFilter{})
	if err != nil {
		journal.Log(ctx, "error getting workflow rules", "error", err)
		return errors.Join(ErrBackendRead, status.Errorf(codes.Internal, "error getting workflow rules: %v", err))
	}
	name, err := makeValidName(agentID, workflowPrefix)
	if err != nil {
		journal.Log(ctx, "error making agentID a valid Kubernetes name", "error", err)
		return status.Errorf(codes.Internal, "error making agentID a valid Kubernetes name: %v", err)
	}
	log = log.WithValues("workflowName", name)

	final := &match{}
	for _, wr := range wrs {
		// The Workflow's owner reference to wr is only valid in wr's namespace.
		if ns := wr.Spec.Workflow.Namespace; ns != "" && ns != wr.Namespace { //nolint:staticcheck // read only to reject a mismatch until the field is removed
			journal.Log(ctx, "skipping WorkflowRuleSet whose spec.workflow.namespace is not its own namespace",
				"workflowRuleSet", wr.Name, "namespace", wr.Namespace, "workflowNamespace", ns)
			log.Info("skipping WorkflowRuleSet whose spec.workflow.namespace is not its own namespace",
				"workflowRuleSet", wr.Name, "namespace", wr.Namespace, "workflowNamespace", ns)
			continue
		}
		// A Workflow can only reference Hardware in its own namespace, and it is created in wr's.
		if hardware != nil && wr.Namespace != hardware.Namespace {
			journal.Log(ctx, "skipping WorkflowRuleSet outside the Hardware's namespace",
				"workflowRuleSet", wr.Name, "namespace", wr.Namespace, "hardwareNamespace", hardware.Namespace)
			log.Info("skipping WorkflowRuleSet outside the Hardware's namespace",
				"workflowRuleSet", wr.Name, "namespace", wr.Namespace, "hardwareNamespace", hardware.Namespace)
			continue
		}
		m, err := findMatch(wr, attr, final.numMatches)
		if err != nil {
			journal.Log(ctx, "error matching pattern", "workflowRuleSet", wr.Name, "namespace", wr.Namespace, "error", err)
			log.Error(err, "error matching pattern", "workflowRuleSet", wr.Name, "namespace", wr.Namespace)
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
				Namespace: final.wrs.Namespace,
				Labels: map[string]string{
					"tinkerbell.org/auto-enrollment": "true",
				},
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
		if hardware != nil {
			awf.Spec.HardwareRef = hardware.Name
		}

		if final.wrs.Spec.Workflow.AddAttributes {
			// Add attributes to the Workflow as an annotation.
			if awf.Annotations == nil {
				awf.Annotations = make(map[string]string)
			}
			if a, err := json.Marshal(attr); err == nil {
				awf.Annotations[constant.AttributesAnnotation] = string(a)
			} else {
				journal.Log(ctx, "error marshalling attributes to json", "error", err)
				log.Error(err, "error marshalling attributes to json")
			}
		}

		if awf.Spec.HardwareMap == nil {
			awf.Spec.HardwareMap = make(map[string]string)
		}
		awf.Spec.HardwareMap[final.wrs.Spec.Workflow.Template.AgentValue] = agentID
		maps.Copy(awf.Spec.HardwareMap, final.wrs.Spec.Workflow.Template.KVs)
		// TODO: if the awf.Spec.HardwareRef is an empty string, then query for a Hardware object with some corresponding value from the attributes.
		// If a Hardware object is found add it to the awf.Spec.HardwareRef.
		if err := h.AutoCapabilities.Enrollment.CreateWorkflow(ctx, awf); err != nil {
			if apierrors.IsAlreadyExists(err) {
				journal.Log(ctx, "workflow already exists", "workflow", name, "namespace", final.wrs.Namespace)
				// The Workflow exists but isn't in the cache yet. The Agent polls again and finds it once the cache catches up.
				return status.Error(codes.FailedPrecondition, "existing workflow found")
			}
			journal.Log(ctx, "error creating enrollment workflow", "error", err)
			return errors.Join(ErrBackendWrite, status.Errorf(codes.Internal, "error creating enrollment workflow: %v", err))
		}
		journal.Log(ctx, "created enrollment Workflow", "namespace", awf.Namespace, "workflowRuleSet", final.wrs.Name)
		log.Info("created enrollment Workflow", "namespace", awf.Namespace, "workflowRuleSet", final.wrs.Name)

		// The Tink Controller must reconcile the new Workflow before it has an Action to serve.
		// The Agent picks it up on a later poll.
		return status.Error(codes.NotFound, "enrollment Workflow created, no Action available yet")
	}
	journal.Log(ctx, "no WorkflowRuleSet matched")
	return status.Errorf(codes.NotFound, "no Workflow Rule Sets found or matched for Agent %s", agentID)
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
