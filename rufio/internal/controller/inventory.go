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
	"cmp"
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/avast/retry-go/v4"
	bmclib "github.com/bmc-toolbox/bmclib/v2"
	common "github.com/bmc-toolbox/common"
	"github.com/ccoveille/go-safecast/v2"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// inventoryRefreshInterval is how often BMC inventory is refreshed per Machine.
// Inventory collection is slow (5-30s on Redfish), so it deliberately does not run
// on every power-poll reconcile (every machineRequeueInterval/powerCheckInterval) —
// only once this interval has elapsed since the last successful collection.
const inventoryRefreshInterval = 24 * time.Hour

// refreshInventoryAnnotation, when set to "true" on a Machine, forces an immediate
// inventory refresh regardless of inventoryRefreshInterval.
const refreshInventoryAnnotation = "tinkerbell.org/refresh-inventory"

// hardwareBMCRefIndexKey indexes Hardware objects by the name of the Machine
// their spec.bmcRef points at, so the Machine controller can find its linked
// Hardware without a full List+filter.
const hardwareBMCRefIndexKey = ".spec.bmcRef.name"

// hardwareBMCRefIndexFunc is the IndexerFunc for hardwareBMCRefIndexKey.
func hardwareBMCRefIndexFunc(obj ctrlclient.Object) []string {
	hw, ok := obj.(*tinkerbell.Hardware)
	if !ok || hw.Spec.BMCRef == nil {
		return nil
	}
	return []string{hw.Spec.BMCRef.Name}
}

// dueForInventoryRefresh returns true if inventory has never been collected, is
// stale, or a manual refresh was explicitly requested via refreshInventoryAnnotation.
func dueForInventoryRefresh(hw *tinkerbell.Hardware, bm *bmc.Machine) bool {
	if bm.Annotations[refreshInventoryAnnotation] == "true" {
		return true
	}
	inv := outOfBandAttributes(hw)
	if inv == nil || inv.LastUpdated == nil {
		return true
	}
	return time.Since(inv.LastUpdated.Time) > inventoryRefreshInterval
}

// outOfBandAttributes returns the out-of-band attributes subtree, or nil if it has
// never been populated. Both intermediate levels are optional pointers.
func outOfBandAttributes(hw *tinkerbell.Hardware) *tinkerbell.Attributes {
	if hw == nil || hw.Status.Attributes == nil {
		return nil
	}
	return hw.Status.Attributes.OutOfBand
}

// clearRefreshInventoryAnnotation removes refreshInventoryAnnotation from the
// Machine after a successful inventory collection, so a manually-requested
// refresh doesn't keep forcing collection on every reconcile if the caller
// forgets to remove the annotation themselves.
func (r *MachineReconciler) clearRefreshInventoryAnnotation(ctx context.Context, logger logr.Logger, bm *bmc.Machine) {
	if bm.Annotations[refreshInventoryAnnotation] != "true" {
		return
	}
	patch := ctrlclient.MergeFrom(bm.DeepCopy())
	delete(bm.Annotations, refreshInventoryAnnotation)
	// Retried: dueForInventoryRefresh treats refreshInventoryAnnotation as an
	// unconditional override, so an un-retried, transiently-failed Patch here
	// would leave it set on the server and force a full (5-30s) BMC inventory
	// collection on every subsequent reconcile — at the powerCheckInterval
	// cadence, not inventoryRefreshInterval — until some later attempt
	// happens to succeed.
	if err := retry.Do(func() error {
		return r.client.Patch(ctx, bm, patch)
	}, retry.Attempts(3), retry.Delay(500*time.Millisecond), retry.Context(ctx)); err != nil {
		logger.Error(err, "failed to clear refresh-inventory annotation after successful collection")
		r.recorder.Eventf(bm, nil, corev1.EventTypeWarning, "ClearRefreshAnnotationFailed", "ClearRefreshInventoryAnnotation", "clear refresh-inventory annotation: %v", err)
	}
}

