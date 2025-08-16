package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	"k8s.io/apimachinery/pkg/api/equality"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// templateDataReferences is the key used to access the Hardware references in the template data.
	// This is lowercase as it is new and follows the all lowercase convention used when referencing
	// fields in the reference object.
	templateDataReferences = "references"
	// templateDataHardware is the key used to access the Hardware data in the template data.
	templateDataHardware = "hardware"
	// templateDataHardwareLegacy is the key used to access the Hardware data in the template data.
	// This is Title cased as it was the original convention used in the template data and is
	// used for backwards compatibility.
	//
	// Deprecated: use templateDataHardware instead. This key will be removed in a future release.
	templateDataHardwareLegacy = "Hardware"
)

type dynamicClient interface {
	DynamicRead(ctx context.Context, gvr schema.GroupVersionResource, name, namespace string) (map[string]interface{}, error)
}

// Reconciler is a type for managing Workflows.
type Reconciler struct {
	client         ctrlclient.Client
	nowFunc        func() time.Time
	backoff        *backoff.ExponentialBackOff
	dynamicClient  dynamicClient
	referenceRules ReferenceRules
}

type ReferenceRules struct {
	Allowlist []string
	Denylist  []string
}

type Option func(*Reconciler)

// WithReferenceRules sets the reference rules for the Reconciler.
func WithAllowReferenceRules(allowlist []string) Option {
	return func(r *Reconciler) {
		r.referenceRules.Allowlist = allowlist
	}
}

// WithDenyReferenceRules sets the reference rules for the Reconciler.
func WithDenyReferenceRules(denylist []string) Option {
	return func(r *Reconciler) {
		r.referenceRules.Denylist = denylist
	}
}

