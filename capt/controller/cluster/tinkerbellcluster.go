/*
Copyright 2022 The Tinkerbell Authors.

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

// Package cluster contains Cluster API controller for the TinkerbellCluster CR.
package cluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	infrastructurev1 "github.com/tinkerbell/tinkerbell/pkg/api/v1beta1/capt"
)

const (
	// ClusterNameLabel is used to mark Hardware as assigned controlplane machine.
	ClusterNameLabel = "v1alpha1.tinkerbell.org/clusterName"

	// ClusterNamespaceLabel is used to mark in which Namespace hardware is used.
	ClusterNamespaceLabel = "v1alpha1.tinkerbell.org/clusterNamespace"

	// KubernetesAPIPort is a port used by Tinkerbell clusters for Kubernetes API.
	KubernetesAPIPort = 6443
)

var (
	// ErrClusterNotReady is returned when trying to reconcile prior to the Cluster resource being ready.
	ErrClusterNotReady = fmt.Errorf("cluster resource not ready")
	// ErrControlPlaneEndpointNotSet is returned when trying to reconcile when the ControlPlane Endpoint is not defined.
	ErrControlPlaneEndpointNotSet = fmt.Errorf("controlplane endpoint is not set")
	// ErrConfigurationNil is the error returned when TinkerbellMachineReconciler or TinkerbellClusterReconciler is nil.
	ErrConfigurationNil = fmt.Errorf("configuration is nil")
	// ErrMissingClient is the error returned when TinkerbellMachineReconciler or TinkerbellClusterReconciler do
	// not have a Client configured.
	ErrMissingClient = fmt.Errorf("client is nil")
)

// TinkerbellClusterReconciler implements Reconciler interface.
type TinkerbellClusterReconciler struct {
	client.Client
	WatchFilterValue string
}

// validate validates if context configuration has all required fields properly populated.
func (tcr *TinkerbellClusterReconciler) validate() error {
	if tcr == nil {
		return ErrConfigurationNil
	}

	if tcr.Client == nil {
		return ErrMissingClient
	}

	return nil
}

// New builds a context for cluster reconciliation process, collecting all required
// information.
//
// If unexpected case occurs, error is returned.
//
// If some data is not yet available, nil is returned.
func (tcr *TinkerbellClusterReconciler) newReconcileContext(ctx context.Context, namespacedName types.NamespacedName) (*clusterReconcileContext, error) {
	log := ctrl.LoggerFrom(ctx)

	if err := tcr.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	crc := &clusterReconcileContext{
		log:               log.WithValues("tinkerbellcluster", namespacedName),
		ctx:               ctx,
		tinkerbellCluster: &infrastructurev1.TinkerbellCluster{},
		client:            tcr.Client,
		namespacedName:    namespacedName,
	}

	if err := crc.client.Get(crc.ctx, namespacedName, crc.tinkerbellCluster); err != nil {
		if apierrors.IsNotFound(err) {
			crc.log.Info("TinkerbellCluster object not found")

			return nil, nil
		}

		return nil, fmt.Errorf("getting TinkerbellCluster: %w", err)
	}

	patchHelper, err := patch.NewHelper(crc.tinkerbellCluster, crc.client)
	if err != nil {
		return nil, fmt.Errorf("initializing patch helper: %w", err)
	}

	crc.patchHelper = patchHelper

	cluster, err := util.GetOwnerCluster(crc.ctx, crc.client, crc.tinkerbellCluster.ObjectMeta)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("getting owner cluster: %w", err)
		}
	}

	if cluster == nil {
		crc.log.Info("OwnerCluster is not set yet.")
	}

	crc.cluster = cluster

	return crc, nil
}

// clusterReconcileContext implements ReconcileContext by reconciling TinkerbellCluster object.
type clusterReconcileContext struct {
	ctx               context.Context
	tinkerbellCluster *infrastructurev1.TinkerbellCluster
	patchHelper       *patch.Helper
	cluster           *clusterv1.Cluster
	log               logr.Logger
	client            client.Client
	namespacedName    types.NamespacedName
}

func (crc *clusterReconcileContext) controlPlaneEndpoint() (clusterv1.APIEndpoint, error) {
	switch {
	case crc.tinkerbellCluster.Spec.ControlPlaneEndpoint.IsValid():
		// If the ControlPlaneEndpoint on tinkCluster is already configured, return it.
		return crc.tinkerbellCluster.Spec.ControlPlaneEndpoint, nil
	case crc.cluster == nil:
		// If the owning cluster has not been set yet, error.
		return clusterv1.APIEndpoint{}, ErrClusterNotReady
	case crc.cluster.Spec.ControlPlaneEndpoint.IsValid():
		// If the ControlPlaneEndpoint on the cluster is already configured, return it.
		return crc.cluster.Spec.ControlPlaneEndpoint, nil
	}

	endpoint := clusterv1.APIEndpoint{
		Host: crc.cluster.Spec.ControlPlaneEndpoint.Host,
		Port: crc.cluster.Spec.ControlPlaneEndpoint.Port,
	}

	if endpoint.Host == "" {
		endpoint.Host = crc.tinkerbellCluster.Spec.ControlPlaneEndpoint.Host
	}

	if endpoint.Port == 0 {
		endpoint.Port = crc.tinkerbellCluster.Spec.ControlPlaneEndpoint.Port
	}

	if endpoint.Host == "" {
		return endpoint, ErrControlPlaneEndpointNotSet
	}

	if endpoint.Port == 0 {
		endpoint.Port = KubernetesAPIPort
	}

	return endpoint, nil
}

// Reconcile implements ReconcileContext interface by ensuring that all TinkerbellCluster object
// fields are properly populated.
func (crc *clusterReconcileContext) reconcile() error {
	controlPlaneEndpoint, err := crc.controlPlaneEndpoint()
	if err != nil {
		return err
	}

	// Ensure that we are setting the ControlPlaneEndpoint on the TinkerbellCluster
	// in the event that it was defined on the Cluster resource instead
	crc.tinkerbellCluster.Spec.ControlPlaneEndpoint.Host = controlPlaneEndpoint.Host
	crc.tinkerbellCluster.Spec.ControlPlaneEndpoint.Port = controlPlaneEndpoint.Port

	crc.tinkerbellCluster.Status.Ready = true

	crc.log.Info("Setting cluster status to ready")

	if err := crc.patchHelper.Patch(crc.ctx, crc.tinkerbellCluster); err != nil {
		return fmt.Errorf("patching cluster object: %w", err)
	}

	return nil
}

func (crc *clusterReconcileContext) reconcileDelete() error {
	return nil
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=tinkerbellclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=tinkerbellclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// Reconcile ensures state of Tinkerbell clusters.
func (tcr *TinkerbellClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	crc, err := tcr.newReconcileContext(ctx, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating reconciliation context: %w", err)
	}

	if crc == nil {
		return ctrl.Result{}, nil
	}

	if !crc.tinkerbellCluster.GetDeletionTimestamp().IsZero() {
		if annotations.HasPaused(crc.tinkerbellCluster) {
			crc.log.Info("TinkerbellCluster is marked as paused. Won't reconcile deletion")

			return ctrl.Result{}, nil
		}

		crc.log.Info("Removing cluster")

		return ctrl.Result{}, crc.reconcileDelete()
	}

	if crc.cluster == nil {
		return ctrl.Result{}, nil
	}

	if annotations.IsPaused(crc.cluster, crc.tinkerbellCluster) {
		crc.log.Info("TinkerbellCluster is marked as paused. Won't reconcile")

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, crc.reconcile()
}

// SetupWithManager configures reconciler with a given manager.
func (tcr *TinkerbellClusterReconciler) SetupWithManager(
	ctx context.Context,
	mgr ctrl.Manager,
	options controller.Options,
) error {
	log := ctrl.LoggerFrom(ctx)

	mapper := util.ClusterToInfrastructureMapFunc(
		ctx,
		infrastructurev1.GroupVersion.WithKind("TinkerbellCluster"),
		mgr.GetClient(),
		&infrastructurev1.TinkerbellCluster{},
	)

	bldr := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrastructurev1.TinkerbellCluster{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), log, tcr.WatchFilterValue)).
		WithEventFilter(predicates.ResourceIsNotExternallyManaged(mgr.GetScheme(), log)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(mapper),
			builder.WithPredicates(predicates.ClusterUnpaused(mgr.GetScheme(), log)),
		)

	if err := bldr.Complete(tcr); err != nil {
		return fmt.Errorf("failed to configure controller: %w", err)
	}

	return nil
}
