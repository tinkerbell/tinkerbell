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

// Package machine contains Cluster API controller for the TinkerbellMachine CR.
package machine

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	rufiov1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/bmc"
	tinkv1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"

	infrastructurev1 "github.com/tinkerbell/tinkerbell/pkg/api/v1beta1/capt"
)

// TinkerbellMachineReconciler implements Reconciler interface by managing Tinkerbell machines.
type TinkerbellMachineReconciler struct {
	Client           client.Client
	WatchFilterValue string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=tinkerbellmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=tinkerbellmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch
// +kubebuilder:rbac:groups=tinkerbell.org,resources=hardware;hardware/status,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=tinkerbell.org,resources=templates;templates/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tinkerbell.org,resources=workflows;workflows/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bmc.tinkerbell.org,resources=jobs,verbs=get;list;watch;create

// Reconcile ensures that all Tinkerbell machines are aligned with a given spec.
func (r *TinkerbellMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// If the TinkerbellMachineReconciler instant is invalid we can't continue. There's also no way
	// for us to recover the TinkerbellMachineReconciler instance (i.e. there's a programmer error).
	// To avoid continuously requeueing resources for the controller that will never resolve its
	// problem, we panic.
	if err := r.validate(); err != nil {
		panic(err)
	}

	log := ctrl.LoggerFrom(ctx)
	log.Info("starting reconcile")

	scope := &machineReconcileScope{
		log:               log,
		ctx:               ctx,
		tinkerbellMachine: &infrastructurev1.TinkerbellMachine{},
		client:            r.Client,
	}

	if err := r.Client.Get(ctx, req.NamespacedName, scope.tinkerbellMachine); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("TinkerbellMachine not found")

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get TinkerbellMachine: %w", err)
	}

	patchHelper, err := patch.NewHelper(scope.tinkerbellMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("initialize patch helper: %w", err)
	}

	scope.patchHelper = patchHelper

	if scope.MachineScheduledForDeletion() {
		return ctrl.Result{}, scope.DeleteMachineWithDependencies()
	}

	// We must be bound to a CAPI Machine object before we can continue.
	machine, err := scope.getReadyMachine()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting valid Machine object: %w", err)
	}

	if machine == nil {
		return ctrl.Result{}, nil
	}

	// We need a bootstrap cloud config secret to bootstrap the node so we can't proceed without it.
	// Typically, this is something akin to cloud-init user-data.
	bootstrapCloudConfig, err := scope.getReadyBootstrapCloudConfig(machine)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("receiving bootstrap cloud config: %w", err)
	}

	if bootstrapCloudConfig == "" {
		const requeueAfter = 30 * time.Second

		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	tinkerbellCluster, err := scope.getReadyTinkerbellCluster(machine)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting TinkerbellCluster: %w", err)
	}

	if tinkerbellCluster == nil {
		log.Info("TinkerbellCluster is not ready yet")

		return ctrl.Result{}, nil
	}

	scope.machine = machine
	scope.bootstrapCloudConfig = bootstrapCloudConfig
	scope.tinkerbellCluster = tinkerbellCluster

	return ctrl.Result{}, scope.Reconcile()
}

// SetupWithManager configures reconciler with a given manager.
func (r *TinkerbellMachineReconciler) SetupWithManager(
	ctx context.Context,
	mgr ctrl.Manager,
	options controller.Options,
) error {
	log := ctrl.LoggerFrom(ctx)

	clusterToObjectFunc, err := util.ClusterToTypedObjectsMapper(
		r.Client,
		&infrastructurev1.TinkerbellMachineList{},
		mgr.GetScheme(),
	)
	if err != nil {
		return fmt.Errorf("failed to create mapper for Cluster to TinkrebellMachines: %w", err)
	}

	bldr := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		For(&infrastructurev1.TinkerbellMachine{}).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(
				util.MachineToInfrastructureMapFunc(infrastructurev1.GroupVersion.WithKind("TinkerbellMachine")),
			),
		).
		Watches(
			&infrastructurev1.TinkerbellCluster{},
			handler.EnqueueRequestsFromMapFunc(r.TinkerbellClusterToTinkerbellMachines(ctx)),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToObjectFunc),
			builder.WithPredicates(predicates.ClusterPausedTransitionsOrInfrastructureReady(mgr.GetScheme(), log)),
		).
		Watches(
			&tinkv1.Workflow{},
			handler.EnqueueRequestForOwner(
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&infrastructurev1.TinkerbellMachine{},
				handler.OnlyControllerOwner(),
			),
		).
		Watches(
			&rufiov1.Job{},
			handler.EnqueueRequestForOwner(
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&infrastructurev1.TinkerbellMachine{},
				handler.OnlyControllerOwner(),
			),
		)

	if err := bldr.Complete(r); err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	return nil
}

// TinkerbellClusterToTinkerbellMachines is a handler.ToRequestsFunc to be used to enqeue requests for reconciliation
// of TinkerbellMachines.
func (r *TinkerbellMachineReconciler) TinkerbellClusterToTinkerbellMachines(ctx context.Context) handler.MapFunc {
	log := ctrl.LoggerFrom(ctx)

	return func(ctx context.Context, o client.Object) []ctrl.Request {
		c, ok := o.(*infrastructurev1.TinkerbellCluster)
		if !ok {
			log.Error(
				fmt.Errorf("expected a TinkerbellCluster but got a %T", o),
				"failed to get TinkerbellMachine for TinkerbellCluster",
			)

			return nil
		}

		log = log.WithValues("TinkerbellCluster", c.Name, "Namespace", c.Namespace)

		// Don't handle deleted TinkerbellClusters
		if !c.DeletionTimestamp.IsZero() {
			log.V(4).Info("TinkerbellCluster has a deletion timestamp, skipping mapping.")

			return nil
		}

		cluster, err := util.GetOwnerCluster(ctx, r.Client, c.ObjectMeta)

		switch {
		case apierrors.IsNotFound(err) || cluster == nil:
			log.Error(err, "owning cluster is not found, skipping mapping.")

			return nil
		case err != nil:
			log.Error(err, "failed to get owning cluster")

			return nil
		}

		machines, err := collections.GetFilteredMachinesForCluster(ctx, r.Client, cluster)
		if err != nil {
			log.Error(err, "failed to get Machines for Cluster")

			return nil
		}

		var result []ctrl.Request

		for _, m := range machines.UnsortedList() {
			if m.Spec.InfrastructureRef.Name == "" {
				continue
			}

			name := client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.InfrastructureRef.Name}

			result = append(result, ctrl.Request{NamespacedName: name})
		}

		return result
	}
}

// validate validates if context configuration has all required fields properly populated.
func (r *TinkerbellMachineReconciler) validate() error {
	if r == nil {
		return ErrConfigurationNil
	}

	if r.Client == nil {
		return ErrMissingClient
	}

	return nil
}
