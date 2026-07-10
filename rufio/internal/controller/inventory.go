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
	"sort"
	"time"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	common "github.com/bmc-toolbox/common"
	"github.com/ccoveille/go-safecast/v2"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
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
	inv := hw.Status.BMCInventory
	if inv == nil || inv.LastUpdated == nil {
		return true
	}
	return time.Since(inv.LastUpdated.Time) > inventoryRefreshInterval
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
	if err := r.client.Patch(ctx, bm, patch); err != nil {
		logger.Error(err, "failed to clear refresh-inventory annotation after successful collection")
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
	return r.applyBMCInventory(ctx, hw, device, bmcClient.GetMetadata().SuccessfulProvider)
}

// applyBMCInventory patches Hardware.status.bmcInventory via Server-Side Apply
// under a dedicated field manager ("machine-controller"), so it can be updated
// independently of status.agentInventory (written by tink-server under its own
// field manager).
func (r *MachineReconciler) applyBMCInventory(ctx context.Context, hw *tinkerbell.Hardware, device *common.Device, collectionMethod string) error {
	now := metav1.Now()
	newInventory := bmcInventoryFromDevice(device, collectionMethod, &now)

	// Idempotency guard: compare everything except LastUpdated (which always
	// differs) and skip the write if nothing actually changed. Combined with
	// sortDevice above, this avoids hot-looping the reconciler when a BMC
	// returns the same logical inventory in a different list order.
	existing := hw.Status.BMCInventory.DeepCopy()
	if existing != nil {
		existing.LastUpdated = nil
	}
	newComparable := newInventory.DeepCopy()
	if newComparable != nil {
		newComparable.LastUpdated = nil
	}
	if equality.Semantic.DeepEqual(existing, newComparable) {
		return nil
	}

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
			BMCInventory: newInventory,
		},
	}
	if err := r.client.Status().Apply(ctx, applyConfig,
		ctrlclient.FieldOwner("machine-controller"),
		ctrlclient.ForceOwnership,
	); err != nil {
		return fmt.Errorf("failed to apply Hardware %s/%s status.bmcInventory: %w", hw.Namespace, hw.Name, err)
	}
	hw.Status.BMCInventory = newInventory

	return nil
}

// hardwareStatusApplyConfiguration is a minimal hand-written implementation of
// runtime.ApplyConfiguration for Hardware, sufficient for the Server-Side Apply
// patch to status.bmcInventory above. controller-runtime's client.Apply /
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

