package controller_test

import (
	"context"
	"testing"

	common "github.com/bmc-toolbox/common"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestMachineReconcileRefreshInventoryKeepsStatus covers a reconcile that collects inventory because
// the Machine asks for it with the refresh-inventory annotation. Clearing the annotation must not drop
// the power state and the Contactable condition the same reconcile sets.
func TestMachineReconcileRefreshInventoryKeepsStatus(t *testing.T) {
	bm := createMachine()
	bm.Annotations = map[string]string{"tinkerbell.org/refresh-inventory": trueString}
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{
		Powerstate: "on",
		InventoryDevice: &common.Device{
			BIOS: &common.BIOS{Common: common.Common{Vendor: "Dell Inc."}},
		},
	}

	// Status subresources for the kinds the reconcile writes, so status and the annotation go in
	// separate requests, as they do against an API server.
	s := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{bmc.AddToScheme, tinkerbell.AddToScheme, corev1.AddToScheme} {
		if err := add(s); err != nil {
			t.Fatal(err)
		}
	}
	client := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&bmc.Machine{}, &tinkerbell.Hardware{}).
		WithIndex(&tinkerbell.Hardware{}, controller.HardwareBMCRefIndexKey, controller.HardwareBMCRefIndexFunc).
		WithObjects(bm, hw, createSecret()).
		Build()

	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(10), newTestClient(provider), 0, 0, true)
	key := types.NamespacedName{Namespace: bm.Namespace, Name: bm.Name}
	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var got bmc.Machine
	if err := client.Get(context.Background(), key, &got); err != nil {
		t.Fatalf("failed to get Machine: %v", err)
	}
	if _, ok := got.Annotations["tinkerbell.org/refresh-inventory"]; ok {
		t.Error("refresh-inventory annotation still present, want it cleared after a successful collection")
	}
	if got.Status.Power != bmc.On {
		t.Errorf("Status.Power = %q, want %q", got.Status.Power, bmc.On)
	}
	contactable := false
	for _, c := range got.Status.Conditions {
		if c.Type == bmc.Contactable && c.Status == bmc.ConditionTrue {
			contactable = true
		}
	}
	if !contactable {
		t.Errorf("Contactable is not True, conditions: %+v", got.Status.Conditions)
	}
}
