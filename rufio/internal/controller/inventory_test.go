package controller_test

import (
	"context"
	"errors"
	"testing"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/bmclib/v2/providers"
	common "github.com/bmc-toolbox/common"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/jacobweinstock/registrar"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const trueString = "true"

// newInventoryTestClient builds a fake client that keeps controller-runtime's
// default FieldManagedObjectTracker (unlike newClientBuilder() above, which
// swaps in a basic ObjectTracker specifically to work around a MergeFrom+Machine
// quirk that's unrelated to this file). It supports real Server-Side Apply
// patch semantics for Hardware, which applyOutOfBandAttributes relies on — unlike
// newClientBuilder()'s SubResourcePatch interceptor, which deliberately ignores
// the patch content and does a full Update instead (see its doc comment).
func newInventoryTestClient(objs ...ctrlclient.Object) ctrlclient.Client {
	s := runtime.NewScheme()
	if err := tinkerbell.AddToScheme(s); err != nil {
		panic(err)
	}
	if err := bmc.AddToScheme(s); err != nil {
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
// call is mapped and written to the linked Hardware's status.attributes.outOfBand, with
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
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0, 0, true)
	bmcClient := newOpenTestBMCClient(t, provider)

	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	var got tinkerbell.Hardware
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "test-namespace", Name: "test-hw"}, &got); err != nil {
		t.Fatalf("failed to get Hardware: %v", err)
	}

	if controller.OutOfBandAttributes(&got) == nil {
		t.Fatal("Status.Attributes.OutOfBand is nil, want it populated")
	}
	if controller.OutOfBandAttributes(&got).BIOS == nil || controller.OutOfBandAttributes(&got).BIOS.Vendor != "Dell Inc." {
		t.Errorf("Status.Attributes.OutOfBand.BIOS = %+v, want Vendor=Dell Inc.", controller.OutOfBandAttributes(&got).BIOS)
	}
	if controller.OutOfBandAttributes(&got).CollectionMethod == "" {
		t.Error("Status.Attributes.OutOfBand.CollectionMethod is empty, want it set from the winning provider")
	}
	if controller.OutOfBandAttributes(&got).LastUpdated == nil {
		t.Error("Status.Attributes.OutOfBand.LastUpdated is nil, want it set")
	}
}

// TestReconcileInventoryIfDue_ClearsRefreshAnnotationOnSuccess verifies that a
// successful collection triggered by the tinkerbell.org/refresh-inventory
// annotation clears that annotation afterward, so it doesn't keep forcing
// collection on every reconcile if the caller forgets to remove it themselves.
func TestReconcileInventoryIfDue_ClearsRefreshAnnotationOnSuccess(t *testing.T) {
	bm := createMachine()
	bm.Annotations = map[string]string{"tinkerbell.org/refresh-inventory": trueString}
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{
		InventoryDevice: &common.Device{
			BIOS: &common.BIOS{Common: common.Common{Vendor: "Dell Inc."}},
		},
	}

	client := newInventoryTestClient(hw, bm)
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0, 0, true)
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

// TestReconcileInventoryIfDue_ProviderError verifies that a provider which
// implements InventoryGetter but fails does not error out to the caller and does
// not create a partial/empty status.attributes.outOfBand.
func TestReconcileInventoryIfDue_ProviderError(t *testing.T) {
	bm := createMachine()
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{
		ErrInventory: errors.New("inventory request failed"),
	}

	client := newInventoryTestClient(hw)
	recorder := events.NewFakeRecorder(2)
	reconciler := controller.NewMachineReconciler(client, recorder, newTestClient(provider), 0, 0, true)
	bmcClient := newOpenTestBMCClient(t, provider)

	// reconcileInventoryIfDue has no return value — failures are logged/evented,
	// never propagated. This call must not panic.
	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	var got tinkerbell.Hardware
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "test-namespace", Name: "test-hw"}, &got); err != nil {
		t.Fatalf("failed to get Hardware: %v", err)
	}
	if controller.OutOfBandAttributes(&got) != nil {
		t.Errorf("Status.Attributes.OutOfBand = %+v, want nil after a failed collection", controller.OutOfBandAttributes(&got))
	}
	if len(recorder.Events) != 1 {
		t.Errorf("got %d events, want 1 warning for a failed collection", len(recorder.Events))
	}
}

