package controller_test

import (
	"context"
	"errors"
	"testing"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	common "github.com/bmc-toolbox/common"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// newInventoryTestClient builds a fake client that keeps controller-runtime's
// default FieldManagedObjectTracker (unlike newClientBuilder() above, which
// swaps in a basic ObjectTracker specifically to work around a MergeFrom+Machine
// quirk that's unrelated to this file). It supports real Server-Side Apply
// patch semantics for Hardware, which applyBMCInventory relies on — unlike
// newClientBuilder()'s SubResourcePatch interceptor, which deliberately ignores
// the patch content and does a full Update instead (see its doc comment).
func newInventoryTestClient(objs ...ctrlclient.Object) ctrlclient.Client {
	s := runtime.NewScheme()
	if err := api.AddToSchemeTinkerbell(s); err != nil {
		panic(err)
	}
	if err := api.AddToSchemeBMC(s); err != nil {
		panic(err)
	}
	if err := scheme.AddToScheme(s); err != nil {
		panic(err)
	}

	return fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&tinkerbell.Hardware{}).
		WithIndex(&tinkerbell.Hardware{}, controller.HardwareBMCRefIndexKey, controller.HardwareBMCRefIndexFunc).
		WithObjects(objs...).
		Build()
}

func createHardwareForMachine(machineName string) *tinkerbell.Hardware {
	return &tinkerbell.Hardware{
		TypeMeta: metav1.TypeMeta{
			APIVersion: tinkerbell.GroupVersion.String(),
			Kind:       "Hardware",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hw",
			Namespace: "test-namespace",
		},
		Spec: tinkerbell.HardwareSpec{
			BMCRef: &corev1.TypedLocalObjectReference{
				Name: machineName,
			},
		},
	}
}

// newOpenTestBMCClient opens a bmclib.Client wrapping the given testProvider,
// the same way MachineReconciler's own bmcClient factory does, without going
// through a full Reconcile() call.
func newOpenTestBMCClient(t *testing.T, provider *testProvider) *bmclib.Client {
	t.Helper()
	cl, err := newTestClient(provider)(context.Background(), logr.Discard(), "0.0.0.0", "user", "pass", &controller.BMCOptions{})
	if err != nil {
		t.Fatalf("failed to open test BMC client: %v", err)
	}
	return cl
}

// TestReconcileInventoryIfDue_Success verifies that a successful Inventory()
// call is mapped and written to the linked Hardware's status.bmcInventory, with
// CollectionMethod set from the provider that produced it.
func TestReconcileInventoryIfDue_Success(t *testing.T) {
	bm := createMachine()
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{
		InventoryDevice: &common.Device{
			BIOS: &common.BIOS{Common: common.Common{Vendor: "Dell Inc."}},
		},
	}

	client := newInventoryTestClient(hw)
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0)
	bmcClient := newOpenTestBMCClient(t, provider)

	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	var got tinkerbell.Hardware
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "test-namespace", Name: "test-hw"}, &got); err != nil {
		t.Fatalf("failed to get Hardware: %v", err)
	}

	if got.Status.BMCInventory == nil {
		t.Fatal("Status.BMCInventory is nil, want it populated")
	}
	if got.Status.BMCInventory.BIOS == nil || got.Status.BMCInventory.BIOS.Vendor != "Dell Inc." {
		t.Errorf("Status.BMCInventory.BIOS = %+v, want Vendor=Dell Inc.", got.Status.BMCInventory.BIOS)
	}
	if got.Status.BMCInventory.CollectionMethod == "" {
		t.Error("Status.BMCInventory.CollectionMethod is empty, want it set from the winning provider")
	}
	if got.Status.BMCInventory.LastUpdated == nil {
		t.Error("Status.BMCInventory.LastUpdated is nil, want it set")
	}
}

// TestReconcileInventoryIfDue_ClearsRefreshAnnotationOnSuccess verifies that a
// successful collection triggered by the tinkerbell.org/refresh-inventory
// annotation clears that annotation afterward, so it doesn't keep forcing
// collection on every reconcile if the caller forgets to remove it themselves.
func TestReconcileInventoryIfDue_ClearsRefreshAnnotationOnSuccess(t *testing.T) {
	bm := createMachine()
	bm.Annotations = map[string]string{"tinkerbell.org/refresh-inventory": "true"}
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{
		InventoryDevice: &common.Device{
			BIOS: &common.BIOS{Common: common.Common{Vendor: "Dell Inc."}},
		},
	}

	client := newInventoryTestClient(hw, bm)
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0)
	bmcClient := newOpenTestBMCClient(t, provider)

	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	var got bmc.Machine
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "test-namespace", Name: "test-bm"}, &got); err != nil {
		t.Fatalf("failed to get Machine: %v", err)
	}
	if _, ok := got.Annotations["tinkerbell.org/refresh-inventory"]; ok {
		t.Error("refresh-inventory annotation still present after a successful manually-triggered collection, want it cleared")
	}
}

