package controller

import (
	"context"
	"testing"
	"time"

	common "github.com/bmc-toolbox/common"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBMCInventoryFromDevice(t *testing.T) {
	now := metav1.Now()
	device := &common.Device{
		Common: common.Common{
			Vendor:      "Dell Inc.",
			Model:       "PowerEdge R750",
			ProductName: "PowerEdge R750",
			Serial:      "SYS-SERIAL-1",
		},
		BIOS: &common.BIOS{
			Common: common.Common{
				Vendor: "Dell Inc.",
				Model:  "2.10.2",
				Serial: "", // CPU/BIOS serials are frequently blank upstream in bmclib
				Status: &common.Status{Health: "OK", State: "Enabled", PostCode: 0},
				Firmware: &common.Firmware{
					Installed: "2.10.2",
				},
			},
		},
		BMC: &common.BMC{
			Common: common.Common{
				Vendor: "Dell Inc.",
				Firmware: &common.Firmware{
					Installed: "6.10.30.00",
				},
			},
			NIC: &common.NIC{
				NICPorts: []*common.NICPort{
					{
						ID:         "1",
						MacAddress: "00:00:5e:00:53:af",
						LinkStatus: "Up",
						SpeedBits:  1_000_000_000,
					},
				},
			},
		},
		Mainboard: &common.Mainboard{
			Common: common.Common{Vendor: "Dell Inc.", Serial: "MB123"},
		},
		CPUs: []*common.CPU{
			{
				Common:       common.Common{Vendor: "Intel", Model: "Xeon Gold 6248R"},
				ID:           "CPU.1",
				Slot:         "CPU1",
				Cores:        24,
				Threads:      48,
				ClockSpeedHz: 3000000000,
			},
			nil, // nil entries must be skipped, not panic
		},
		Memory: []*common.Memory{
			{
				Common:       common.Common{Vendor: "Samsung"},
				ID:           "DIMM.1",
				Slot:         "A1",
				SizeBytes:    34359738368,
				ClockSpeedHz: 3200000000,
				FormFactor:   "DIMM",
				PartNumber:   "M393A4K40DB3-CWE",
			},
		},
		NICs: []*common.NIC{
			{
				Common: common.Common{Vendor: "Broadcom", Serial: "NIC-1"},
				ID:     "NIC.1",
				NICPorts: []*common.NICPort{
					{
						ID:         "1",
						PhysicalID: "NIC.1.1",
						MacAddress: "aa:bb:cc:dd:ee:ff",
						LinkStatus: "Up",
						SpeedBits:  25_000_000_000,
						MTUSize:    9000,
					},
					nil, // nil ports must be skipped, not panic
				},
			},
		},
		Drives: []*common.Drive{
			{
				Common:        common.Common{Vendor: "Samsung"},
				ID:            "Disk.1",
				CapacityBytes: 960197124096,
				Type:          "SSD",
				WWN:           "0x5002538e40a12345",
				SmartStatus:   "ok",
			},
		},
		StorageControllers: []*common.StorageController{
			{
				Common: common.Common{Vendor: "Dell", Firmware: &common.Firmware{Installed: "25.5.9.0001"}},
				ID:     "RAID.1",
			},
		},
		PSUs: []*common.PSU{
			{
				Common:             common.Common{Vendor: "Dell", Status: &common.Status{Health: "OK"}},
				ID:                 "PSU.1",
				PowerCapacityWatts: 800,
			},
		},
		TPMs: []*common.TPM{
			{Common: common.Common{Serial: "TPM-1"}, InterfaceType: "TPM2_0"},
		},
		GPUs: []*common.GPU{
			{Common: common.Common{Vendor: "NVIDIA", Model: "A100"}},
		},
	}

	got := bmcInventoryFromDevice(device, "redfish", &now)

	if got.CollectionMethod != "redfish" {
		t.Errorf("CollectionMethod = %q, want %q", got.CollectionMethod, "redfish")
	}
	if got.LastUpdated != &now {
		t.Errorf("LastUpdated not set to the provided timestamp")
	}
	if got.BIOS == nil || got.BIOS.FirmwareInstalled != "2.10.2" {
		t.Errorf("BIOS.FirmwareInstalled = %+v, want 2.10.2", got.BIOS)
	}
	if got.BIOS.Status == nil || got.BIOS.Status.Health != "OK" {
		t.Errorf("BIOS.Status.Health = %+v, want OK", got.BIOS.Status)
	}
	if got.BMC == nil || got.BMC.FirmwareInstalled != "6.10.30.00" {
		t.Errorf("BMC.FirmwareInstalled = %+v, want 6.10.30.00", got.BMC)
	}
	if got.BMC.NIC == nil || len(got.BMC.NIC.Ports) != 1 || got.BMC.NIC.Ports[0].MACAddress != "00:00:5e:00:53:af" {
		t.Errorf("BMC.NIC = %+v, want one port with MACAddress=00:00:5e:00:53:af", got.BMC.NIC)
	}
	for _, n := range got.NICs {
		for _, p := range n.Ports {
			if p.MACAddress == "00:00:5e:00:53:af" {
				t.Errorf("BMC's own NIC MAC leaked into host NICs list: %+v", n)
			}
		}
	}
	if got.Mainboard == nil || got.Mainboard.SerialNumber != "MB123" {
		t.Errorf("Mainboard.SerialNumber = %+v, want MB123", got.Mainboard)
	}
	if got.Product == nil || got.Product.SerialNumber != "SYS-SERIAL-1" || got.Product.ProductName != "PowerEdge R750" {
		t.Errorf("Product = %+v, want SerialNumber=SYS-SERIAL-1 ProductName=PowerEdge R750", got.Product)
	}
	if len(got.CPUs) != 1 {
		t.Fatalf("len(CPUs) = %d, want 1 (nil entries must be skipped)", len(got.CPUs))
	}
	if got.CPUs[0].Cores != 24 || got.CPUs[0].Threads != 48 || got.CPUs[0].Slot != "CPU1" || got.CPUs[0].ClockSpeedMHz != 3000 {
		t.Errorf("CPUs[0] = %+v, want Cores=24 Threads=48 Slot=CPU1 ClockSpeedMHz=3000", got.CPUs[0])
	}
	if len(got.Memory) != 1 || got.Memory[0].SpeedMHz != 3200 || got.Memory[0].FormFactor != "DIMM" || got.Memory[0].PartNumber != "M393A4K40DB3-CWE" {
		t.Errorf("Memory[0] = %+v, want SpeedMHz=3200 FormFactor=DIMM PartNumber=M393A4K40DB3-CWE", got.Memory)
	}
	if len(got.NICs) != 1 || got.NICs[0].SerialNumber != "NIC-1" {
		t.Errorf("NICs[0] = %+v, want SerialNumber=NIC-1", got.NICs)
	}
	if len(got.NICs[0].Ports) != 1 {
		t.Fatalf("len(NICs[0].Ports) = %d, want 1 (nil port entries must be skipped)", len(got.NICs[0].Ports))
	}
	port := got.NICs[0].Ports[0]
	if port.PortID != "NIC.1.1" || port.MACAddress != "aa:bb:cc:dd:ee:ff" || port.LinkStatus != "Up" || port.SpeedMbps != 25000 || port.MTU != 9000 {
		t.Errorf("NICs[0].Ports[0] = %+v, want PortID=NIC.1.1 MACAddress=aa:bb:cc:dd:ee:ff LinkStatus=Up SpeedMbps=25000 MTU=9000", port)
	}
	if len(got.Drives) != 1 || got.Drives[0].SizeBytes != 960197124096 || got.Drives[0].WWN != "0x5002538e40a12345" || got.Drives[0].SmartStatus != "ok" {
		t.Errorf("Drives[0] = %+v, want SizeBytes=960197124096 WWN=0x5002538e40a12345 SmartStatus=ok", got.Drives)
	}
	if len(got.StorageControllers) != 1 || got.StorageControllers[0].FirmwareInstalled != "25.5.9.0001" {
		t.Errorf("StorageControllers = %+v, want 1 entry with FirmwareInstalled=25.5.9.0001", got.StorageControllers)
	}
	if len(got.PSUs) != 1 || got.PSUs[0].Status == nil || got.PSUs[0].Status.Health != "OK" || got.PSUs[0].PowerCapacityWatts != 800 {
		t.Errorf("PSUs[0] = %+v, want Status.Health=OK PowerCapacityWatts=800", got.PSUs)
	}
	if len(got.TPMs) != 1 || got.TPMs[0].Description != "TPM2_0" {
		t.Errorf("TPMs[0] = %+v, want Description=TPM2_0 (from InterfaceType fallback)", got.TPMs)
	}
	if len(got.GPUs) != 1 || got.GPUs[0].Model != "A100" {
		t.Errorf("GPUs[0] = %+v, want Model=A100", got.GPUs)
	}
}

func TestBMCInventoryFromDeviceNil(t *testing.T) {
	if got := bmcInventoryFromDevice(nil, "redfish", nil); got != nil {
		t.Errorf("bmcInventoryFromDevice(nil, ...) = %+v, want nil", got)
	}
}

// TestApplyBMCInventoryNilDeviceNoPanic is a regression test: a provider
// implementation that returns (nil, nil) from Inventory() — no error, but also
// no device — must not panic applyBMCInventory's idempotency-guard comparison.
func TestApplyBMCInventoryNilDeviceNoPanic(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := api.AddToSchemeTinkerbell(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hw", Namespace: "test-namespace"},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&tinkerbell.Hardware{}).
		WithObjects(hw).
		Build()

	r := &MachineReconciler{client: fakeClient}
	if err := r.applyBMCInventory(context.Background(), hw, nil, "redfish"); err != nil {
		t.Fatalf("applyBMCInventory(nil device) error = %v, want nil", err)
	}
}

// TestSortDeviceDeterminism verifies that feeding the same logical inventory in
// two different slice orderings produces byte-identical mapped output — this is
// the fix for the reconcile-loop bug where BMCs return list fields in
// non-deterministic order across polls.
func TestSortDeviceDeterminism(t *testing.T) {
	build := func(order []string) *common.Device {
		cpus := make([]*common.CPU, 0, len(order))
		for _, id := range order {
			cpus = append(cpus, &common.CPU{ID: id, Common: common.Common{Vendor: "Intel"}})
		}
		return &common.Device{CPUs: cpus}
	}

	a := build([]string{"CPU.2", "CPU.1"})
	b := build([]string{"CPU.1", "CPU.2"})

	sortDevice(a)
	sortDevice(b)

	invA := bmcInventoryFromDevice(a, "redfish", nil)
	invB := bmcInventoryFromDevice(b, "redfish", nil)

	if diff := cmp.Diff(invA, invB); diff != "" {
		t.Errorf("sorted inventories differ despite same logical content (-a +b):\n%s", diff)
	}
}

// TestSortDeviceDeterminism_GPUsAndBMCNIC is a regression test for a gap where
// sortDevice sorted every other slice field but left Device.GPUs and
// Device.BMC.NIC.NICPorts (the BMC's own management NIC) in whatever order the
// BMC returned them, letting those two fields still churn status writes.
func TestSortDeviceDeterminism_GPUsAndBMCNIC(t *testing.T) {
	build := func(gpuOrder, bmcNICPortOrder []string) *common.Device {
		gpus := make([]*common.GPU, 0, len(gpuOrder))
		for _, serial := range gpuOrder {
			gpus = append(gpus, &common.GPU{Common: common.Common{Vendor: "NVIDIA", Serial: serial}})
		}
		ports := make([]*common.NICPort, 0, len(bmcNICPortOrder))
		for _, id := range bmcNICPortOrder {
			ports = append(ports, &common.NICPort{ID: id, MacAddress: "aa:bb:cc:dd:ee:" + id})
		}
		return &common.Device{
			GPUs: gpus,
			BMC:  &common.BMC{NIC: &common.NIC{NICPorts: ports}},
		}
	}

	a := build([]string{"GPU.2", "GPU.1"}, []string{"02", "01"})
	b := build([]string{"GPU.1", "GPU.2"}, []string{"01", "02"})

	sortDevice(a)
	sortDevice(b)

	invA := bmcInventoryFromDevice(a, "redfish", nil)
	invB := bmcInventoryFromDevice(b, "redfish", nil)

	if diff := cmp.Diff(invA, invB); diff != "" {
		t.Errorf("sorted inventories differ despite same logical content (-a +b):\n%s", diff)
	}
}

func TestDueForInventoryRefresh(t *testing.T) {
	now := metav1.Now()
	stale := metav1.NewTime(now.Add(-25 * time.Hour))
	fresh := metav1.NewTime(now.Add(-1 * time.Hour))

	tests := map[string]struct {
		hw   *tinkerbell.Hardware
		bm   *bmc.Machine
		want bool
	}{
		"never collected": {
			hw:   &tinkerbell.Hardware{},
			bm:   &bmc.Machine{},
			want: true,
		},
		"stale": {
			hw:   &tinkerbell.Hardware{Status: tinkerbell.HardwareStatus{BMCInventory: &tinkerbell.BMCInventory{LastUpdated: &stale}}},
			bm:   &bmc.Machine{},
			want: true,
		},
		"fresh": {
			hw:   &tinkerbell.Hardware{Status: tinkerbell.HardwareStatus{BMCInventory: &tinkerbell.BMCInventory{LastUpdated: &fresh}}},
			bm:   &bmc.Machine{},
			want: false,
		},
		"fresh but refresh annotation forces it": {
			hw: &tinkerbell.Hardware{Status: tinkerbell.HardwareStatus{BMCInventory: &tinkerbell.BMCInventory{LastUpdated: &fresh}}},
			bm: &bmc.Machine{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{refreshInventoryAnnotation: "true"},
			}},
			want: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := dueForInventoryRefresh(tt.hw, tt.bm); got != tt.want {
				t.Errorf("dueForInventoryRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}