// noInventoryProvider wraps testProvider without promoting its Inventory
// method, mirroring bmclib providers (ipmitool, homeassistant) that do not
// implement InventoryGetter.
type noInventoryProvider struct {
	p *testProvider
}

func (n *noInventoryProvider) Open(ctx context.Context) error  { return n.p.Open(ctx) }
func (n *noInventoryProvider) Close(ctx context.Context) error { return n.p.Close(ctx) }
func (n *noInventoryProvider) PowerStateGet(ctx context.Context) (string, error) {
	return n.p.PowerStateGet(ctx)
}

// TestReconcileInventoryIfDue_Unsupported verifies that a BMC without any
// InventoryGetter provider emits no event and records the attempt, so it is not
// retried (and logged) on every reconcile.
func TestReconcileInventoryIfDue_Unsupported(t *testing.T) {
	bm := createMachine()
	hw := createHardwareForMachine(bm.Name)
	provider := &noInventoryProvider{p: &testProvider{}}

	reg := registrar.NewRegistry()
	reg.Register("noinventory", "ipmi", registrar.Features{providers.FeaturePowerState}, nil, provider)
	bmcClient := bmclib.NewClient("0.0.0.0", "user", "pass", bmclib.WithRegistry(reg))
	if err := bmcClient.Open(context.Background()); err != nil {
		t.Fatalf("failed to open test BMC client: %v", err)
	}

	client := newInventoryTestClient(hw)
	recorder := events.NewFakeRecorder(2)
	reconciler := controller.NewMachineReconciler(client, recorder, newTestClient(provider.p), 0, 0, true)
	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	if len(recorder.Events) != 0 {
		t.Errorf("got event %q, want none for a BMC that does not support inventory", <-recorder.Events)
	}
	var got tinkerbell.Hardware
	if err := client.Get(context.Background(), types.NamespacedName{Namespace: "test-namespace", Name: "test-hw"}, &got); err != nil {
		t.Fatalf("failed to get Hardware: %v", err)
	}
	oob := controller.OutOfBandAttributes(&got)
	if oob == nil || oob.LastUpdated == nil {
		t.Fatalf("Status.Attributes.OutOfBand = %+v, want LastUpdated set so the next attempt waits a refresh interval", oob)
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
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(4), newTestClient(provider), 0, 0, true)
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
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0, 0, true)
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
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0, 0, true)
	bmcClient := newOpenTestBMCClient(t, provider)

	// Must not panic despite the ambiguous match.
	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	if provider.InventoryCalls != 0 {
		t.Errorf("Inventory() called %d times with an ambiguous Hardware link, want 0", provider.InventoryCalls)
	}
}

// TestReconcileInventoryIfDue_CollectionDisabledFleetWide verifies that the
// fleet-wide inventoryCollectionEnabled=false kill switch skips inventory
// collection entirely, even for a Machine/Hardware pair that would otherwise
// be due.
func TestReconcileInventoryIfDue_CollectionDisabledFleetWide(t *testing.T) {
	bm := createMachine()
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{
		InventoryDevice: &common.Device{
			BIOS: &common.BIOS{Common: common.Common{Vendor: "Dell Inc."}},
		},
	}

	client := newInventoryTestClient(hw)
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0, 0, false)
	bmcClient := newOpenTestBMCClient(t, provider)

	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	if provider.InventoryCalls != 0 {
		t.Errorf("Inventory() called %d times with fleet-wide collection disabled, want 0", provider.InventoryCalls)
	}
}