// TestReconcileInventoryIfDue_IPMIOnlyHardError verifies that a provider which
// cannot produce inventory at all (mirroring bmclib's ipmitool driver, which
// does not implement InventoryGetter) does not error out to the caller and does
// not create a partial/empty status.bmcInventory.
func TestReconcileInventoryIfDue_IPMIOnlyHardError(t *testing.T) {
	bm := createMachine()
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{
		ErrInventory: errors.New("not a InventoryGetter implementation"),
	}

	client := newInventoryTestClient(hw)
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0)
	bmcClient := newOpenTestBMCClient(t, provider)

	// reconcileInventoryIfDue has no return value — failures are logged/evented,
	// never propagated. This call must not panic.
	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	var got tinkerbell.Hardware
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "test-namespace", Name: "test-hw"}, &got); err != nil {
		t.Fatalf("failed to get Hardware: %v", err)
	}
	if got.Status.BMCInventory != nil {
		t.Errorf("Status.BMCInventory = %+v, want nil after a failed collection", got.Status.BMCInventory)
	}
}

// TestReconcileInventoryIfDue_Cadence verifies that inventory is only collected
// once it's due (see dueForInventoryRefresh), not on every call — mirroring the
// 3-minute power-poll reconcile cadence in doReconcile. Repeated calls right
// after a successful collection must not call Inventory() again.
func TestReconcileInventoryIfDue_Cadence(t *testing.T) {
	bm := createMachine()
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{
		InventoryDevice: &common.Device{
			BIOS: &common.BIOS{Common: common.Common{Vendor: "Dell Inc."}},
		},
	}

	client := newInventoryTestClient(hw)
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(4), newTestClient(provider), 0)
	bmcClient := newOpenTestBMCClient(t, provider)

	for i := 0; i < 3; i++ {
		reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)
	}

	if provider.InventoryCalls != 1 {
		t.Errorf("Inventory() called %d times across 3 calls, want 1 (cadence gating should skip once collected)", provider.InventoryCalls)
	}
}

// TestReconcileInventoryIfDue_NoLinkedHardware verifies that a Machine with no
// linked Hardware (spec.bmcRef not pointed at by any Hardware) is a no-op —
// Inventory() must never be called.
func TestReconcileInventoryIfDue_NoLinkedHardware(t *testing.T) {
	bm := createMachine()
	provider := &testProvider{}

	client := newInventoryTestClient() // no Hardware objects at all
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0)
	bmcClient := newOpenTestBMCClient(t, provider)

	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	if provider.InventoryCalls != 0 {
		t.Errorf("Inventory() called %d times with no linked Hardware, want 0", provider.InventoryCalls)
	}
}

// TestReconcileInventoryIfDue_AmbiguousHardware verifies that when more than
// one Hardware object references the same Machine via spec.bmcRef, inventory
// collection is skipped entirely (ambiguous target) rather than silently
// picking one of them arbitrarily, which could flap between reconciles.
func TestReconcileInventoryIfDue_AmbiguousHardware(t *testing.T) {
	bm := createMachine()
	hw1 := createHardwareForMachine(bm.Name)
	hw2 := createHardwareForMachine(bm.Name)
	hw2.Name = "test-hw-2"
	provider := &testProvider{}

	client := newInventoryTestClient(hw1, hw2)
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0)
	bmcClient := newOpenTestBMCClient(t, provider)

	// Must not panic despite the ambiguous match.
	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	if provider.InventoryCalls != 0 {
		t.Errorf("Inventory() called %d times with an ambiguous Hardware link, want 0", provider.InventoryCalls)
	}
}

// TestMachineReconcile_WithHardwareLinked is an end-to-end smoke test through
// the real Reconcile() entry point, confirming the inventory step doesn't break
// normal Machine reconciliation when a linked Hardware object exists (using
// newClientBuilder(), the same fixture used by every other Machine test in this
// package — this deliberately does not assert on BMCInventory content; that's
// covered directly above).
func TestMachineReconcile_WithHardwareLinked(t *testing.T) {
	bm := createMachine()
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{Powerstate: "on"}

	client := newClientBuilder().
		WithObjects(bm, hw, createSecret()).
		Build()

	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0)
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "test-namespace", Name: "test-bm"}}

	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}
