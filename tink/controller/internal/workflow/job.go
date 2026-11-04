package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type jobName string

const (
	jobNameNetboot             jobName = "netboot"
	jobNameISOMount            jobName = "iso-mount"
	jobNameISOEject            jobName = "iso-eject"
	jobNameCustombootPreparing jobName = "customboot-preparing"
	jobNameCustombootPost      jobName = "customboot-post"
)

func (j jobName) String() string {
	return string(j)
}

// this function will update the Workflow status.
func (s *state) handleJob(ctx context.Context, actions []bmc.Action, name jobName) (reconcile.Result, error) {
	// No actions and no job recorded means there is no job to create or track, and so nothing to clean up either.
	if len(actions) == 0 && s.workflow.Status.BootOptions.Jobs[name.String()].UID == "" {
		journal.Log(ctx, "no actions provided", "name", name)
		return reconcile.Result{}, nil
	}

	// there are 3 phases. 1. Clean up existing 2. Create new 3. Track status
	// 1. clean up existing job if it wasn't already deleted.
	// This must run before the first create, so that only a leftover Job from an earlier Workflow
	// with the same name is deleted, never the Job this Workflow created (Rufio may already be running it).
	if j := s.workflow.Status.BootOptions.Jobs[name.String()]; !j.ExistingJobDeleted {
		existing := &bmc.Job{}
		err := s.client.Get(ctx, client.ObjectKey{Name: name.String(), Namespace: s.workflow.Namespace}, existing)
		if client.IgnoreNotFound(err) != nil {
			journal.Log(ctx, "error getting existing job", "name", name, "error", err)
			// Returning the error would fail the Workflow over a passing error: try again instead.
			return reconcile.Result{Requeue: true}, nil //nolint:nilerr,staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
		}
		if err == nil && metav1.IsControlledBy(existing, s.workflow) {
			// This Workflow already created the Job, e.g. with a tink-controller version that created it
			// before cleaning up. There is nothing to clean up, and createJob adopts it.
			journal.Log(ctx, "existing job is owned by this workflow, keeping it", "name", name)
			jStatus := s.workflow.Status.BootOptions.Jobs[name.String()]
			jStatus.ExistingJobDeleted = true
			s.workflow.Status.BootOptions.Jobs[name.String()] = jStatus
			return reconcile.Result{Requeue: true}, nil //nolint:staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
		}
		journal.Log(ctx, "deleting existing job", "name", name)
		result, err := s.deleteExisting(ctx, name)
		if err != nil {
			return result, err
		}

		return result, nil
	}

	// 2. create a new job
	if uid := s.workflow.Status.BootOptions.Jobs[name.String()].UID; uid == "" {
		journal.Log(ctx, "no uid found for job", "name", name)
		result, err := s.createJob(ctx, actions, name)
		if err != nil {
			s.workflow.Status.SetCondition(v1alpha1.WorkflowCondition{
				Type:    v1alpha1.BootJobSetupFailed,
				Status:  metav1.ConditionTrue,
				Reason:  reasonError,
				Message: fmt.Sprintf("error creating job: %v", err),
				Time:    &metav1.Time{Time: metav1.Now().UTC()},
			})
			return result, err
		}
		s.workflow.Status.SetCondition(v1alpha1.WorkflowCondition{
			Type:    v1alpha1.BootJobSetupComplete,
			Status:  metav1.ConditionTrue,
			Reason:  "Created",
			Message: "job created",
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})
		return result, nil
	}

	// 3. track status
	if !s.workflow.Status.BootOptions.Jobs[name.String()].Complete {
		journal.Log(ctx, "tracking job", "name", name)
		// track status
		r, tState, err := s.trackRunningJob(ctx, name)
		if err != nil {
			s.workflow.Status.SetCondition(v1alpha1.WorkflowCondition{
				Type:    v1alpha1.BootJobFailed,
				Status:  metav1.ConditionTrue,
				Reason:  reasonError,
				Message: err.Error(),
				Time:    &metav1.Time{Time: metav1.Now().UTC()},
			})
			return r, err
		}
		if tState == trackedStateComplete {
			s.workflow.Status.SetCondition(v1alpha1.WorkflowCondition{
				Type:    v1alpha1.BootJobComplete,
				Status:  metav1.ConditionTrue,
				Reason:  "Complete",
				Message: "job completed",
				Time:    &metav1.Time{Time: metav1.Now().UTC()},
			})
		}
		return r, nil
	}

	return reconcile.Result{Requeue: true}, nil //nolint:staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
}

