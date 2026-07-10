package workflow

import (
	"context"
	"fmt"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// failCustombootAction records a BootJobSetupFailed condition (matching the netboot/isoboot
// convention in pre.go) and returns the error, so a hardwareFrom/templating/webhook failure is
// visible on the Workflow's status.conditions, not just in reconciler logs.
func (s *state) failCustombootAction(msg string, err error) (reconcile.Result, bool, error) {
	s.workflow.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
		Type:    v1alpha1.BootJobSetupFailed,
		Status:  metav1.ConditionFalse,
		Reason:  "Error",
		Message: fmt.Sprintf("%s: %s", msg, err.Error()),
		Time:    &metav1.Time{Time: metav1.Now().UTC()},
	})
	return reconcile.Result{}, false, fmt.Errorf("%s: %w", msg, err)
}

// handleCustombootActions walks an ordered PreparingActions/PostActions list one entry (or one
// contiguous run of BMC-action entries) at a time, resuming from
// Status.BootOptions.Actions[listName]. Webhook entries are called directly; runs of contiguous
// BMC-action entries are batched into a single bmc.Job (via the existing
// handleJob/createJob/trackRunningJob machinery), named "<baseName>-<batchStartIndex>" so a list
// with more than one BMC run gets more than one Job — that name is always recomputed from
// baseName and the current Completed index, never cached. Returns done=true once every entry in
// the list has succeeded (or the list is empty).
func (s *state) handleCustombootActions(ctx context.Context, actions []v1alpha1.CustombootAction, listName string, baseName jobName) (reconcile.Result, bool, error) {
	if len(actions) == 0 {
		return reconcile.Result{}, true, nil
	}
	if s.workflow.Status.BootOptions.Actions == nil {
		s.workflow.Status.BootOptions.Actions = map[string]v1alpha1.ActionListStatus{}
	}
	status := s.workflow.Status.BootOptions.Actions[listName]
	if status.Completed >= len(actions) {
		return reconcile.Result{}, true, nil
	}

	if actions[status.Completed].Webhook != nil {
		hw, err := hardwareFrom(ctx, s.client, s.workflow)
		if err != nil {
			return s.failCustombootAction("failed to get hardware", err)
		}
		rw, err := templateWebhook(ctx, s.client, s.workflow.Namespace, *actions[status.Completed].Webhook, hw)
		if err != nil {
			return s.failCustombootAction("failed to template webhook", err)
		}
		if err := s.callWebhook(ctx, rw); err != nil {
			return s.failCustombootAction(fmt.Sprintf("entry %q (index %d) webhook failed", listName, status.Completed), err)
		}

		status.Completed++
		s.workflow.Status.BootOptions.Actions[listName] = status
		return reconcile.Result{Requeue: true}, status.Completed == len(actions), nil
	}

	// BMC-action entry: batch this entry and every immediately-following BMC-action entry
	// into one bmc.Job, same as the existing whole-phase Job, just scoped to a contiguous run.
	end := status.Completed
	for end < len(actions) && actions[end].Webhook == nil {
		end++
	}
	batch := make([]bmc.Action, 0, end-status.Completed)
	for _, a := range actions[status.Completed:end] {
		batch = append(batch, a.Action) // the embedded bmc.Action
	}

	// Deterministic from baseName + the batch's start index — no need to cache it in status.
	name := fmt.Sprintf("%s-%d", baseName.String(), status.Completed)

	hw, err := hardwareFrom(ctx, s.client, s.workflow)
	if err != nil {
		return s.failCustombootAction("failed to get hardware", err)
	}
	templated, err := templateActions(batch, hw)
	if err != nil {
		return s.failCustombootAction("failed to template actions", err)
	}
	r, err := s.handleJob(ctx, templated, jobName(name))
	if err != nil {
		return r, false, err
	}
	if !s.workflow.Status.BootOptions.Jobs[name].Complete {
		return r, false, nil
	}

	status.Completed = end
	s.workflow.Status.BootOptions.Actions[listName] = status
	return reconcile.Result{Requeue: true}, status.Completed == len(actions), nil
}