// hardwareStatusApplyConfigurationStatus only carries the one field this
// controller owns. BMCInventory is the concrete API type (not a further
// apply-configuration wrapper): this controller always applies the whole
// sub-object atomically, never a partial deep-merge within it, so the extra
// per-field apply modeling generated types use isn't needed here.
type hardwareStatusApplyConfigurationStatus struct {
	BMCInventory *tinkerbell.BMCInventory `json:"bmcInventory,omitempty"`
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
	sort.Slice(d.CPUs, func(i, j int) bool { return cpuKey(d.CPUs[i]) < cpuKey(d.CPUs[j]) })
	sort.Slice(d.Memory, func(i, j int) bool { return memoryKey(d.Memory[i]) < memoryKey(d.Memory[j]) })
	sort.Slice(d.NICs, func(i, j int) bool { return nicKey(d.NICs[i]) < nicKey(d.NICs[j]) })
	for _, n := range d.NICs {
		if n == nil {
			continue
		}
		sort.Slice(n.NICPorts, func(i, j int) bool { return nicPortKey(n.NICPorts[i]) < nicPortKey(n.NICPorts[j]) })
	}
	sort.Slice(d.Drives, func(i, j int) bool { return driveKey(d.Drives[i]) < driveKey(d.Drives[j]) })
	sort.Slice(d.StorageControllers, func(i, j int) bool {
		return storageControllerKey(d.StorageControllers[i]) < storageControllerKey(d.StorageControllers[j])
	})
	sort.Slice(d.PSUs, func(i, j int) bool { return psuKey(d.PSUs[i]) < psuKey(d.PSUs[j]) })
	sort.Slice(d.TPMs, func(i, j int) bool { return tpmKey(d.TPMs[i]) < tpmKey(d.TPMs[j]) })
	sort.Slice(d.GPUs, func(i, j int) bool { return gpuKey(d.GPUs[i]) < gpuKey(d.GPUs[j]) })
	if d.BMC != nil && d.BMC.NIC != nil {
		sort.Slice(d.BMC.NIC.NICPorts, func(i, j int) bool {
			return nicPortKey(d.BMC.NIC.NICPorts[i]) < nicPortKey(d.BMC.NIC.NICPorts[j])
		})
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func cpuKey(c *common.CPU) string {
	if c == nil {
		return ""
	}
	return firstNonEmpty(c.ID, c.Slot, c.Serial)
}

func memoryKey(m *common.Memory) string {
	if m == nil {
		return ""
	}
	return firstNonEmpty(m.ID, m.Slot, m.Serial)
}

func nicKey(n *common.NIC) string {
	if n == nil {
		return ""
	}
	return firstNonEmpty(n.ID, n.Serial)
}

func nicPortKey(p *common.NICPort) string {
	if p == nil {
		return ""
	}
	return firstNonEmpty(p.PhysicalID, p.ID, p.MacAddress)
}

func driveKey(dr *common.Drive) string {
	if dr == nil {
		return ""
	}
	return firstNonEmpty(dr.ID, dr.Serial, dr.WWN)
}

func storageControllerKey(sc *common.StorageController) string {
	if sc == nil {
		return ""
	}
	return firstNonEmpty(sc.ID, sc.Serial)
}

func psuKey(p *common.PSU) string {
	if p == nil {
		return ""
	}
	return firstNonEmpty(p.ID, p.Serial)
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
	return firstNonEmpty(g.Serial, g.Model, g.ProductName)
}

// bmcInventoryFromDevice maps a bmclib/common.Device onto the Tinkerbell
// BMCInventory status type. Every field on both sides is optional: BMC
// vendors/protocols vary widely in what they report, so a missing field here
// reflects what the BMC/driver reports, not a mapping bug.
func bmcInventoryFromDevice(device *common.Device, collectionMethod string, t *metav1.Time) *tinkerbell.BMCInventory {
	if device == nil {
		return nil
	}

	inv := &tinkerbell.BMCInventory{
		LastUpdated:      t,
		CollectionMethod: collectionMethod,
		Product:          productFromCommon(device.Common),
		BIOS:             firmwareComponentFromBIOS(device.BIOS),
		BMC:              firmwareComponentFromBMC(device.BMC),
		Mainboard:        componentFromMainboard(device.Mainboard),
	}

	for _, c := range device.CPUs {
		if c == nil {
			continue
		}
		cores, _ := safecast.Convert[uint32](c.Cores)
		threads, _ := safecast.Convert[uint32](c.Threads)
		clockSpeedMHz, _ := safecast.Convert[uint32](c.ClockSpeedHz / 1_000_000)
		inv.CPUs = append(inv.CPUs, tinkerbell.BMCCPUComponent{
			Vendor:            c.Vendor,
			Model:             c.Model,
			SerialNumber:      c.Serial,
			Slot:              c.Slot,
			Cores:             cores,
			Threads:           threads,
			ClockSpeedMHz:     clockSpeedMHz,
			FirmwareInstalled: firmwareInstalled(c.Firmware),
		})
	}

	for _, m := range device.Memory {
		if m == nil {
			continue
		}
		speedMHz, _ := safecast.Convert[uint32](m.ClockSpeedHz / 1_000_000)
		inv.Memory = append(inv.Memory, tinkerbell.BMCMemoryComponent{
			Vendor:            m.Vendor,
			Model:             m.Model,
			SerialNumber:      m.Serial,
			Slot:              m.Slot,
			SizeBytes:         m.SizeBytes,
			SpeedMHz:          speedMHz,
			FormFactor:        m.FormFactor,
			PartNumber:        m.PartNumber,
			FirmwareInstalled: firmwareInstalled(m.Firmware),
		})
	}

	for _, n := range device.NICs {
		if nic := nicComponentFromCommon(n); nic != nil {
			inv.NICs = append(inv.NICs, *nic)
		}
	}

	for _, d := range device.Drives {
		if d == nil {
			continue
		}
		inv.Drives = append(inv.Drives, tinkerbell.BMCDriveComponent{
			Vendor:            d.Vendor,
			Model:             d.Model,
			SerialNumber:      d.Serial,
			WWN:               d.WWN,
			SizeBytes:         d.CapacityBytes,
			Type:              d.Type,
			SmartStatus:       d.SmartStatus,
			FirmwareInstalled: firmwareInstalled(d.Firmware),
		})
	}

	for _, sc := range device.StorageControllers {
		if sc == nil {
			continue
		}
		inv.StorageControllers = append(inv.StorageControllers, componentFromCommon(sc.Common))
	}

	for _, p := range device.PSUs {
		if p == nil {
			continue
		}
		c := componentFromCommon(p.Common)
		inv.PSUs = append(inv.PSUs, tinkerbell.BMCPSUComponent{
			Vendor:             c.Vendor,
			Model:              c.Model,
			SerialNumber:       c.SerialNumber,
			Description:        c.Description,
			Status:             c.Status,
			PowerCapacityWatts: p.PowerCapacityWatts,
		})
	}

	for _, tpm := range device.TPMs {
		if tpm == nil {
			continue
		}
		c := componentFromCommon(tpm.Common)
		if c.Description == "" {
			c.Description = tpm.InterfaceType
		}
		inv.TPMs = append(inv.TPMs, c)
	}

	for _, g := range device.GPUs {
		if g == nil {
			continue
		}
		inv.GPUs = append(inv.GPUs, componentFromCommon(g.Common))
	}

	return inv
}

// productFromCommon maps the top-level Device.Common fields — the machine's
// own identity (e.g. the Redfish "System" resource) — separate from any
// individual component like the Mainboard or BMC.
func productFromCommon(c common.Common) *tinkerbell.BMCProduct {
	if c.Vendor == "" && c.Model == "" && c.ProductName == "" && c.Serial == "" {
		return nil
	}
	return &tinkerbell.BMCProduct{
		Vendor:       c.Vendor,
		Model:        c.Model,
		ProductName:  c.ProductName,
		SerialNumber: c.Serial,
		Status:       statusFromCommon(c.Status),
	}
}

func firmwareInstalled(f *common.Firmware) string {
	if f == nil {
		return ""
	}
	return f.Installed
}

func statusFromCommon(s *common.Status) *tinkerbell.BMCStatus {
	if s == nil {
		return nil
	}
	postCode, _ := safecast.Convert[int32](s.PostCode)
	return &tinkerbell.BMCStatus{
		Health:         s.Health,
		State:          s.State,
		PostCode:       postCode,
		PostCodeStatus: s.PostCodeStatus,
	}
}

func firmwareComponentFromBIOS(b *common.BIOS) *tinkerbell.BMCFirmwareComponent {
	if b == nil {
		return nil
	}
	return &tinkerbell.BMCFirmwareComponent{
		Vendor:            b.Vendor,
		Model:             b.Model,
		SerialNumber:      b.Serial,
		FirmwareInstalled: firmwareInstalled(b.Firmware),
		Status:            statusFromCommon(b.Status),
	}
}

func firmwareComponentFromBMC(bmcComp *common.BMC) *tinkerbell.BMCFirmwareComponent {
	if bmcComp == nil {
		return nil
	}
	return &tinkerbell.BMCFirmwareComponent{
		Vendor:            bmcComp.Vendor,
		Model:             bmcComp.Model,
		SerialNumber:      bmcComp.Serial,
		FirmwareInstalled: firmwareInstalled(bmcComp.Firmware),
		Status:            statusFromCommon(bmcComp.Status),
		NIC:               nicComponentFromCommon(bmcComp.NIC),
	}
}

// nicComponentFromCommon converts a bmclib NIC (used for both the host's NICs
// and the BMC's own out-of-band management NIC) into its Tinkerbell API
// representation.
func nicComponentFromCommon(n *common.NIC) *tinkerbell.BMCNICComponent {
	if n == nil {
		return nil
	}
	nic := &tinkerbell.BMCNICComponent{
		Vendor:            n.Vendor,
		Model:             n.Model,
		SerialNumber:      n.Serial,
		FirmwareInstalled: firmwareInstalled(n.Firmware),
	}
	for _, p := range n.NICPorts {
		if p == nil {
			continue
		}
		speedMbps, _ := safecast.Convert[uint32](p.SpeedBits / 1_000_000)
		mtu, _ := safecast.Convert[uint32](p.MTUSize)
		nic.Ports = append(nic.Ports, tinkerbell.BMCNICPort{
			PortID:     firstNonEmpty(p.PhysicalID, p.ID),
			MACAddress: p.MacAddress,
			LinkStatus: p.LinkStatus,
			SpeedMbps:  speedMbps,
			MTU:        mtu,
		})
	}
	return nic
}

func componentFromMainboard(m *common.Mainboard) *tinkerbell.BMCComponent {
	if m == nil {
		return nil
	}
	c := componentFromCommon(m.Common)
	return &c
}

func componentFromCommon(c common.Common) tinkerbell.BMCComponent {
	return tinkerbell.BMCComponent{
		Vendor:            c.Vendor,
		Model:             c.Model,
		SerialNumber:      c.Serial,
		Description:       c.Description,
		FirmwareInstalled: firmwareInstalled(c.Firmware),
		Status:            statusFromCommon(c.Status),
	}
}
