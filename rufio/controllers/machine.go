/*
Copyright 2022 Tinkerbell.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/tinkerbell/rufio/api/v1alpha1"
)

// MachineReconciler reconciles a Machine object.
type MachineReconciler struct {
	client           client.Client
	recorder         record.EventRecorder
	bmcClientFactory ClientFunc
}

const (
	EventGetPowerStateFailed = "GetPowerStateFailed"
	EventSetPowerStateFailed = "SetPowerStateFailed"
)

// NewMachineReconciler returns a new MachineReconciler.
func NewMachineReconciler(c client.Client, recorder record.EventRecorder, bmcClientFactory ClientFunc) *MachineReconciler {
	return &MachineReconciler{
		client:           c,
		recorder:         recorder,
		bmcClientFactory: bmcClientFactory,
	}
}

// machineFieldReconciler defines a function to reconcile Machine spec field.
type machineFieldReconciler func(context.Context, *v1alpha1.Machine, *bmclib.Client) error

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=machines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=machines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=machines/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

// Reconcile ensures the state of a Machine.
// Gets the Machine object and uses the SecretReference to initialize a BMC Client.
// Updates the Power status and conditions accordingly.
func (r *MachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithName("controllers/Machine").WithValues("Machine", req.NamespacedName)
	logger.Info("Reconciling Machine")

	// Fetch the Machine object
	machine := &v1alpha1.Machine{}
	err := r.client.Get(ctx, req.NamespacedName, machine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get Machine")
		return ctrl.Result{}, err
	}

	// Deletion is a noop.
	if !machine.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Create a patch from the initial Machine object
	// Patch is used to update Status after reconciliation
	machinePatch := client.MergeFrom(machine.DeepCopy())

	return r.doReconcile(ctx, machine, machinePatch, logger)
}

func (r *MachineReconciler) doReconcile(ctx context.Context, bm *v1alpha1.Machine, bmPatch client.Patch, logger logr.Logger) (ctrl.Result, error) {
	// Fetching username, password from SecretReference
	// Requeue if error fetching secret
	username, password, err := resolveAuthSecretRef(ctx, r.client, bm.Spec.Connection.AuthSecretRef)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("resolving Machine %s/%s SecretReference: %w", bm.Namespace, bm.Name, err)
	}

	// Initializing BMC Client
	bmcClient, err := r.bmcClientFactory(ctx, logger, bm.Spec.Connection.Host, strconv.Itoa(bm.Spec.Connection.Port), username, password)
	if err != nil {
		logger.Error(err, "BMC connection failed", "host", bm.Spec.Connection.Host)
		bm.SetCondition(v1alpha1.Contactable, v1alpha1.ConditionFalse, v1alpha1.WithMachineConditionMessage(err.Error()))
		if patchErr := r.patchStatus(ctx, bm, bmPatch); patchErr != nil {
			return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
		}

		return ctrl.Result{}, err
	}

	// Close BMC connection after reconciliation
	defer func() {
		if err := bmcClient.Close(ctx); err != nil {
			md := bmcClient.GetMetadata()
			logger.Error(err, "BMC close connection failed", "host", bm.Spec.Connection.Host, "providersAttempted", md.ProvidersAttempted)

			return
		}
		md := bmcClient.GetMetadata()
		logger.Info("BMC connection closed", "host", bm.Spec.Connection.Host, "successfulCloseConns", md.SuccessfulCloseConns, "providersAttempted", md.ProvidersAttempted, "successfulProvider", md.SuccessfulProvider)
	}()

	// Setting condition Contactable to True.
	bm.SetCondition(v1alpha1.Contactable, v1alpha1.ConditionTrue)

	// fieldReconcilers defines Machine spec field reconciler functions
	fieldReconcilers := []machineFieldReconciler{
		r.reconcilePower,
	}

	var aggErr utilerrors.Aggregate
	for _, reconiler := range fieldReconcilers {
		if err := reconiler(ctx, bm, bmcClient); err != nil {
			logger.Error(err, "Failed to reconcile Machine", "host", bm.Spec.Connection.Host)
			aggErr = utilerrors.NewAggregate([]error{err, aggErr})
		}
	}

	// Patch the status after each reconciliation
	if err := r.patchStatus(ctx, bm, bmPatch); err != nil {
		aggErr = utilerrors.NewAggregate([]error{err, aggErr})
	}

	return ctrl.Result{}, utilerrors.Flatten(aggErr)
}

// reconcilePower ensures the Machine Power is in the desired state.
func (r *MachineReconciler) reconcilePower(ctx context.Context, bm *v1alpha1.Machine, bmcClient *bmclib.Client) error {
	rawState, err := bmcClient.GetPowerState(ctx)
	if err != nil {
		r.recorder.Eventf(bm, corev1.EventTypeWarning, EventGetPowerStateFailed, "get power state: %v", err)
		return fmt.Errorf("get power state: %w", err)
	}

	state, err := convertRawBMCPowerState(rawState)
	if err != nil {
		return err
	}

	bm.Status.Power = state

	return nil
}

// patchStatus patches the specifies patch on the Machine.
func (r *MachineReconciler) patchStatus(ctx context.Context, bm *v1alpha1.Machine, patch client.Patch) error {
	err := r.client.Status().Patch(ctx, bm, patch)
	if err != nil {
		return fmt.Errorf("failed to patch Machine %s/%s status: %w", bm.Namespace, bm.Name, err)
	}

	return nil
}

// convertRawBMCPowerState takes a raw BMC power state response and attempts to convert it to
// a PowerState.
func convertRawBMCPowerState(response string) (v1alpha1.PowerState, error) {
	// Normalize the response string for comparison.
	response = strings.ToLower(response)

	switch {
	case strings.Contains(response, "on"):
		return v1alpha1.On, nil
	case strings.Contains(response, "off"):
		return v1alpha1.Off, nil
	}

	return "", fmt.Errorf("unknown bmc power state: %v", response)
}

// resolveAuthSecretRef Gets the Secret from the SecretReference.
// Returns the username and password encoded in the Secret.
func resolveAuthSecretRef(ctx context.Context, c client.Client, secretRef corev1.SecretReference) (string, string, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: secretRef.Namespace, Name: secretRef.Name}

	if err := c.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", fmt.Errorf("secret %s not found: %w", key, err)
		}

		return "", "", fmt.Errorf("failed to retrieve secret %s : %w", secretRef, err)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("'username' required in Machine secret")
	}

	password, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("'password' required in Machine secret")
	}

	return string(username), string(password), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Machine{}).
		Complete(r)
}