func (s *state) deleteExisting(ctx context.Context, name jobName) (reconcile.Result, error) {
	// delete the tasks.bmc.tinkerbell.org objects
	// Generally, deleting tasks is not needed. But because we have an embedded kube-apiserver option
	// this is precautionary. The Kubernetes garbage collector is not part of the
	// kube-apiserver, it is a separate controller (kube-controller-manager) that
	// watches for delete events, and is not embedded in Tinkerbell, at the moment.
	op := []client.DeleteAllOfOption{
		client.GracePeriodSeconds(0),
		client.PropagationPolicy(metav1.DeletePropagationBackground),
		client.MatchingLabels{"owner-name": name.String()},
		client.InNamespace(s.workflow.Namespace),
	}
	if err := s.client.DeleteAllOf(ctx, &bmc.Task{}, op...); client.IgnoreNotFound(err) != nil {
		journal.Log(ctx, "error deleting tasks", "name", name, "error", err)
		return reconcile.Result{}, fmt.Errorf("error deleting tasks.bmc.tinkerbell.org objects: %w", err)
	}

	opts := []client.DeleteOption{
		client.GracePeriodSeconds(0),
		client.PropagationPolicy(metav1.DeletePropagationBackground),
	}
	existingJob := &bmc.Job{ObjectMeta: metav1.ObjectMeta{Name: name.String(), Namespace: s.workflow.Namespace}}
	if err := s.client.Delete(ctx, existingJob, opts...); client.IgnoreNotFound(err) != nil {
		journal.Log(ctx, "error deleting job", "name", name, "error", err)
		return reconcile.Result{}, fmt.Errorf("error deleting job.bmc.tinkerbell.org object: %w", err)
	}

	journal.Log(ctx, "job deleted", "name", name)

	jStatus := s.workflow.Status.BootOptions.Jobs[name.String()]
	jStatus.ExistingJobDeleted = true
	// if we delete an existing job, we need to remove any uid that was set.
	jStatus.UID = ""
	jStatus.Complete = false
	s.workflow.Status.BootOptions.Jobs[name.String()] = jStatus

	return reconcile.Result{Requeue: true}, nil //nolint:staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
}

// This function will update the Workflow status.
func (s *state) createJob(ctx context.Context, actions []bmc.Action, name jobName) (reconcile.Result, error) {
	// create a new job
	// The assumption is that the UID is not set. UID checking is not handled here.
	// 1. look up if there's an existing job with the same name, if so update the status with the UID and return
	// 2. if there's no existing job, create a new job, update the status with the UID, and return

	rj := &bmc.Job{}
	err := s.client.Get(ctx, client.ObjectKey{Name: name.String(), Namespace: s.workflow.Namespace}, rj)
	if client.IgnoreNotFound(err) != nil {
		journal.Log(ctx, "error getting job", "name", name, "error", err)
		// Returning the error would fail the Workflow over a passing error: try again instead.
		return reconcile.Result{Requeue: true}, nil //nolint:nilerr,staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
	}
	if err == nil {
		journal.Log(ctx, "job already exists", "name", name)
		if !rj.DeletionTimestamp.IsZero() {
			journal.Log(ctx, "job is being deleted", "name", name)
			return reconcile.Result{Requeue: true}, nil //nolint:staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
		}
		if !metav1.IsControlledBy(rj, s.workflow) {
			// A Job of the same name from another Workflow: one created after the clean up phase ran,
			// or a deleted one the cache still shows. Only a Job this Workflow controls is adopted.
			journal.Log(ctx, "job is not owned by this workflow, deleting it", "name", name)
			return s.deleteExisting(ctx, name)
		}
		// get the UID and update the status
		jStatus := s.workflow.Status.BootOptions.Jobs[name.String()]
		jStatus.UID = rj.GetUID()
		s.workflow.Status.BootOptions.Jobs[name.String()] = jStatus

		return reconcile.Result{Requeue: true}, nil //nolint:staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
	}

	// create a new job
	hw, err := hardwareFrom(ctx, s.client, s.workflow)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("error getting hardware: %w", err)
	}
	if hw.Spec.BMCRef == nil {
		return reconcile.Result{}, fmt.Errorf("hardware %q does not have a BMC", hw.Name)
	}

	ownerRef := []metav1.OwnerReference{
		{
			APIVersion: s.workflow.APIVersion,
			Kind:       s.workflow.Kind,
			Name:       s.workflow.Name,
			UID:        s.workflow.UID,
			Controller: valueToPointer(true),
		},
	}

	if err := create(ctx, s.client, name.String(), hw, s.workflow.Namespace, actions, ownerRef); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// The cache doesn't show yet the Job a previous pass created. The next pass adopts it,
			// or deletes it if it isn't this Workflow's.
			journal.Log(ctx, "job already exists but is not in the cache yet", "name", name)
			return reconcile.Result{Requeue: true}, nil //nolint:staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
		}
		return reconcile.Result{}, fmt.Errorf("error creating job: %w", err)
	}
	journal.Log(ctx, "job created", "name", name)

	return reconcile.Result{Requeue: true}, nil //nolint:staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
}

