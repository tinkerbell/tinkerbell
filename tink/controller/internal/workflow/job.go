package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type jobName string

const (
	jobNameNetboot  jobName = "netboot"
	jobNameISOMount jobName = "iso-mount"
	jobNameISOEject jobName = "iso-eject"
)

func (j jobName) String() string {
	return string(j)
}

// this function will update the Workflow status.
func (s *state) handleJob(ctx context.Context, actions []bmc.Action, name jobName) (reconcile.Result, error) {
	// there are 3 phases. 1. Clean up existing 2. Create new 3. Track status
	// 1. clean up existing job if it wasn't already deleted
	if j := s.workflow.Status.BootOptions.Jobs[name.String()]; !j.ExistingJobDeleted {
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
				Reason:  "Error",
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
				Reason:  "Error",
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

	return reconcile.Result{Requeue: true}, nil
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

	return reconcile.Result{Requeue: true}, nil
}

// This function will update the Workflow status.
func (s *state) createJob(ctx context.Context, actions []bmc.Action, name jobName) (reconcile.Result, error) {
	// create a new job
	// The assumption is that the UID is not set. UID checking is not handled here.
	// 1. look up if there's an existing job with the same name, if so update the status with the UID and return
	// 2. if there's no existing job, create a new job, update the status with the UID, and return

	rj := &bmc.Job{}
	if err := s.client.Get(ctx, client.ObjectKey{Name: name.String(), Namespace: s.workflow.Namespace}, rj); err == nil {
		journal.Log(ctx, "job already exists", "name", name)
		if !rj.DeletionTimestamp.IsZero() {
			journal.Log(ctx, "job is being deleted", "name", name)
			return reconcile.Result{Requeue: true}, nil
		}
		// TODO(jacobweinstock): job exists means that the job name and uid from the status are the same.
		// get the UID and update the status
		jStatus := s.workflow.Status.BootOptions.Jobs[name.String()]
		jStatus.UID = rj.GetUID()
		s.workflow.Status.BootOptions.Jobs[name.String()] = jStatus

		return reconcile.Result{Requeue: true}, nil
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
		return reconcile.Result{}, fmt.Errorf("error creating job: %w", err)
	}
	journal.Log(ctx, "job created", "name", name)

	return reconcile.Result{Requeue: true}, nil
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
	return reconcile.Result{Requeue: true}, trackedStateRunning, nil
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
