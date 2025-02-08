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

package controller

import (
	"context"
	"fmt"
	"time"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/bmclib/v2/providers/rpc"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/bmc/v1alpha1"
)

// MachineReconciler reconciles a Machine object.
type MachineReconciler struct {
	client    client.Client
	recorder  record.EventRecorder
	bmcClient ClientFunc
}

const (
	// machineRequeueInterval is the interval at which the machine's power state is reconciled.
	// This should only be used when the power state was successfully retrieved.
	machineRequeueInterval = 3 * time.Minute
)

// NewMachineReconciler returns a new MachineReconciler.
func NewMachineReconciler(c client.Client, recorder record.EventRecorder, bmcClientFactory ClientFunc) *MachineReconciler {
	return &MachineReconciler{
		client:    c,
		recorder:  recorder,
		bmcClient: bmcClientFactory,
	}
}

//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=machines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=machines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=machines/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

// Reconcile reports on the state of a Machine. It does not change the state of the Machine in any way.
// Updates the Power status and conditions accordingly.
func (r *MachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx).WithName("controllers/Machine")
	logger.Info("reconciling machine")

	// Fetch the Machine object
	machine := &v1alpha1.Machine{}
	if err := r.client.Get(ctx, req.NamespacedName, machine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "failed to get Machine from KubeAPI")
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
	var username, password string
	opts := &BMCOptions{
		ProviderOptions: bm.Spec.Connection.ProviderOptions,
	}
	if bm.Spec.Connection.ProviderOptions != nil && bm.Spec.Connection.ProviderOptions.RPC != nil {
		opts.ProviderOptions = bm.Spec.Connection.ProviderOptions
		if len(bm.Spec.Connection.ProviderOptions.RPC.HMAC.Secrets) > 0 {
			se, err := retrieveHMACSecrets(ctx, r.client, bm.Spec.Connection.ProviderOptions.RPC.HMAC.Secrets)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("unable to get hmac secrets: %w", err)
			}
			opts.rpcSecrets = se
		}
	} else {
		// Fetching username, password from SecretReference
		// Requeue if error fetching secret
		var err error
		username, password, err = resolveAuthSecretRef(ctx, r.client, bm.Spec.Connection.AuthSecretRef)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("resolving Machine %s/%s SecretReference: %w", bm.Namespace, bm.Name, err)
		}
	}

	// Initializing BMC Client and Open the connection.
	bmcClient, err := r.bmcClient(ctx, logger, bm.Spec.Connection.Host, username, password, opts)
	if err != nil {
		logger.Error(err, "BMC connection failed", "host", bm.Spec.Connection.Host)
		bm.SetCondition(v1alpha1.Contactable, v1alpha1.ConditionFalse, v1alpha1.WithMachineConditionMessage(err.Error()))
		bm.Status.Power = v1alpha1.Unknown
		if patchErr := r.patchStatus(ctx, bm, bmPatch); patchErr != nil {
			return ctrl.Result{}, utilerrors.NewAggregate([]error{patchErr, err})
		}

		// requeue as bmc connections can be transient.
		return ctrl.Result{RequeueAfter: machineRequeueInterval}, nil
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

	contactable := v1alpha1.ConditionTrue
	conditionMsg := v1alpha1.WithMachineConditionMessage("")
	multiErr := []error{}
	pErr := r.updatePowerState(ctx, bm, bmcClient)
	if pErr != nil {
		logger.Error(pErr, "failed to get Machine power state", "host", bm.Spec.Connection.Host)
		contactable = v1alpha1.ConditionFalse
		conditionMsg = v1alpha1.WithMachineConditionMessage(pErr.Error())
		multiErr = append(multiErr, pErr)
	}

	// Set condition.
	bm.SetCondition(v1alpha1.Contactable, contactable, conditionMsg)

	// Patch the status after each reconciliation
	if err := r.patchStatus(ctx, bm, bmPatch); err != nil {
		multiErr = append(multiErr, err)
		return ctrl.Result{}, utilerrors.NewAggregate(multiErr)
	}

	return ctrl.Result{RequeueAfter: machineRequeueInterval}, nil
}

// updatePowerState gets the current power state of the machine.
func (r *MachineReconciler) updatePowerState(ctx context.Context, bm *v1alpha1.Machine, bmcClient *bmclib.Client) error {
	rawState, err := bmcClient.GetPowerState(ctx)
	if err != nil {
		bm.Status.Power = v1alpha1.Unknown
		r.recorder.Eventf(bm, corev1.EventTypeWarning, "GetPowerStateFailed", "get power state: %v", err)
		return fmt.Errorf("get power state: %w", err)
	}

	bm.Status.Power = toPowerState(rawState)

	return nil
}

// patchStatus patches the specifies patch on the Machine.
func (r *MachineReconciler) patchStatus(ctx context.Context, bm *v1alpha1.Machine, patch client.Patch) error {
	if err := r.client.Status().Patch(ctx, bm, patch); err != nil {
		return fmt.Errorf("failed to patch Machine %s/%s status: %w", bm.Namespace, bm.Name, err)
	}

	return nil
}

func retrieveHMACSecrets(ctx context.Context, c client.Client, hmacSecrets v1alpha1.HMACSecrets) (rpc.Secrets, error) {
	sec := rpc.Secrets{}
	for k, v := range hmacSecrets {
		for _, s := range v {
			secret := &corev1.Secret{}
			key := types.NamespacedName{Namespace: s.Namespace, Name: s.Name}

			if err := c.Get(ctx, key, secret); err != nil {
				if apierrors.IsNotFound(err) {
					return nil, fmt.Errorf("secret %s not found: %w", key, err)
				}

				return nil, fmt.Errorf("failed to retrieve secret %s : %w", s, err)
			}

			sec[rpc.Algorithm(k)] = append(sec[rpc.Algorithm(k)], string(secret.Data["secret"]))
		}
	}

	return sec, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Machine{}).
		Complete(r)
}