// TestReconcileInventoryIfDue_DisabledForSpecificHardware verifies that a
// Hardware object annotated with tinkerbell.org/disable-outofband-inventory
// opts out of collection on its own, without affecting the fleet-wide
// inventoryCollectionEnabled setting.
func TestReconcileInventoryIfDue_DisabledForSpecificHardware(t *testing.T) {
	bm := createMachine()
	hw := createHardwareForMachine(bm.Name)
	hw.Annotations = map[string]string{"tinkerbell.org/disable-outofband-inventory": trueString}
	provider := &testProvider{
		InventoryDevice: &common.Device{
			BIOS: &common.BIOS{Common: common.Common{Vendor: "Dell Inc."}},
		},
	}

	client := newInventoryTestClient(hw)
	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0, 0, true)
	bmcClient := newOpenTestBMCClient(t, provider)

	reconciler.ReconcileInventoryIfDueForTest(context.Background(), logr.Discard(), bmcClient, bm)

	if provider.InventoryCalls != 0 {
		t.Errorf("Inventory() called %d times for a Hardware with disable-outofband-inventory annotation, want 0", provider.InventoryCalls)
	}
}

// TestMachineReconcile_WithHardwareLinked is an end-to-end smoke test through
// the real Reconcile() entry point, confirming the inventory step doesn't break
// normal Machine reconciliation when a linked Hardware object exists (using
// newClientBuilder(), the same fixture used by every other Machine test in this
// package — this deliberately does not assert on attribute content; that's
// covered directly above).
func TestMachineReconcile_WithHardwareLinked(t *testing.T) {
	bm := createMachine()
	hw := createHardwareForMachine(bm.Name)
	provider := &testProvider{Powerstate: "on"}

	client := newClientBuilder().
		WithObjects(bm, hw, createSecret()).
		Build()

	reconciler := controller.NewMachineReconciler(client, events.NewFakeRecorder(2), newTestClient(provider), 0, 0, true)
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "test-namespace", Name: "test-bm"}}

	if _, err := reconciler.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

// hardwareLinkedTo returns a Hardware linked to ref ("" for no bmcRef) with the
// inventory opt-out annotation set to optOut ("" for none).
func hardwareLinkedTo(ref, optOut string) *tinkerbell.Hardware {
	hw := createHardwareForMachine(ref)
	if ref == "" {
		hw.Spec.BMCRef = nil
	}
	if optOut != "" {
		hw.Annotations = map[string]string{"tinkerbell.org/disable-outofband-inventory": optOut}
	}
	return hw
}

