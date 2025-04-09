package workflow

import (
	"context"
	serrors "errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
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
	bo := backoff.NewExponentialBackOff()
	bo.MaxInterval = 5 * time.Second // this should keep all NextBackOff's under 10 seconds
	return &Reconciler{
		client:  client,
		nowFunc: time.Now,
		backoff: bo,
	}
}

func (r *Reconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.
		NewControllerManagedBy(mgr).
		For(&v1alpha1.Workflow{}).
		Complete(r)
}

type state struct {
	client   ctrlclient.Client
	workflow *v1alpha1.Workflow
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

	stored := &v1alpha1.Workflow{}
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
		stored.Status.BootOptions.Jobs = make(map[string]v1alpha1.JobStatus)
	}

	wflow := stored.DeepCopy()

	switch wflow.Status.State {
	case "":
		journal.Log(ctx, "new workflow")
		resp, err := r.processNewWorkflow(ctx, logger, wflow)
		// If the Workflow spec is disabled, just set the AgentID and return.
		// The Agent ID is used as an index in the Tink Server backend, so it needs to be set even when the Workflow is disabled.
		if wflow.Spec.Disabled != nil && *wflow.Spec.Disabled {
			journal.Log(ctx, "workflow disabled")
			wflow2 := stored.DeepCopy()
			wflow2.Status.AgentID = wflow.Status.AgentID
			return reconcile.Result{}, mergePatchStatus(ctx, r.client, stored, wflow2)
		}

		return resp, serrors.Join(err, mergePatchStatus(ctx, r.client, stored, wflow))
	case v1alpha1.WorkflowStatePreparing:
		journal.Log(ctx, "preparing workflow")
		s := &state{
			client:   r.client,
			workflow: wflow,
			backoff:  r.backoff,
		}
		resp, err := s.prepareWorkflow(ctx)

		return resp, serrors.Join(err, mergePatchStatus(ctx, r.client, stored, s.workflow))
	case v1alpha1.WorkflowStateRunning:
		journal.Log(ctx, "process running workflow")
		rr := reconcile.Result{}
		/*
			if st := startTime(wflow); st != nil {
				if r.nowFunc().After(st.Add(time.Duration(wflow.Status.GlobalTimeout) * time.Second)) {
					wflow.Status.State = v1alpha1.WorkflowStateTimeout
				} else {
					// requeue after the global timeout to check for expiration
					// TODO: this should only be done once.
					rr = reconcile.Result{RequeueAfter: time.Until(st.Add(time.Duration(wflow.Status.GlobalTimeout) * time.Second))}
				}
			}
		*/

		// requeue after the global timeout to check for expiration
		return rr, mergePatchStatus(ctx, r.client, stored, wflow)
	case v1alpha1.WorkflowStatePost:
		journal.Log(ctx, "post actions")
		s := &state{
			client:   r.client,
			workflow: wflow,
			backoff:  r.backoff,
		}
		/*
			if st := startTime(wflow); st != nil {
				if r.nowFunc().UTC().After(st.Add(time.Duration(wflow.Status.GlobalTimeout) * time.Second)) {
					wflow.Status.State = v1alpha1.WorkflowStateTimeout
					return reconcile.Result{}, mergePatchStatus(ctx, r.client, stored, wflow)
				}
			}
		*/
		rc, err := s.postActions(ctx)

		return rc, serrors.Join(err, mergePatchStatus(ctx, r.client, stored, wflow))
	case v1alpha1.WorkflowStatePending, v1alpha1.WorkflowStateTimeout, v1alpha1.WorkflowStateFailed, v1alpha1.WorkflowStateSuccess:
		journal.Log(ctx, "controller will not trigger another reconcile", "state", wflow.Status.State)
		return reconcile.Result{}, nil
	}
	logger.Info("debugging", "state", wflow.Status.State, "outsideofswitch", true, "storedState", stored.Status.State)

	return reconcile.Result{}, nil
}

// mergePatchStatus merges an updated Workflow with an original Workflow and patches the Status object via the client (cc).
func mergePatchStatus(ctx context.Context, cc ctrlclient.Client, original, updated *v1alpha1.Workflow) error {
	// Patch any changes, regardless of errors
	if !equality.Semantic.DeepEqual(updated.Status, original.Status) {
		journal.Log(ctx, "patching status")
		if err := cc.Status().Patch(ctx, updated, ctrlclient.MergeFrom(original)); err != nil {
			return fmt.Errorf("error patching status of workflow: %s, error: %w", updated.Name, err)
		}
	}
	return nil
}

