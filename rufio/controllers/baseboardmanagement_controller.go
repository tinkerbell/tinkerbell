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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
)

// BMCClient represents a baseboard management controller client.
// It defines a set of methods to connect and interact with a BMC.
type BMCClient interface {
	// Close ends the connection with the bmc.
	Close(ctx context.Context) error
	// GetPowerState fetches the current power status of the bmc.
	GetPowerState(ctx context.Context) (string, error)
	// SetPowerState power controls the bmc to the input power state.
	SetPowerState(ctx context.Context, state string) (bool, error)
	// SetBootDevice sets the boot device on the bmc.
	// Currently this sets the first boot device.
	// setPersistent, if true will set the boot device permanently. If false, sets one time boot.
	// efiBoot, if true passes efiboot options while setting boot device.
	SetBootDevice(ctx context.Context, bootDevice string, setPersistent, efiBoot bool) (bool, error)
}

// BMCClientFactoryFunc defines a func that returns a BMCClient
type BMCClientFactoryFunc func(ctx context.Context, hostIP, port, username, password string) (BMCClient, error)

// BaseboardManagementReconciler reconciles a BaseboardManagement object
type BaseboardManagementReconciler struct {
	client           client.Client
	recorder         record.EventRecorder
	bmcClientFactory BMCClientFactoryFunc
	logger           logr.Logger
}

const (
	EventGetPowerStateFailed = "GetPowerStateFailed"
	EventSetPowerStateFailed = "SetPowerStateFailed"
)

// NewBaseboardManagementReconciler returns a new BaseboardManagementReconciler
func NewBaseboardManagementReconciler(client client.Client, recorder record.EventRecorder, bmcClientFactory BMCClientFactoryFunc, logger logr.Logger) *BaseboardManagementReconciler {
	return &BaseboardManagementReconciler{
		client:           client,
		recorder:         recorder,
		bmcClientFactory: bmcClientFactory,
		logger:           logger,
	}
}

// baseboardManagementFieldReconciler defines a function to reconcile BaseboardManagement spec field
type baseboardManagementFieldReconciler func(context.Context, *bmcv1alpha1.BaseboardManagement, BMCClient) error

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=baseboardmanagements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=baseboardmanagements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=baseboardmanagements/finalizers,verbs=update

// Reconcile ensures the state of a BaseboardManagement.
// Gets the BaseboardManagement object and uses the SecretReference to initialize a BMC Client.
// Ensures the BMC power is set to the desired state.
// Updates the status and conditions accordingly.
func (r *BaseboardManagementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.WithValues("BaseboardManagement", req.NamespacedName)
	logger.Info("Reconciling BaseboardManagement")

	// Fetch the BaseboardManagement object
	baseboardManagement := &bmcv1alpha1.BaseboardManagement{}
	err := r.client.Get(ctx, req.NamespacedName, baseboardManagement)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get BaseboardManagement")
		return ctrl.Result{}, err
	}

	// Create a patch from the initial BaseboardManagement object
	// Patch is used to update Status after reconciliation
	baseboardManagementPatch := client.MergeFrom(baseboardManagement.DeepCopy())

	// Deletion is a noop.
	if !baseboardManagement.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	return r.reconcile(ctx, baseboardManagement, baseboardManagementPatch, logger)
}

