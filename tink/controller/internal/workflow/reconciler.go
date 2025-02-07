package workflow

import (
	"context"
	serrors "errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/tink/controller/internal/workflow/journal"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler is a type for managing Workflows.
type Reconciler struct {
	client  ctrlclient.Client
	nowFunc func() time.Time
	backoff *backoff.ExponentialBackOff
}

// TODO(jacobweinstock): add functional arguments to the signature.
// TODO(jacobweinstock): write functional argument for customizing the backoff.
func NewReconciler(client ctrlclient.Client) *Reconciler {
	return &Reconciler{
		client:  client,
		nowFunc: time.Now,
		backoff: backoff.NewExponentialBackOff([]backoff.ExponentialBackOffOpts{
			backoff.WithMaxInterval(5 * time.Second), // this should keep all NextBackOff's under 10 seconds
		}...),
	}
}

func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.
		NewControllerManagedBy(mgr).
		For(&tinkerbell.Workflow{}).
		Complete(r)
}

type state struct {
	client   ctrlclient.Client
	workflow *tinkerbell.Workflow
	backoff  *backoff.ExponentialBackOff
}

// +kubebuilder:rbac:groups=tinkerbell.org,resources=hardware;hardware/status,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=tinkerbell.org,resources=templates;templates/status,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=tinkerbell.org,resources=workflows;workflows/status,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=job;job/status,verbs=get;list;watch;delete;create