// TODO(jacobweinstock): add functional arguments to the signature.
// TODO(jacobweinstock): write functional argument for customizing the backoff.
func NewReconciler(client ctrlclient.Client, dc dynamicClient, opts ...Option) *Reconciler {
	bo := backoff.NewExponentialBackOff()
	bo.MaxInterval = 5 * time.Second // this should keep all NextBackOff's under 10 seconds
	d := &Reconciler{
		client:        client,
		nowFunc:       time.Now,
		backoff:       bo,
		dynamicClient: dc,
		referenceRules: ReferenceRules{
			Allowlist: []string{},
			Denylist:  []string{`{"reference": {"name": [{"wildcard": "*"}]}}`}, // deny all by default.
		},
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
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
		if kerrors.IsNotFound(err) {
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

		return resp, errors.Join(err, mergePatchStatus(ctx, r.client, stored, wflow))
	case v1alpha1.WorkflowStatePreparing:
		journal.Log(ctx, "preparing workflow")
		s := &state{
			client:   r.client,
			workflow: wflow,
			backoff:  r.backoff,
		}
		resp, err := s.prepareWorkflow(ctx)

		return resp, errors.Join(err, mergePatchStatus(ctx, r.client, stored, s.workflow))
	case v1alpha1.WorkflowStateRunning:
		journal.Log(ctx, "process running workflow")

		// Check if the global timeout has been reached.
		if wflow.Status.GlobalExecutionStop != nil && r.nowFunc().After(wflow.Status.GlobalExecutionStop.Time) {
			journal.Log(ctx, "global timeout reached")
			wflow.Status.State = v1alpha1.WorkflowStateTimeout
			return reconcile.Result{}, mergePatchStatus(ctx, r.client, stored, wflow)
		}

		// Update AgentID if transitioning between tasks
		if updateAgentIDIfNeeded(wflow) {
			journal.Log(ctx, "updated workflow AgentID for task transition", "newAgentID", wflow.Status.AgentID)
		}

		first := firstAction(wflow)
		if wflow.Status.GlobalExecutionStop == nil && first != nil && wflow.Status.CurrentState != nil && first.ID == wflow.Status.CurrentState.ActionID {
			if first.ExecutionStart == nil {
				return reconcile.Result{}, nil
			}
			now := r.nowFunc()
			var skew time.Duration
			if now.After(first.ExecutionStart.Time) {
				skew = now.Sub(first.ExecutionStart.Time).Abs()
			}
			wflow.Status.GlobalExecutionStop = &metav1.Time{
				Time: now.Add(time.Duration(wflow.Status.GlobalTimeout) * time.Second).Add(skew),
			}
			journal.Log(ctx, "global execution times set")
			return reconcile.Result{RequeueAfter: time.Until(wflow.Status.GlobalExecutionStop.Time)}, mergePatchStatus(ctx, r.client, stored, wflow)
		}

		return reconcile.Result{}, mergePatchStatus(ctx, r.client, stored, wflow)
	case v1alpha1.WorkflowStatePost:
		journal.Log(ctx, "post actions")
		s := &state{
			client:   r.client,
			workflow: wflow,
			backoff:  r.backoff,
		}
		rc, err := s.postActions(ctx)

		return rc, errors.Join(err, mergePatchStatus(ctx, r.client, stored, wflow))
	case v1alpha1.WorkflowStatePending, v1alpha1.WorkflowStateTimeout, v1alpha1.WorkflowStateFailed, v1alpha1.WorkflowStateSuccess:
		journal.Log(ctx, "controller will not trigger another reconcile", "state", wflow.Status.State)

		return reconcile.Result{}, nil
	case v1alpha1.WorkflowState("STATE_PENDING"):
		journal.Log(ctx, "workflow using a deprecated pending state, reprocessing", "state", wflow.Status.State)

		return reconcile.Result{}, errors.Join(r.processWorkflow(ctx, logger, wflow), mergePatchStatus(ctx, r.client, stored, wflow))
	default:
		journal.Log(ctx, "controller will not trigger reconcile, unknown state", "state", wflow.Status.State)
	}

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

func (r *Reconciler) processWorkflow(ctx context.Context, logger logr.Logger, stored *v1alpha1.Workflow) error {
	tpl := &v1alpha1.Template{}
	if err := r.client.Get(ctx, ctrlclient.ObjectKey{Name: stored.Spec.TemplateRef, Namespace: stored.Namespace}, tpl); err != nil {
		if kerrors.IsNotFound(err) {
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

			return fmt.Errorf(
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
		return err
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
		return err
	}

	if stored.Spec.HardwareRef != "" && kerrors.IsNotFound(err) {
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
		return fmt.Errorf(
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
	data[templateDataHardware] = func() interface{} {
		// structToMap is used so that fields are accessible in Templates by their json struct tag names instead of
		// their Go struct field names and their case.
		// for example, {{ hardware.spec.metadata.instance.id }} instead of {{ hardware.Spec.Metadata.Instance.ID }}.
		v, err := structToMap(hardware)
		if err != nil {
			logger.V(1).Info("error converting hardware to map for use in template data", "error", err)
			return map[string]interface{}{}
		}
		return v
	}()
	data[templateDataHardwareLegacy] = contract
	references := make(map[string]interface{})
	var refErr error
	for refName, rf := range hardware.Spec.References {
		ed := evaluationData{
			Source: source{
				Name:      hardware.Name,
				Namespace: hardware.Namespace,
			},
			Reference: rf,
		}
		denied, drules, err := evaluate(ctx, r.referenceRules.Denylist, ed)
		if err != nil {
			refErr = errors.Join(refErr, err)
			logger.V(1).Info("error applying denylist rules", "error", err, "denyRules", r.referenceRules.Denylist)
			continue
		}
		allowed, arules, err := evaluate(ctx, r.referenceRules.Allowlist, ed)
		if err != nil {
			refErr = errors.Join(refErr, err)
			logger.V(1).Info("error applying allowlist rules", "error", err, "allowRules", r.referenceRules.Allowlist)
			continue
		}
		if denied && !allowed {
			refErr = errors.Join(refErr, errors.New("reference denied"))
			logger.V(1).Info("reference denied", "referenceName", refName, "denyRules", drules, "allowRules", arules)
			continue
		}
		logger.V(1).Info("reference allowed", "referenceName", refName, "denyRules", drules, "allowRules", arules)
		gvr := schema.GroupVersionResource{Group: rf.Group, Version: rf.Version, Resource: rf.Resource}
		if v, err := r.dynamicClient.DynamicRead(ctx, gvr, rf.Name, rf.Namespace); err == nil || v != nil {
			references[refName] = v
		} else {
			refErr = errors.Join(refErr, err)
			logger.V(1).Info("error getting reference", "referenceName", rf.Name, "namespace", rf.Namespace, "gvr", gvr, "error", err, "refNil", v == nil)
		}
	}
	data[templateDataReferences] = references

	tinkWf, err := renderTemplateHardware(stored.Name, pointerToValue(tpl.Spec.Data), data)
	if err != nil {
		journal.Log(ctx, "error rendering template")
		stored.Status.TemplateRendering = v1alpha1.TemplateRenderingFailed
		stored.Status.SetConditionIfDifferent(v1alpha1.WorkflowCondition{
			Type:    v1alpha1.TemplateRenderedSuccess,
			Status:  metav1.ConditionFalse,
			Reason:  "Error",
			Message: fmt.Sprintf("error rendering template: %v", errors.Join(refErr, err)),
			Time:    &metav1.Time{Time: metav1.Now().UTC()},
		})

		return err
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

	return nil
}

func (r *Reconciler) processNewWorkflow(ctx context.Context, logger logr.Logger, stored *v1alpha1.Workflow) (reconcile.Result, error) {
	if err := r.processWorkflow(ctx, logger, stored); err != nil {
		return reconcile.Result{}, err
	}

	// set hardware allowPXE if requested.
	if stored.Spec.BootOptions.ToggleAllowNetboot || stored.Spec.BootOptions.BootMode != "" {
		stored.Status.State = v1alpha1.WorkflowStatePreparing
		return reconcile.Result{Requeue: true}, nil
	}

	stored.Status.State = v1alpha1.WorkflowStatePending

	return reconcile.Result{}, nil
}

// structToMap converts a struct to a map[string]interface{}.
func structToMap(item interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Marshal the struct to JSON.
	jsonBytes, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON to a map[string]interface{}.
	if err = json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}

	return result, nil
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

// firstAction returns the first Action of the first Task in the Workflow.
func firstAction(w *v1alpha1.Workflow) *v1alpha1.Action {
	if len(w.Status.Tasks) > 0 {
		if len(w.Status.Tasks[0].Actions) > 0 {
			return &w.Status.Tasks[0].Actions[0]
		}
	}
	return nil
}

// updateAgentIDIfNeeded updates the Workflow's status.AgentID when transitioning between tasks.
// It checks if the current task is complete and if we need to move to the next task with a different agent.
func updateAgentIDIfNeeded(wf *v1alpha1.Workflow) bool {
	// Early return if we don't have the necessary state information
	if wf.Status.CurrentState == nil || len(wf.Status.Tasks) == 0 {
		return false
	}

	// Find the current task index
	currentTaskIndex := -1
	for i, task := range wf.Status.Tasks {
		if task.ID == wf.Status.CurrentState.TaskID {
			currentTaskIndex = i
			break
		}
	}

	// If we can't find the current task, nothing to do
	if currentTaskIndex == -1 {
		return false
	}

	// Step 1: Check for invalid index or if we're in the last task
	if currentTaskIndex >= len(wf.Status.Tasks)-1 {
		// Invalid state: currentTaskIndex out of bounds
		// or we're in the last task, no update needed
		return false
	}

	currentTask := wf.Status.Tasks[currentTaskIndex]
	// Defensive check to prevent out-of-bounds access
	if currentTaskIndex+1 >= len(wf.Status.Tasks) {
		return false
	}
	nextTask := wf.Status.Tasks[currentTaskIndex+1]

	// Step 2: Check if the current task is complete
	// A task is complete when all its actions are in SUCCESS state
	for _, action := range currentTask.Actions {
		if action.State != v1alpha1.WorkflowStateSuccess {
			return false // Current task is not complete
		}
	}

	// Step 3: Check if the next task's first action is pending
	if len(nextTask.Actions) == 0 {
		return false // Next task has no actions
	}

	if nextTask.Actions[0].State != v1alpha1.WorkflowStatePending {
		return false // Next task's first action is not pending
	}

	// Step 4: Check if the current AgentID is not equal to the next task's agent ID
	if wf.Status.AgentID == nextTask.AgentID {
		return false // AgentID is already correct
	}

	// All conditions met, update the status.AgentID to the next task's agentID
	wf.Status.AgentID = nextTask.AgentID
	return true // Indicates that an update was made
}