func TestHardwareBecameInventoryEligible_Create(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		hw   *tinkerbell.Hardware
		want bool
	}{
		"bmcRef set":                {hw: hardwareLinkedTo("bmc-1", ""), want: true},
		"no bmcRef":                 {hw: hardwareLinkedTo("", ""), want: false},
		"bmcRef empty name":         {hw: createHardwareForMachine(""), want: false},
		"bmcRef set but opted out":  {hw: hardwareLinkedTo("bmc-1", trueString), want: false},
		"bmcRef set, opt-out false": {hw: hardwareLinkedTo("bmc-1", "false"), want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := controller.HardwareBecameInventoryEligible().Create(event.CreateEvent{Object: tt.hw}); got != tt.want {
				t.Errorf("Create() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHardwareBecameInventoryEligible_Update(t *testing.T) {
	t.Parallel()

	withOutOfBand := hardwareLinkedTo("bmc-1", "")
	withOutOfBand.Status.Attributes = &tinkerbell.HardwareAttributes{
		OutOfBand: &tinkerbell.Attributes{LastUpdated: &metav1.Time{}},
	}
	withAnnotation := hardwareLinkedTo("bmc-1", "")
	withAnnotation.Annotations = map[string]string{"example.com/note": "x"}

	tests := map[string]struct {
		old, new *tinkerbell.Hardware
		want     bool
	}{
		"bmcRef added":                             {old: hardwareLinkedTo("", ""), new: hardwareLinkedTo("bmc-1", ""), want: true},
		"bmcRef moved to another Machine":          {old: hardwareLinkedTo("bmc-1", ""), new: hardwareLinkedTo("bmc-2", ""), want: true},
		"bmcRef removed":                           {old: hardwareLinkedTo("bmc-1", ""), new: hardwareLinkedTo("", ""), want: false},
		"same bmcRef":                              {old: hardwareLinkedTo("bmc-1", ""), new: hardwareLinkedTo("bmc-1", ""), want: false},
		"status-only update":                       {old: hardwareLinkedTo("bmc-1", ""), new: withOutOfBand, want: false},
		"metadata-only update":                     {old: hardwareLinkedTo("bmc-1", ""), new: withAnnotation, want: false},
		"neither linked":                           {old: hardwareLinkedTo("", ""), new: hardwareLinkedTo("", ""), want: false},
		"inventory opt-out removed":                {old: hardwareLinkedTo("bmc-1", trueString), new: hardwareLinkedTo("bmc-1", ""), want: true},
		"inventory opt-out set to false":           {old: hardwareLinkedTo("bmc-1", trueString), new: hardwareLinkedTo("bmc-1", "false"), want: true},
		"inventory opt-out added":                  {old: hardwareLinkedTo("bmc-1", ""), new: hardwareLinkedTo("bmc-1", trueString), want: false},
		"inventory opt-out unchanged":              {old: hardwareLinkedTo("bmc-1", trueString), new: hardwareLinkedTo("bmc-1", trueString), want: false},
		"inventory opt-out removed while unlinked": {old: hardwareLinkedTo("", trueString), new: hardwareLinkedTo("", ""), want: false},
		"bmcRef added while opted out":             {old: hardwareLinkedTo("", trueString), new: hardwareLinkedTo("bmc-1", trueString), want: false},
		"bmcRef moved while opted out":             {old: hardwareLinkedTo("bmc-1", trueString), new: hardwareLinkedTo("bmc-2", trueString), want: false},
		"bmcRef added and opt-out removed at once": {old: hardwareLinkedTo("", trueString), new: hardwareLinkedTo("bmc-1", ""), want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := controller.HardwareBecameInventoryEligible().Update(event.UpdateEvent{ObjectOld: tt.old, ObjectNew: tt.new})
			if got != tt.want {
				t.Errorf("Update() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHardwareBecameInventoryEligible_DeleteAndGeneric(t *testing.T) {
	t.Parallel()

	hw := hardwareLinkedTo("bmc-1", "")
	if controller.HardwareBecameInventoryEligible().Delete(event.DeleteEvent{Object: hw}) {
		t.Error("Delete() = true, want false")
	}
	if controller.HardwareBecameInventoryEligible().Generic(event.GenericEvent{Object: hw}) {
		t.Error("Generic() = true, want false")
	}
}

func TestMachineRequestsForHardware(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		hw   *tinkerbell.Hardware
		want []reconcile.Request
	}{
		"linked Hardware enqueues its Machine in the Hardware's namespace": {
			hw:   hardwareLinkedTo("bmc-1", ""),
			want: []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "test-namespace", Name: "bmc-1"}}},
		},
		"unlinked Hardware enqueues nothing": {
			hw:   hardwareLinkedTo("", ""),
			want: nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := controller.MachineRequestsForHardware(t.Context(), tt.hw)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MachineRequestsForHardware() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestHardwareBMCRefIndexFunc(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		obj  ctrlclient.Object
		want []string
	}{
		"linked Hardware is indexed by its Machine name":    {obj: hardwareLinkedTo("bmc-1", ""), want: []string{"bmc-1"}},
		"Hardware without bmcRef is not indexed":            {obj: hardwareLinkedTo("", ""), want: nil},
		"Hardware with an empty bmcRef name is not indexed": {obj: createHardwareForMachine(""), want: nil},
		"non-Hardware object is not indexed":                {obj: &bmc.Machine{}, want: nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := controller.HardwareBMCRefIndexFunc(tt.obj)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("HardwareBMCRefIndexFunc() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