// findLinkedHardware returns the Hardware object whose spec.bmcRef points at the
// given Machine, if any. Returns (nil, nil) if no Hardware references this Machine.
func (r *MachineReconciler) findLinkedHardware(ctx context.Context, bm *bmc.Machine) (*tinkerbell.Hardware, error) {
	var hwList tinkerbell.HardwareList
	if err := r.client.List(ctx, &hwList,
		ctrlclient.InNamespace(bm.Namespace),
		ctrlclient.MatchingFields{hardwareBMCRefIndexKey: bm.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list Hardware for Machine %s/%s: %w", bm.Namespace, bm.Name, err)
	}
	if len(hwList.Items) == 0 {
		return nil, nil
	}
	if len(hwList.Items) > 1 {
		return nil, fmt.Errorf("ambiguous Hardware link for Machine %s/%s: %d Hardware objects reference it via spec.bmcRef, want at most 1", bm.Namespace, bm.Name, len(hwList.Items))
	}
	return &hwList.Items[0], nil
}

// reconcileInventory collects BMC inventory using the already-open bmcClient
// (reused from the power-polling step in doReconcile — no second BMC connection is
// opened) and writes it to the linked Hardware's status.
func (r *MachineReconciler) reconcileInventory(ctx context.Context, bmcClient *bmclib.Client, hw *tinkerbell.Hardware) error {
	device, err := bmcClient.Inventory(ctx)
	if err != nil {
		return fmt.Errorf("get BMC inventory: %w", err)
	}
	sortDevice(device)

	// bmcClient.GetMetadata().SuccessfulProvider tells us which bmclib driver
	// actually produced this inventory (e.g. "redfish", "dell", "asrockrack") —
	// already public API, no upstream bmclib change needed.
	return r.applyOutOfBandAttributes(ctx, hw, device, bmcClient.GetMetadata().SuccessfulProvider)
}

// applyOutOfBandAttributes patches Hardware.status.attributes.outOfBand via
// Server-Side Apply under a dedicated field manager ("machine-controller"), so a
// future sibling subtree written by another controller stays a disjoint path and
// the two writers cannot conflict.
func (r *MachineReconciler) applyOutOfBandAttributes(ctx context.Context, hw *tinkerbell.Hardware, device *common.Device, collectionMethod string) error {
	now := metav1.Now()
	newInventory := attributesFromDevice(device, collectionMethod, &now)

	if newInventory == nil {
		// A provider that returns (nil, nil) from Inventory() has nothing new
		// to report, but the collection attempt itself completed. Carry
		// forward any previously-collected attributes untouched (no data
		// loss) except for LastUpdated, which must still advance to "now":
		// otherwise dueForInventoryRefresh treats this Hardware as never
		// collected forever and retries the slow (5-30s) BMC round-trip on
		// every subsequent reconcile instead of respecting
		// inventoryRefreshInterval. This intentionally bypasses the
		// idempotency guard below, since the write here exists purely to
		// advance the timestamp, not to reflect a content change.
		existing := outOfBandAttributes(hw)
		if existing == nil {
			newInventory = &tinkerbell.Attributes{LastUpdated: &now, CollectionMethod: collectionMethod}
		} else {
			newInventory = existing.DeepCopy()
			newInventory.LastUpdated = &now
			newInventory.CollectionMethod = collectionMethod
		}
		return r.applyHardwareOutOfBand(ctx, hw, newInventory)
	}

	// Idempotency guard: compare everything except LastUpdated (which always
	// differs) and skip the write if nothing actually changed. Combined with
	// sortDevice above, this avoids hot-looping the reconciler when a BMC
	// returns the same logical inventory in a different list order.
	existing := outOfBandAttributes(hw).DeepCopy()
	if existing != nil {
		existing.LastUpdated = nil
	}
	newComparable := newInventory.DeepCopy()
	newComparable.LastUpdated = nil
	if equality.Semantic.DeepEqual(existing, newComparable) {
		return nil
	}

	return r.applyHardwareOutOfBand(ctx, hw, newInventory)
}

// applyHardwareOutOfBand issues the Server-Side Apply patch for
// status.attributes.outOfBand and updates hw in place to match, so callers
// observing hw afterward (e.g. within the same reconcile) see the new value
// without needing a fresh Get.
func (r *MachineReconciler) applyHardwareOutOfBand(ctx context.Context, hw *tinkerbell.Hardware, newInventory *tinkerbell.Attributes) error {
	apiVersion := tinkerbell.GroupVersion.String()
	kind := "Hardware"
	name := hw.Name
	namespace := hw.Namespace
	applyConfig := &hardwareStatusApplyConfiguration{
		Kind:       &kind,
		APIVersion: &apiVersion,
		Metadata: hardwareApplyMetadata{
			Name:      &name,
			Namespace: &namespace,
		},
		Status: &hardwareStatusApplyConfigurationStatus{
			Attributes: &hardwareAttributesApplyConfiguration{
				OutOfBand: newInventory,
			},
		},
	}
	if err := r.client.Status().Apply(ctx, applyConfig,
		ctrlclient.FieldOwner("machine-controller"),
		ctrlclient.ForceOwnership,
	); err != nil {
		return fmt.Errorf("failed to apply Hardware %s/%s status.attributes.outOfBand: %w", hw.Namespace, hw.Name, err)
	}
	if hw.Status.Attributes == nil {
		hw.Status.Attributes = &tinkerbell.HardwareAttributes{}
	}
	hw.Status.Attributes.OutOfBand = newInventory

	return nil
}

// hardwareStatusApplyConfiguration is a minimal hand-written implementation of
// runtime.ApplyConfiguration for Hardware, sufficient for the Server-Side Apply
// patch to status.attributes.outOfBand above. controller-runtime's client.Apply /
// SubResourceWriter.Apply want an object satisfying an internal
// "applyConfiguration" interface (GetName/GetNamespace/GetKind/GetAPIVersion,
// all returning *string) — the shape client-gen's apply-configuration-gen
// produces for built-in types (e.g. corev1.SecretApplyConfiguration). This repo
// has no such codegen wired up for its custom CRDs, and generating a full one
// for Hardware is out of scope for the single field this controller writes, so
// this is hand-written instead.
type hardwareStatusApplyConfiguration struct {
	Kind       *string                                 `json:"kind,omitempty"`
	APIVersion *string                                 `json:"apiVersion,omitempty"`
	Metadata   hardwareApplyMetadata                   `json:"metadata,omitempty"`
	Status     *hardwareStatusApplyConfigurationStatus `json:"status,omitempty"`
}

type hardwareApplyMetadata struct {
	Name      *string `json:"name,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
}

// hardwareStatusApplyConfigurationStatus only carries the one path this controller
// owns. status.attributes is modeled one level deeper than the leaf so the applied
// patch names only outOfBand: SSA merges maps by key, so a sibling subtree added
// later stays owned by its own writer rather than being pruned by this one.
type hardwareStatusApplyConfigurationStatus struct {
	Attributes *hardwareAttributesApplyConfiguration `json:"attributes,omitempty"`
}

// hardwareAttributesApplyConfiguration carries only the out-of-band subtree.
// OutOfBand is the concrete API type (not a further apply-configuration wrapper):
// this controller always applies the whole sub-object atomically, never a partial
// deep-merge within it, so the extra per-field apply modeling generated types use
// isn't needed here.
type hardwareAttributesApplyConfiguration struct {
	OutOfBand *tinkerbell.Attributes `json:"outOfBand,omitempty"`
}

func (h *hardwareStatusApplyConfiguration) IsApplyConfiguration()  {}
func (h *hardwareStatusApplyConfiguration) GetKind() *string       { return h.Kind }
func (h *hardwareStatusApplyConfiguration) GetAPIVersion() *string { return h.APIVersion }
func (h *hardwareStatusApplyConfiguration) GetName() *string       { return h.Metadata.Name }
func (h *hardwareStatusApplyConfiguration) GetNamespace() *string  { return h.Metadata.Namespace }

// sortDevice sorts every slice field on device by a stable per-component key.
// BMCs return list fields (NICs, drives, CPUs, memory, PSUs, etc.) in
// non-deterministic order across polls; without this, a naive status write would
// produce a spurious diff every reconcile even when nothing physically changed.
func sortDevice(d *common.Device) {
	if d == nil {
		return
	}
	sortByKeyThenContent(d.CPUs, cpuKey)
	sortByKeyThenContent(d.Memory, memoryKey)
	sortByKeyThenContent(d.NICs, nicKey)
	for _, n := range d.NICs {
		if n == nil {
			continue
		}
		sortByKeyThenContent(n.NICPorts, nicPortKey)
	}
	sortByKeyThenContent(d.Drives, driveKey)
	sortByKeyThenContent(d.StorageControllers, storageControllerKey)
	sortByKeyThenContent(d.PSUs, psuKey)
	sortByKeyThenContent(d.TPMs, tpmKey)
	sortByKeyThenContent(d.GPUs, gpuKey)
	if d.BMC != nil && d.BMC.NIC != nil {
		sortByKeyThenContent(d.BMC.NIC.NICPorts, nicPortKey)
	}
}

// sortByKeyThenContent sorts s by key, breaking ties on full content (%+v)
// rather than leaving them in whatever order the caller passed in. key()
// commonly returns "" for components bmclib reports without a serial/ID (see
// the key funcs below), and plain sort.Slice has no guaranteed, deterministic
// behavior for elements it considers equal — so two polls returning the same
// logical components in a different order would otherwise not converge on a
// single canonical order, causing sortDevice to fail at the one thing it
// exists for. Ordering fully-identical elements this way is a no-op in
// practice: two elements with identical content are indistinguishable in the
// applied status regardless of which position each ends up in.
func sortByKeyThenContent[T any](s []T, key func(T) string) {
	sort.Slice(s, func(i, j int) bool {
		if ki, kj := key(s[i]), key(s[j]); ki != kj {
			return ki < kj
		}
		return fmt.Sprintf("%+v", s[i]) < fmt.Sprintf("%+v", s[j])
	})
}

func cpuKey(c *common.CPU) string {
	if c == nil {
		return ""
	}
	return cmp.Or(c.ID, c.Slot, c.Serial)
}

func memoryKey(m *common.Memory) string {
	if m == nil {
		return ""
	}
	return cmp.Or(m.ID, m.Slot, m.Serial)
}

func nicKey(n *common.NIC) string {
	if n == nil {
		return ""
	}
	return cmp.Or(n.ID, n.Serial)
}

func nicPortKey(p *common.NICPort) string {
	if p == nil {
		return ""
	}
	return cmp.Or(p.PhysicalID, p.ID, p.MacAddress)
}

func driveKey(dr *common.Drive) string {
	if dr == nil {
		return ""
	}
	return cmp.Or(dr.ID, dr.Serial, dr.WWN)
}

func storageControllerKey(sc *common.StorageController) string {
	if sc == nil {
		return ""
	}
	return cmp.Or(sc.ID, sc.Serial)
}

func psuKey(p *common.PSU) string {
	if p == nil {
		return ""
	}
	return cmp.Or(p.ID, p.Serial)
}

func tpmKey(t *common.TPM) string {
	if t == nil {
		return ""
	}
	return t.Serial
}

func gpuKey(g *common.GPU) string {
	if g == nil {
		return ""
	}
	return cmp.Or(g.Serial, g.Model, g.ProductName)
}

// attributesFromDevice maps a bmclib/common.Device onto the shared Tinkerbell
// Attributes type, for storage under status.attributes.outOfBand. Every field on
// both sides is optional: BMC vendors/protocols vary widely in what they report,
// so a missing field here reflects what the BMC/driver reports, not a mapping bug.
// Fields on Attributes that only the in-band collector can fill (PCIDevices,
// Memory.UsableBytes, per-port EnabledCapabilities, OS-visible names) are left
// unset here by design.
func attributesFromDevice(device *common.Device, collectionMethod string, t *metav1.Time) *tinkerbell.Attributes {
	if device == nil {
		return nil
	}

	attrs := &tinkerbell.Attributes{
		LastUpdated:      t,
		CollectionMethod: collectionMethod,
		Product:          productFromCommon(device.Common),
		BIOS:             biosFromCommon(device.BIOS),
		BMC:              bmcFromCommon(device.BMC),
		Baseboard:        baseboardFromMainboard(device.Mainboard),
	}

	var sockets []tinkerbell.CPUSocket
	for _, c := range device.CPUs {
		if c == nil {
			continue
		}
		cores, _ := safecast.Convert[uint32](c.Cores)
		threads, _ := safecast.Convert[uint32](c.Threads)
		clockSpeedMHz, _ := safecast.Convert[uint32](c.ClockSpeedHz / 1_000_000)
		sockets = append(sockets, tinkerbell.CPUSocket{
			Slot:            c.Slot,
			Vendor:          c.Vendor,
			Model:           c.Model,
			Cores:           cores,
			Threads:         threads,
			ClockSpeedMHz:   clockSpeedMHz,
			SerialNumber:    c.Serial,
			FirmwareVersion: firmwareVersion(c.Firmware),
		})
	}
	if len(sockets) > 0 {
		// TotalCores/TotalThreads are deliberately left unset: they are the OS's
		// view of the machine and are reported by the in-band collector. Summing
		// the sockets here would look authoritative while silently disagreeing
		// with the in-band totals on any machine with a disabled core or SMT off.
		attrs.CPU = &tinkerbell.CPU{Sockets: sockets}
	}

	var modules []tinkerbell.MemoryModule
	for _, m := range device.Memory {
		if m == nil {
			continue
		}
		speedMHz, _ := safecast.Convert[uint32](m.ClockSpeedHz / 1_000_000)
		modules = append(modules, tinkerbell.MemoryModule{
			Slot:            m.Slot,
			Vendor:          m.Vendor,
			Model:           m.Model,
			SerialNumber:    m.Serial,
			PartNumber:      m.PartNumber,
			SizeBytes:       m.SizeBytes,
			SpeedMHz:        speedMHz,
			FormFactor:      m.FormFactor,
			FirmwareVersion: firmwareVersion(m.Firmware),
		})
	}
	if len(modules) > 0 {
		// TotalBytes/UsableBytes left unset for the same reason as CPU totals.
		attrs.Memory = &tinkerbell.Memory{Modules: modules}
	}

	for _, n := range device.NICs {
		if nic := networkInterfaceFromCommon(n); nic != nil {
			attrs.NetworkInterfaces = append(attrs.NetworkInterfaces, *nic)
		}
	}

	for _, d := range device.Drives {
		if d == nil {
			continue
		}
		attrs.BlockDevices = append(attrs.BlockDevices, tinkerbell.BlockDevice{
			Vendor:          d.Vendor,
			Model:           d.Model,
			SerialNumber:    d.Serial,
			WWN:             d.WWN,
			SizeBytes:       d.CapacityBytes,
			DriveType:       d.Type,
			SmartStatus:     d.SmartStatus,
			FirmwareVersion: firmwareVersion(d.Firmware),
			Status:          statusFromCommon(d.Status),
		})
	}

	for _, sc := range device.StorageControllers {
		if sc == nil {
			continue
		}
		attrs.StorageControllers = append(attrs.StorageControllers, tinkerbell.StorageController{
			Vendor:          sc.Vendor,
			Model:           sc.Model,
			SerialNumber:    sc.Serial,
			Description:     sc.Description,
			FirmwareVersion: firmwareVersion(sc.Firmware),
			Status:          statusFromCommon(sc.Status),
		})
	}

	for _, p := range device.PSUs {
		if p == nil {
			continue
		}
		attrs.PSUs = append(attrs.PSUs, tinkerbell.PSU{
			Vendor:             p.Vendor,
			Model:              p.Model,
			SerialNumber:       p.Serial,
			Description:        p.Description,
			FirmwareVersion:    firmwareVersion(p.Firmware),
			PowerCapacityWatts: p.PowerCapacityWatts,
			Status:             statusFromCommon(p.Status),
		})
	}

	for _, tpm := range device.TPMs {
		if tpm == nil {
			continue
		}
		attrs.TPMs = append(attrs.TPMs, tinkerbell.TPM{
			Vendor:          tpm.Vendor,
			Model:           tpm.Model,
			SerialNumber:    tpm.Serial,
			InterfaceType:   tpm.InterfaceType,
			FirmwareVersion: firmwareVersion(tpm.Firmware),
			Status:          statusFromCommon(tpm.Status),
		})
	}

	for _, g := range device.GPUs {
		if g == nil {
			continue
		}
		attrs.GPUDevices = append(attrs.GPUDevices, tinkerbell.GPUDevice{
			Vendor:          g.Vendor,
			Model:           g.Model,
			SerialNumber:    g.Serial,
			Description:     g.Description,
			FirmwareVersion: firmwareVersion(g.Firmware),
			Status:          statusFromCommon(g.Status),
		})
	}

	return attrs
}

// productFromCommon maps the top-level Device.Common fields — the machine's own
// identity (e.g. the Redfish "System" resource) — separate from any individual
// component like the Baseboard or BMC. POST code lives on the device-level status
// in bmclib (only the asrockrack driver populates it), so it is mapped here rather
// than onto BIOS.
func productFromCommon(c common.Common) *tinkerbell.Product {
	status := postStatusFromCommon(c.Status)
	if c.Vendor == "" && c.Model == "" && c.ProductName == "" && c.Serial == "" && status == nil {
		return nil
	}
	return &tinkerbell.Product{
		Name:         c.ProductName,
		Vendor:       c.Vendor,
		Model:        c.Model,
		SerialNumber: c.Serial,
		Status:       status,
	}
}

func firmwareVersion(f *common.Firmware) string {
	if f == nil {
		return ""
	}
	return f.Installed
}

// statusFromCommon maps component health/state. PostCode is not mapped here: it is
// a device-level POST diagnostic, not a per-component field, and mapping it would
// emit a meaningless postCode on every component.
func statusFromCommon(s *common.Status) *tinkerbell.ComponentStatus {
	if s == nil {
		return nil
	}
	return &tinkerbell.ComponentStatus{
		Health: s.Health,
		State:  s.State,
	}
}

// postStatusFromCommon is statusFromCommon plus the POST diagnostics, for the
// device-level status only. PostCode is a pointer so that 0 — a successful POST —
// survives serialization instead of being dropped by omitempty.
func postStatusFromCommon(s *common.Status) *tinkerbell.ComponentStatus {
	cs := statusFromCommon(s)
	if cs == nil {
		return nil
	}
	if s.PostCodeStatus != "" {
		postCode, _ := safecast.Convert[int32](s.PostCode)
		cs.PostCode = &postCode
		cs.PostCodeStatus = s.PostCodeStatus
	}
	return cs
}

func biosFromCommon(b *common.BIOS) *tinkerbell.BIOS {
	if b == nil {
		return nil
	}
	return &tinkerbell.BIOS{
		Vendor:          b.Vendor,
		Model:           b.Model,
		SerialNumber:    b.Serial,
		FirmwareVersion: firmwareVersion(b.Firmware),
		Status:          statusFromCommon(b.Status),
	}
}

func bmcFromCommon(bmcComp *common.BMC) *tinkerbell.BMC {
	if bmcComp == nil {
		return nil
	}
	return &tinkerbell.BMC{
		Vendor:          bmcComp.Vendor,
		Model:           bmcComp.Model,
		SerialNumber:    bmcComp.Serial,
		FirmwareVersion: firmwareVersion(bmcComp.Firmware),
		NIC:             networkInterfaceFromCommon(bmcComp.NIC),
		Status:          statusFromCommon(bmcComp.Status),
	}
}

// networkInterfaceFromCommon converts a bmclib NIC (used for both the host's NICs
// and the BMC's own out-of-band management NIC) into its Tinkerbell API
// representation.
func networkInterfaceFromCommon(n *common.NIC) *tinkerbell.NetworkInterface {
	if n == nil {
		return nil
	}
	nic := &tinkerbell.NetworkInterface{
		Vendor:          n.Vendor,
		Model:           n.Model,
		SerialNumber:    n.Serial,
		FirmwareVersion: firmwareVersion(n.Firmware),
	}
	for _, p := range n.NICPorts {
		if p == nil {
			continue
		}
		speedMbps, _ := safecast.Convert[uint32](p.SpeedBits / 1_000_000)
		mtu, _ := safecast.Convert[uint32](p.MTUSize)
		nic.Ports = append(nic.Ports, tinkerbell.NetworkPort{
			PortID:     cmp.Or(p.PhysicalID, p.ID),
			MAC:        p.MacAddress,
			SpeedMbps:  speedMbps,
			MTU:        mtu,
			LinkStatus: p.LinkStatus,
		})
	}
	return nic
}

func baseboardFromMainboard(m *common.Mainboard) *tinkerbell.Baseboard {
	if m == nil {
		return nil
	}
	return &tinkerbell.Baseboard{
		Vendor:          m.Vendor,
		Model:           m.Model,
		SerialNumber:    m.Serial,
		Description:     m.Description,
		FirmwareVersion: firmwareVersion(m.Firmware),
		Status:          statusFromCommon(m.Status),
	}
}