type trackedState string

var (
	trackedStateComplete trackedState = "complete"
	trackedStateRunning  trackedState = "running"
	trackedStateError    trackedState = "error"
	trackedStateFailed   trackedState = "failed"
)

// This function will update the Workflow status.
func (s *state) trackRunningJob(ctx context.Context, name jobName) (reconcile.Result, trackedState, error) {
	// track status
	// get the job
	rj := &bmc.Job{}
	if err := s.client.Get(ctx, client.ObjectKey{Name: name.String(), Namespace: s.workflow.Namespace}, rj); err != nil {
		if !apierrors.IsNotFound(err) {
			// A passing error: Rufio goes on running the Job, so try again.
			journal.Log(ctx, "error getting job", "name", name, "error", err)
			return reconcile.Result{Requeue: true}, trackedStateRunning, nil //nolint:staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
		}
		return reconcile.Result{}, trackedStateError, fmt.Errorf("error getting job: %w", err)
	}
	if rj.HasCondition(bmc.JobFailed, bmc.ConditionTrue) {
		journal.Log(ctx, "job failed", "name", name)
		// job failed
		return reconcile.Result{}, trackedStateFailed, fmt.Errorf("job failed")
	}
	if rj.HasCondition(bmc.JobCompleted, bmc.ConditionTrue) {
		journal.Log(ctx, "job completed", "name", name)
		// job completed
		jStatus := s.workflow.Status.BootOptions.Jobs[name.String()]
		jStatus.Complete = true
		s.workflow.Status.BootOptions.Jobs[name.String()] = jStatus

		return reconcile.Result{}, trackedStateComplete, nil
	}
	// still running
	journal.Log(ctx, "job still running", "name", name)
	time.Sleep(s.backoff.NextBackOff())
	return reconcile.Result{Requeue: true}, trackedStateRunning, nil //nolint:staticcheck // SA1019: Requeue keeps rate-limited backoff; RequeueAfter would change behavior.
}

func create(ctx context.Context, cc client.Client, name string, hw *v1alpha1.Hardware, ns string, tasks []bmc.Action, ownerRef []metav1.OwnerReference) error {
	journal.Log(ctx, "creating job", "name", name)
	if err := cc.Create(ctx, &bmc.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				"tink-controller-auto-created": "true",
			},
			Labels: map[string]string{
				"tink-controller-auto-created": "true",
			},
			OwnerReferences: ownerRef,
		},
		Spec: bmc.JobSpec{
			MachineRef: bmc.MachineRef{
				Name:      hw.Spec.BMCRef.Name,
				Namespace: ns,
			},
			Tasks: tasks,
		},
	}); err != nil {
		return fmt.Errorf("error creating job.bmc.tinkerbell.org object for netbooting machine: %w", err)
	}

	return nil
}