func (r *BaseboardManagementReconciler) reconcile(ctx context.Context, bm *bmcv1alpha1.BaseboardManagement, bmPatch client.Patch, logger logr.Logger) (ctrl.Result, error) {
	// Fetching username, password from SecretReference
	// Requeue if error fetching secret
	username, password, err := resolveAuthSecretRef(ctx, r.client, bm.Spec.Connection.AuthSecretRef)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("resolving BaseboardManagement %s/%s SecretReference: %v", bm.Namespace, bm.Name, err)
	}

	// Initializing BMC Client
	bmcClient, err := r.bmcClientFactory(ctx, bm.Spec.Connection.Host, strconv.Itoa(bm.Spec.Connection.Port), username, password)
	if err != nil {
		logger.Error(err, "BMC connection failed", "host", bm.Spec.Connection.Host)
		bm.SetCondition(bmcv1alpha1.Contactable, bmcv1alpha1.BaseboardManagementConditionFalse, bmcv1alpha1.WithBaseboardManagementConditionMessage(err.Error()))
		result, patchErr := r.patchStatus(ctx, bm, bmPatch)
		if patchErr != nil {
			return result, utilerrors.NewAggregate([]error{patchErr, err})
		}

		return result, err
	}
	// Setting condition Contactable to True.
	bm.SetCondition(bmcv1alpha1.Contactable, bmcv1alpha1.BaseboardManagementConditionTrue)

	// Close BMC connection after reconcilation
	defer func() {
		err = bmcClient.Close(ctx)
		if err != nil {
			logger.Error(err, "BMC close connection failed", "host", bm.Spec.Connection.Host)
		}
	}()

	// fieldReconcilers defines BaseboardManagement spec field reconciler functions
	fieldReconcilers := []baseboardManagementFieldReconciler{
		r.reconcilePower,
	}

	var aggErr utilerrors.Aggregate
	for _, reconiler := range fieldReconcilers {
		if err := reconiler(ctx, bm, bmcClient); err != nil {
			logger.Error(err, "Failed to reconcile BaseboardManagement", "host", bm.Spec.Connection.Host)
			aggErr = utilerrors.NewAggregate([]error{err, aggErr})
		}
	}

	// Patch the status after each reconciliation
	result, err := r.patchStatus(ctx, bm, bmPatch)
	if err != nil {
		aggErr = utilerrors.NewAggregate([]error{err, aggErr})
	}

	return result, utilerrors.Flatten(aggErr)
}

// reconcilePower ensures the BaseboardManagement Power is in the desired state.
func (r *BaseboardManagementReconciler) reconcilePower(ctx context.Context, bm *bmcv1alpha1.BaseboardManagement, bmcClient BMCClient) error {
	powerStatus, err := bmcClient.GetPowerState(ctx)
	if err != nil {
		r.recorder.Eventf(bm, corev1.EventTypeWarning, EventGetPowerStateFailed, "failed to get power state: %v", err)
		return fmt.Errorf("failed to get power state: %v", err)
	}

	// If BaseboardManagement has desired power state then return
	if bm.Spec.Power == bmcv1alpha1.PowerState(strings.ToLower(powerStatus)) {
		// Update status to represent current power state
		bm.Status.Power = bm.Spec.Power
		return nil
	}

	// Setting baseboard management to desired power state
	_, err = bmcClient.SetPowerState(ctx, string(bm.Spec.Power))
	if err != nil {
		r.recorder.Eventf(bm, corev1.EventTypeWarning, EventSetPowerStateFailed, "failed to set power state: %v", err)
		return fmt.Errorf("failed to set power state: %v", err)
	}

	// Update status to represent current power state
	bm.Status.Power = bm.Spec.Power

	return nil
}

// patchStatus patches the specifies patch on the BaseboardManagement.
func (r *BaseboardManagementReconciler) patchStatus(ctx context.Context, bm *bmcv1alpha1.BaseboardManagement, patch client.Patch) (ctrl.Result, error) {
	err := r.client.Status().Patch(ctx, bm, patch)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch BaseboardManagement %s/%s status: %v", bm.Namespace, bm.Name, err)
	}

	return ctrl.Result{}, nil
}

// resolveAuthSecretRef Gets the Secret from the SecretReference.
// Returns the username and password encoded in the Secret.
func resolveAuthSecretRef(ctx context.Context, c client.Client, secretRef corev1.SecretReference) (string, string, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: secretRef.Namespace, Name: secretRef.Name}

	if err := c.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", fmt.Errorf("secret %s not found: %v", key, err)
		}

		return "", "", fmt.Errorf("failed to retrieve secret %s : %v", secretRef, err)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("'username' required in BaseboardManagement secret")
	}

	password, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("'password' required in BaseboardManagement secret")
	}

	return string(username), string(password), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BaseboardManagementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bmcv1alpha1.BaseboardManagement{}).
		Complete(r)
}
