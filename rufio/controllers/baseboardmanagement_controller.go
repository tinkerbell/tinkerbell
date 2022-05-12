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
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/rufio/pkg/bmcclient"
)

// BaseboardManagementReconciler reconciles a BaseboardManagement object
type BaseboardManagementReconciler struct {
	client           client.Client
	scheme           *runtime.Scheme
	bmcClientFactory bmcclient.BMCClientFactory
	logger           logr.Logger
}

// NewBaseboardManagementReconciler returns a new BaseboardManagementReconciler
func NewBaseboardManagementReconciler(client client.Client, scheme *runtime.Scheme, bmcClientFactory bmcclient.BMCClientFactory, logger logr.Logger) *BaseboardManagementReconciler {
	return &BaseboardManagementReconciler{
		client:           client,
		scheme:           scheme,
		bmcClientFactory: bmcClientFactory,
		logger:           logger,
	}
}

// bmFieldReconciler defines a function to reconcile BaseboardManagement spec field
type bmFieldReconciler func(context.Context, *bmcv1alpha1.BaseboardManagement, bmcclient.BMCClient) error

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=baseboardmanagements,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=baseboardmanagements/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=baseboardmanagements/finalizers,verbs=update

// Reconcile ensures the state of a BaseboardManagement.
// Gets the BaseboardManagement object and uses the SecretReference to initialize a BMC Client.
// Ensures the BMC power is set to the desired state.
// Updates the status and conditions accordingly.
func (r *BaseboardManagementReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.WithValues("BaseboardManagement", req.NamespacedName)
	logger.Info("Reconciling BaseboardManagement", "name", req.NamespacedName)

	// Fetch the BaseboardManagement object
	baseboardManagement := &bmcv1alpha1.BaseboardManagement{}
	err := r.client.Get(ctx, req.NamespacedName, baseboardManagement)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get BaseboardManagement", "name", req.NamespacedName)
		return ctrl.Result{}, fmt.Errorf("failed to get BaseboardManagement: %v", err)
	}

	// Deletion is a noop.
	if !baseboardManagement.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	return r.reconcile(ctx, baseboardManagement)
}

func (r *BaseboardManagementReconciler) reconcile(ctx context.Context, bm *bmcv1alpha1.BaseboardManagement) (ctrl.Result, error) {
	logger := r.logger.WithValues("BaseboardManagement", bm.Name, "Namespace", bm.Namespace)

	// Fetching username, password from SecretReference
	// Requeue if error fetching secret
	username, password, err := r.resolveAuthSecretRef(ctx, bm.Spec.Connection.AuthSecretRef)
	if err != nil {
		return ctrl.Result{Requeue: true}, fmt.Errorf("resolving authentication from SecretReference: %v", err)
	}

	// TODO (pokearu): Remove port hardcoding
	// Initializing BMC Client
	bmcClient := r.bmcClientFactory(bm.Spec.Connection.Host, "623", username, password)
	err = bmcClient.OpenConnection(ctx)
	if err != nil {
		logger.Error(err, "BMC connection failed", "host", bm.Spec.Connection.Host)
		result, setConditionErr := r.setCondition(ctx, bm, bmcv1alpha1.ConnectionError, err.Error())
		if setConditionErr != nil {
			return result, utilerrors.NewAggregate([]error{fmt.Errorf("failed to set conditions: %v", setConditionErr), err})
		}
		return result, err
	}

	// Close BMC connection after reconcilation
	defer func() {
		err = bmcClient.CloseConnection(ctx)
		if err != nil {
			logger.Error(err, "BMC close connection failed", "host", bm.Spec.Connection.Host)
		}
	}()

	// fieldReconcilers defines BaseboardManagement spec field reconciler functions
	fieldReconcilers := []bmFieldReconciler{
		r.reconcilePower,
	}
	for _, reconiler := range fieldReconcilers {
		err := reconiler(ctx, bm, bmcClient)
		if err != nil {
			logger.Error(err, "Failed to reconcile BaseboardManagement", "host", bm.Spec.Connection.Host)
			return ctrl.Result{}, err
		}
	}

	// Patch the status after each reconciliation
	return r.reconcileStatus(ctx, bm)
}

