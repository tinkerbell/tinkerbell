package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/tink/controller/internal/workflow/journal"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (s *state) postActions(ctx context.Context) (reconcile.Result, error) {
	// 1. Handle toggling allowPXE in a hardware object if toggleAllowNetboot is true.
	if s.workflow.Spec.BootOptions.ToggleAllowNetboot && !s.workflow.Status.BootOptions.AllowNetboot.ToggledFalse {
		journal.Log(ctx, "toggling allowPXE false")
		if err := s.toggleHardware(ctx, false); err != nil {
			s.workflow.Status.State = v1alpha1.WorkflowStateFailed
			// TODO: add a status field for why the post Action failed.
			return reconcile.Result{}, err
		}
	}

	// 2. Handle ISO eject scenario.
	//nolint: nestif // This is what it is.
	if s.workflow.Spec.BootOptions.BootMode == v1alpha1.BootModeISO || s.workflow.Spec.BootOptions.BootMode == v1alpha1.BootModeISOBoot {
		name := jobName(fmt.Sprintf("%s-%s", jobNameISOEject, s.workflow.GetName()))
		if j := s.workflow.Status.BootOptions.Jobs[name.String()]; !j.ExistingJobDeleted || j.UID == "" || !j.Complete {
			journal.Log(ctx, "boot mode iso")
			if s.workflow.Spec.BootOptions.ISOURL == "" {
				s.workflow.Status.State = v1alpha1.WorkflowStateFailed
				return reconcile.Result{}, errors.New("iso url must be a valid url")
			}
			actions := []bmc.Action{
				{
					VirtualMediaAction: &bmc.VirtualMediaAction{
						MediaURL: "", // empty to unmount/eject the media
						Kind:     bmc.VirtualMediaCD,
					},
				},
			}

			r, err := s.handleJob(ctx, actions, name)
			if err != nil {
				s.workflow.Status.State = v1alpha1.WorkflowStateFailed
				return r, err
			}
			if s.workflow.Status.BootOptions.Jobs[name.String()].Complete {
				// Post Action handling must only change the Status.State if the status.State was not a failure state (i.e. not STATE_FAILED, STATE_TIMEOUT).
				if s.workflow.Status.CurrentState != nil {
					s.workflow.Status.State = s.workflow.Status.CurrentState.State
				}
			}
			return r, nil
		}
	}

	if s.workflow.Status.CurrentState != nil {
		s.workflow.Status.State = s.workflow.Status.CurrentState.State
	}
	return reconcile.Result{}, nil
}
