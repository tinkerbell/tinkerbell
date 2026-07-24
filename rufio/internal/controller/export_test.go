package controller

import (
	"context"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
)

// Exported for use by tests in the controller_test package. This file is only
// compiled during `go test` and never affects the package's real public API.
var (
	HardwareBMCRefIndexKey  = hardwareBMCRefIndexKey
	HardwareBMCRefIndexFunc = hardwareBMCRefIndexFunc
)

// ReconcileInventoryIfDueForTest exposes reconcileInventoryIfDue so tests can
// exercise BMC inventory collection directly, without driving a full
// Reconcile() (and therefore without needing a Machine object's own
// MergeFrom-based status patch to succeed against the fake client).
func (r *MachineReconciler) ReconcileInventoryIfDueForTest(ctx context.Context, logger logr.Logger, bmcClient *bmclib.Client, bm *bmc.Machine) {
	r.reconcileInventoryIfDue(ctx, logger, bmcClient, bm)
}