func (r *Reconciler) processNewWorkflow(ctx context.Context, logger logr.Logger, stored *v1alpha1.Workflow) (reconcile.Result, error) {
	tpl := &v1alpha1.Template{}
	if err := r.client.Get(ctx, ctrlclient.ObjectKey{Name: stored.Spec.TemplateRef, Namespace: stored.Namespace}, tpl); err != nil {
		if errors.IsNotFound(err) {
			// Throw an error to raise awareness and take advantage of immediate requeue.
			logger.Error(err, "error getting Template object in processNewWorkflow function")
			journal.Log(ctx, "template not found")
			stored.Status.TemplateRendering = v1alpha1.TemplateRenderingFailed
			stored.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
				Type:    v1alpha1.TemplateRenderedSuccess,
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
		stored.Status.TemplateRendering = v1alpha1.TemplateRenderingFailed
		stored.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
			Type:    v1alpha1.TemplateRenderedSuccess,
			Status:  metav1.ConditionFalse,
			Reason:  "Error",
			Message: err.Error(),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})
		return reconcile.Result{}, err
	}

	var hardware v1alpha1.Hardware
	err := r.client.Get(ctx, ctrlclient.ObjectKey{Name: stored.Spec.HardwareRef, Namespace: stored.Namespace}, &hardware)
	if ctrlclient.IgnoreNotFound(err) != nil {
		logger.Error(err, "error getting Hardware object in processNewWorkflow function")
		journal.Log(ctx, "hardware not found")
		stored.Status.TemplateRendering = v1alpha1.TemplateRenderingFailed
		stored.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
			Type:    v1alpha1.TemplateRenderedSuccess,
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
		stored.Status.TemplateRendering = v1alpha1.TemplateRenderingFailed
		stored.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
			Type:    v1alpha1.TemplateRenderedSuccess,
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
		journal.Log(ctx, "error rendering template")
		stored.Status.TemplateRendering = v1alpha1.TemplateRenderingFailed
		stored.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
			Type:    v1alpha1.TemplateRenderedSuccess,
			Status:  metav1.ConditionFalse,
			Reason:  "Error",
			Message: fmt.Sprintf("error rendering template: %v", err),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})

		return reconcile.Result{}, err
	}

	// populate Task and Action data
	stored.Status = *YAMLToStatus(tinkWf)
	stored.Status.TemplateRendering = v1alpha1.TemplateRenderingSuccessful
	stored.Status.SetCondition(v1alpha1.WorkflowCondition{
		Type:    v1alpha1.TemplateRenderedSuccess,
		Status:  metav1.ConditionTrue,
		Reason:  "Complete",
		Message: "template rendered successfully",
		Time:    &metav1.Time{Time: metav1.Now().UTC()},
	})

	// set hardware allowPXE if requested.
	if stored.Spec.BootOptions.ToggleAllowNetboot || stored.Spec.BootOptions.BootMode != "" {
		stored.Status.State = v1alpha1.WorkflowStatePreparing
		return reconcile.Result{Requeue: true}, nil
	}

	stored.Status.State = v1alpha1.WorkflowStatePending

	return reconcile.Result{}, nil
}

// templateHardwareData defines the data exposed for a Hardware instance to a Template.
type templateHardwareData struct {
	Disks      []string
	Interfaces []v1alpha1.Interface
	UserData   string
	Metadata   v1alpha1.HardwareMetadata
	VendorData string
}

// toTemplateHardwareData converts a Hardware instance of templateHardwareData for use in template
// rendering.
func toTemplateHardwareData(hardware v1alpha1.Hardware) templateHardwareData {
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

func pointerToValue[V any](ptr *V) V {
	if ptr == nil {
		var zero V
		return zero
	}
	return *ptr
}

// startTime returns the start time, for the first action of the first task.
func startTime(w *v1alpha1.Workflow) *metav1.Time {
	if len(w.Status.Tasks) > 0 {
		if len(w.Status.Tasks[0].Actions) > 0 {
			return w.Status.Tasks[0].Actions[0].ExecutionStart
		}
	}
	return nil
}
