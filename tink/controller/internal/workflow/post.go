package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
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
		jname := ternary((len(s.workflow.Spec.BootOptions.OverridePost) > 0), jobNamePostOverride, jobNameISOEject)
		name := jobName(fmt.Sprintf("%s-%s", jname, s.workflow.GetName()))
		if j := s.workflow.Status.BootOptions.Jobs[name.String()]; !j.ExistingJobDeleted || j.UID == "" || !j.Complete {
			journal.Log(ctx, "boot mode isoboot")
			var actions []bmc.Action
			// by specifying the post override the user takes responsibility for all BMC Actions.
			if len(s.workflow.Spec.BootOptions.OverridePost) > 0 {
				actions = s.workflow.Spec.BootOptions.OverridePost
			} else {
				if s.workflow.Spec.BootOptions.ISOURL == "" {
					s.workflow.Status.State = v1alpha1.WorkflowStateFailed
					return reconcile.Result{}, errors.New("iso url must be a valid url")
				}
				actions = []bmc.Action{
					{
						VirtualMediaAction: &bmc.VirtualMediaAction{
							MediaURL: "", // empty to unmount/eject the media
							Kind:     bmc.VirtualMediaCD,
						},
					},
				}
			}

			r, err := s.handleJob(ctx, actions, name)
			if err != nil {
				s.workflow.Status.State = v1alpha1.WorkflowStateFailed
				return r, err
			}
			if s.workflow.Status.BootOptions.Jobs[name.String()].Complete {
				// Post Action handling must only change the Status.State if the status.State was not a failure state (i.e. not FAILED, TIMEOUT).
				if s.workflow.Status.CurrentState != nil {
					s.workflow.Status.State = s.workflow.Status.CurrentState.State
				}
			}
			return r, nil
		}
	}

	// There are no built-in post Actions for netboot, but if the user has specified a post override then it's handled here.
	if s.workflow.Spec.BootOptions.BootMode == v1alpha1.BootModeNetboot && len(s.workflow.Spec.BootOptions.OverridePost) > 0 {
		name := jobName(fmt.Sprintf("%s-%s", jobNamePostOverride, s.workflow.GetName()))
		if j := s.workflow.Status.BootOptions.Jobs[name.String()]; !j.ExistingJobDeleted || j.UID == "" || !j.Complete {
			journal.Log(ctx, "boot mode netboot")
			r, err := s.handleJob(ctx, s.workflow.Spec.BootOptions.OverridePost, name)
			if err != nil {
				s.workflow.Status.State = v1alpha1.WorkflowStateFailed
				return r, err
			}
			if s.workflow.Status.BootOptions.Jobs[name.String()].Complete {
				// Post Action handling must only change the Status.State if the status.State was not a failure state (i.e. not FAILED, TIMEOUT).
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