// Reconcile handles Workflow objects. This includes Template rendering, optional Hardware allowPXE toggling, and optional Hardware one-time netbooting.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	ctx = journal.New(ctx)
	logger := ctrl.LoggerFrom(ctx)
	defer func() {
		logger.V(1).Info("Reconcile code flow journal", "journal", journal.Journal(ctx))
	}()
	logger.Info("Reconcile")
	journal.Log(ctx, "starting reconcile")

	stored := &tinkerbell.Workflow{}
	if err := r.client.Get(ctx, req.NamespacedName, stored); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if !stored.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}
	if stored.Status.BootOptions.Jobs == nil {
		stored.Status.BootOptions.Jobs = make(map[string]tinkerbell.JobStatus)
	}

	wflow := stored.DeepCopy()

	switch wflow.Status.State {
	case "":
		journal.Log(ctx, "new workflow")
		resp, err := r.processNewWorkflow(ctx, logger, wflow)

		return resp, serrors.Join(err, mergePatchStatus(ctx, r.client, stored, wflow))
	case tinkerbell.WorkflowStatePreparing:
		journal.Log(ctx, "preparing workflow")
		s := &state{
			client:   r.client,
			workflow: wflow,
			backoff:  r.backoff,
		}
		resp, err := s.prepareWorkflow(ctx)

		return resp, serrors.Join(err, mergePatchStatus(ctx, r.client, stored, s.workflow))
	case tinkerbell.WorkflowStateRunning:
		journal.Log(ctx, "process running workflow")
		r.processRunningWorkflow(wflow)

		return reconcile.Result{}, mergePatchStatus(ctx, r.client, stored, wflow)
	case tinkerbell.WorkflowStatePost:
		journal.Log(ctx, "post actions")
		s := &state{
			client:   r.client,
			workflow: wflow,
			backoff:  r.backoff,
		}
		rc, err := s.postActions(ctx)

		return rc, serrors.Join(err, mergePatchStatus(ctx, r.client, stored, wflow))
	case tinkerbell.WorkflowStatePending, tinkerbell.WorkflowStateTimeout, tinkerbell.WorkflowStateFailed, tinkerbell.WorkflowStateSuccess:
		journal.Log(ctx, "controller will not trigger another reconcile", "state", wflow.Status.State)
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

// mergePatchStatus merges an updated Workflow with an original Workflow and patches the Status object via the client (cc).
func mergePatchStatus(ctx context.Context, cc ctrlclient.Client, original, updated *tinkerbell.Workflow) error {
	// Patch any changes, regardless of errors
	if !equality.Semantic.DeepEqual(updated.Status, original.Status) {
		journal.Log(ctx, "patching status")
		if err := cc.Status().Patch(ctx, updated, ctrlclient.MergeFrom(original)); err != nil {
			return fmt.Errorf("error patching status of workflow: %s, error: %w", updated.Name, err)
		}
	}
	return nil
}

func (r *Reconciler) processNewWorkflow(ctx context.Context, logger logr.Logger, stored *tinkerbell.Workflow) (reconcile.Result, error) {
	tpl := &tinkerbell.Template{}
	if err := r.client.Get(ctx, ctrlclient.ObjectKey{Name: stored.Spec.TemplateRef, Namespace: stored.Namespace}, tpl); err != nil {
		if errors.IsNotFound(err) {
			// Throw an error to raise awareness and take advantage of immediate requeue.
			logger.Error(err, "error getting Template object in processNewWorkflow function")
			journal.Log(ctx, "template not found")
			stored.Status.TemplateRendering = tinkerbell.TemplateRenderingFailed
			stored.Status.SetCondition(tinkerbell.WorkflowCondition{
				Type:    tinkerbell.TemplateRenderedSuccess,
				Status:  metav1.ConditionFalse,
				Reason:  "Error",
				Message: "template not found",
				Time:    &metav1.Time{Time: metav1.Now().UTC()},
			})
			return reconcile.Result{}, fmt.Errorf(
				"no template found: name=%v; namespace=%v",
				stored.Spec.TemplateRef,
				stored.Namespace,
			)
		}
		stored.Status.TemplateRendering = tinkerbell.TemplateRenderingFailed
		stored.Status.SetCondition(tinkerbell.WorkflowCondition{
			Type:    tinkerbell.TemplateRenderedSuccess,
			Status:  metav1.ConditionFalse,
			Reason:  "Error",
			Message: err.Error(),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})
		return reconcile.Result{}, err
	}

	var hardware tinkerbell.Hardware
	err := r.client.Get(ctx, ctrlclient.ObjectKey{Name: stored.Spec.HardwareRef, Namespace: stored.Namespace}, &hardware)
	if ctrlclient.IgnoreNotFound(err) != nil {
		logger.Error(err, "error getting Hardware object in processNewWorkflow function")
		journal.Log(ctx, "hardware not found")
		stored.Status.TemplateRendering = tinkerbell.TemplateRenderingFailed
		stored.Status.SetCondition(tinkerbell.WorkflowCondition{
			Type:    tinkerbell.TemplateRenderedSuccess,
			Status:  metav1.ConditionFalse,
			Reason:  "Error",
			Message: fmt.Sprintf("error getting hardware: %v", err),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})
		return reconcile.Result{}, err
	}

	if stored.Spec.HardwareRef != "" && errors.IsNotFound(err) {
		logger.Error(err, "hardware not found in processNewWorkflow function")
		journal.Log(ctx, "hardware not found")
		stored.Status.TemplateRendering = tinkerbell.TemplateRenderingFailed
		stored.Status.SetCondition(tinkerbell.WorkflowCondition{
			Type:    tinkerbell.TemplateRenderedSuccess,
			Status:  metav1.ConditionFalse,
			Reason:  "Error",
			Message: fmt.Sprintf("hardware not found: %v", err),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})
		return reconcile.Result{}, fmt.Errorf(
			"hardware not found: name=%v; namespace=%v",
			stored.Spec.HardwareRef,
			stored.Namespace,
		)
	}

	data := make(map[string]interface{})
	for key, val := range stored.Spec.HardwareMap {
		data[key] = val
	}
	contract := toTemplateHardwareData(hardware)
	data["Hardware"] = contract

	tinkWf, err := renderTemplateHardware(stored.Name, pointerToValue(tpl.Spec.Data), data)
	if err != nil {
		stored.Status.TemplateRendering = tinkerbell.TemplateRenderingFailed
		stored.Status.SetCondition(tinkerbell.WorkflowCondition{
			Type:    tinkerbell.TemplateRenderedSuccess,
			Status:  metav1.ConditionFalse,
			Reason:  "Error",
			Message: fmt.Sprintf("error rendering template: %v", err),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})
		return reconcile.Result{}, err
	}

	// populate Task and Action data
	stored.Status = *YAMLToStatus(tinkWf)
	stored.Status.TemplateRendering = tinkerbell.TemplateRenderingSuccessful
	stored.Status.SetCondition(tinkerbell.WorkflowCondition{
		Type:    tinkerbell.TemplateRenderedSuccess,
		Status:  metav1.ConditionTrue,
		Reason:  "Complete",
		Message: "template rendered successfully",
		Time:    &metav1.Time{Time: metav1.Now().UTC()},
	})

	// set hardware allowPXE if requested.
	if stored.Spec.BootOptions.ToggleAllowNetboot || stored.Spec.BootOptions.BootMode != "" {
		stored.Status.State = tinkerbell.WorkflowStatePreparing
		return reconcile.Result{Requeue: true}, nil
	}

	stored.Status.State = tinkerbell.WorkflowStatePending
	return reconcile.Result{}, nil
}

// templateHardwareData defines the data exposed for a Hardware instance to a Template.
type templateHardwareData struct {
	Disks      []string
	Interfaces []tinkerbell.Interface
	UserData   string
	Metadata   tinkerbell.HardwareMetadata
	VendorData string
}

// toTemplateHardwareData converts a Hardware instance of templateHardwareData for use in template
// rendering.
func toTemplateHardwareData(hardware tinkerbell.Hardware) templateHardwareData {
	var contract templateHardwareData
	for _, disk := range hardware.Spec.Disks {
		contract.Disks = append(contract.Disks, disk.Device)
	}
	if len(hardware.Spec.Interfaces) > 0 {
		contract.Interfaces = hardware.Spec.Interfaces
	}
	if hardware.Spec.UserData != nil {
		contract.UserData = pointerToValue(hardware.Spec.UserData)
	}
	if hardware.Spec.Metadata != nil {
		contract.Metadata = *hardware.Spec.Metadata
	}
	if hardware.Spec.VendorData != nil {
		contract.VendorData = pointerToValue(hardware.Spec.VendorData)
	}
	return contract
}

func (r *Reconciler) processRunningWorkflow(stored *tinkerbell.Workflow) {
	// Check for global timeout expiration
	if r.nowFunc().After(stored.GetStartTime().Add(time.Duration(stored.Status.GlobalTimeout) * time.Second)) {
		stored.Status.State = tinkerbell.WorkflowStateTimeout
	}

	// check for any running actions that may have timed out
	for ti, task := range stored.Status.Tasks {
		for ai, action := range task.Actions {
			// A running workflow task action has timed out
			if action.Status == tinkerbell.WorkflowStateRunning && action.StartedAt != nil &&
				r.nowFunc().After(action.StartedAt.Add(time.Duration(action.Timeout)*time.Second)) {
				// Set fields on the timed out action
				stored.Status.Tasks[ti].Actions[ai].Status = tinkerbell.WorkflowStateTimeout
				stored.Status.Tasks[ti].Actions[ai].Message = "Action timed out"
				stored.Status.Tasks[ti].Actions[ai].Seconds = int64(r.nowFunc().Sub(action.StartedAt.Time).Seconds())
				// Mark the workflow as timed out
				stored.Status.State = tinkerbell.WorkflowStateTimeout
			}
			// Update the current action in the status
			if action.Status == tinkerbell.WorkflowStateRunning && stored.Status.CurrentAction != action.Name {
				stored.Status.CurrentAction = action.Name
			}
		}
	}
}

func pointerToValue[V any](ptr *V) V {
	if ptr == nil {
		var zero V
		return zero
	}
	return *ptr
}