func (r *BaseboardManagementReconciler) reconcilePower(ctx context.Context, bm *bmcv1alpha1.BaseboardManagement, bmcClient bmcclient.BMCClient) error {
	powerStatus, err := bmcClient.GetPowerStatus(ctx)
	if err != nil {
		return err
	}

	// If BaseboardManagement has desired power state then return
	if bm.Spec.Power == bmcv1alpha1.PowerState(strings.ToLower(powerStatus)) {
		return nil
	}

	// Setting baseboard management to desired power state
	err = bmcClient.SetPowerState(ctx, string(bm.Spec.Power))
	if err != nil {
		return err
	}

	return nil
}

// setCondition updates the status.Condition if the condition type is present.
// Appends if new condition is found.
// Patches the BaseboardManagement status.
func (r *BaseboardManagementReconciler) setCondition(ctx context.Context, bm *bmcv1alpha1.BaseboardManagement, cType bmcv1alpha1.BaseboardManagementConditionType, message string) (ctrl.Result, error) {
	patch := client.MergeFrom(bm.DeepCopy())

	currentConditions := bm.Status.Conditions
	for i := range currentConditions {
		// If condition exists, update the message if different
		if currentConditions[i].Type == cType {
			if currentConditions[i].Message != message {
				bm.Status.Conditions[i].Message = message
				return r.patchStatus(ctx, bm, patch)
			}
			return ctrl.Result{}, nil
		}
	}

	// Append new condition to Conditions
	condition := bmcv1alpha1.BaseboardManagementCondition{
		Type:    cType,
		Message: message,
	}
	bm.Status.Conditions = append(bm.Status.Conditions, condition)

	return r.patchStatus(ctx, bm, patch)
}

// reconcileStatus updates the Power and Conditions and patches BaseboardManagement status.
func (r *BaseboardManagementReconciler) reconcileStatus(ctx context.Context, bm *bmcv1alpha1.BaseboardManagement) (ctrl.Result, error) {
	patch := client.MergeFrom(bm.DeepCopy())

	// Update the power status of the BaseboardManagement
	bm.Status.Power = bm.Spec.Power
	// Clear conditions
	bm.Status.Conditions = []bmcv1alpha1.BaseboardManagementCondition{}

	return r.patchStatus(ctx, bm, patch)
}

// patchStatus patches the specifies patch on the BaseboardManagement.
func (r *BaseboardManagementReconciler) patchStatus(ctx context.Context, bm *bmcv1alpha1.BaseboardManagement, patch client.Patch) (ctrl.Result, error) {
	logger := r.logger.WithValues("BaseboardManagement", bm.Name, "Namespace", bm.Namespace)

	err := r.client.Status().Patch(ctx, bm, patch)
	if err != nil {
		logger.Error(err, "Failed to patch BaseboardManagement status")
		return ctrl.Result{}, fmt.Errorf("failed to patch BaseboardManagement status: %v", err)
	}

	return ctrl.Result{}, nil
}

// resolveAuthSecretRef Gets the Secret from the SecretReference.
// Returns the username and password encoded in the Secret.
func (r *BaseboardManagementReconciler) resolveAuthSecretRef(ctx context.Context, secretRef corev1.SecretReference) (string, string, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: secretRef.Namespace, Name: secretRef.Name}

	if err := r.client.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", "", fmt.Errorf("secret %s not found: %v", key, err)
		}

		return "", "", fmt.Errorf("failed to retrieve secret %s : %v", secretRef, err)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("required secret key username not present")
	}

	password, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("required secret key password not present")
	}

	return string(username), string(password), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BaseboardManagementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bmcv1alpha1.BaseboardManagement{}).
		Complete(r)
}
