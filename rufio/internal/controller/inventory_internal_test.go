package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	common "github.com/bmc-toolbox/common"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestAttributesFromDevice(t *testing.T) {
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

	got := attributesFromDevice(device, "redfish", &now)

	if got.CollectionMethod != "redfish" {
		t.Errorf("CollectionMethod = %q, want %q", got.CollectionMethod, "redfish")
	}
	if got.LastUpdated != &now {
		t.Errorf("LastUpdated not set to the provided timestamp")
	}
	if got.BIOS == nil || got.BIOS.FirmwareVersion != "2.10.2" {
		t.Errorf("BIOS.FirmwareVersion = %+v, want 2.10.2", got.BIOS)
	}
	if got.BIOS.Status == nil || got.BIOS.Status.Health != "OK" {
		t.Errorf("BIOS.Status.Health = %+v, want OK", got.BIOS.Status)
	}
	if got.BMC == nil || got.BMC.FirmwareVersion != "6.10.30.00" {
		t.Errorf("BMC.FirmwareVersion = %+v, want 6.10.30.00", got.BMC)
	}
	if got.BMC.NIC == nil || len(got.BMC.NIC.Ports) != 1 || got.BMC.NIC.Ports[0].MAC != "00:00:5e:00:53:af" {
		t.Errorf("BMC.NIC = %+v, want one port with MAC=00:00:5e:00:53:af", got.BMC.NIC)
	}
	for _, n := range got.NetworkInterfaces {
		for _, p := range n.Ports {
			if p.MAC == "00:00:5e:00:53:af" {
				t.Errorf("BMC's own NIC MAC leaked into host NetworkInterfaces list: %+v", n)
			}
		}
	}
	if got.Baseboard == nil || got.Baseboard.SerialNumber != "MB123" {
		t.Errorf("Baseboard.SerialNumber = %+v, want MB123", got.Baseboard)
	}
	if got.Product == nil || got.Product.SerialNumber != "SYS-SERIAL-1" || got.Product.Name != "PowerEdge R750" {
		t.Errorf("Product = %+v, want SerialNumber=SYS-SERIAL-1 Name=PowerEdge R750", got.Product)
	}
	if len(got.CPU.Sockets) != 1 {
		t.Fatalf("len(CPU.Sockets) = %d, want 1 (nil entries must be skipped)", len(got.CPU.Sockets))
	}
	if got.CPU.Sockets[0].Cores != 24 || got.CPU.Sockets[0].Threads != 48 || got.CPU.Sockets[0].Slot != "CPU1" || got.CPU.Sockets[0].ClockSpeedMHz != 3000 {
		t.Errorf("CPU.Sockets[0] = %+v, want Cores=24 Threads=48 Slot=CPU1 ClockSpeedMHz=3000", got.CPU.Sockets[0])
	}
	if len(got.Memory.Modules) != 1 || got.Memory.Modules[0].SpeedMHz != 3200 || got.Memory.Modules[0].FormFactor != "DIMM" || got.Memory.Modules[0].PartNumber != "M393A4K40DB3-CWE" {
		t.Errorf("Memory.Modules[0] = %+v, want SpeedMHz=3200 FormFactor=DIMM PartNumber=M393A4K40DB3-CWE", got.Memory.Modules)
	}
	if len(got.NetworkInterfaces) != 1 || got.NetworkInterfaces[0].SerialNumber != "NIC-1" {
		t.Errorf("NetworkInterfaces[0] = %+v, want SerialNumber=NIC-1", got.NetworkInterfaces)
	}
	if len(got.NetworkInterfaces[0].Ports) != 1 {
		t.Fatalf("len(NetworkInterfaces[0].Ports) = %d, want 1 (nil port entries must be skipped)", len(got.NetworkInterfaces[0].Ports))
	}
	port := got.NetworkInterfaces[0].Ports[0]
	if port.PortID != "NIC.1.1" || port.MAC != "aa:bb:cc:dd:ee:ff" || port.LinkStatus != "Up" || port.SpeedMbps != 25000 || port.MTU != 9000 {
		t.Errorf("NetworkInterfaces[0].Ports[0] = %+v, want PortID=NIC.1.1 MAC=aa:bb:cc:dd:ee:ff LinkStatus=Up SpeedMbps=25000 MTU=9000", port)
	}
	if len(got.BlockDevices) != 1 || got.BlockDevices[0].SizeBytes != 960197124096 || got.BlockDevices[0].WWN != "0x5002538e40a12345" || got.BlockDevices[0].SmartStatus != "ok" {
		t.Errorf("BlockDevices[0] = %+v, want SizeBytes=960197124096 WWN=0x5002538e40a12345 SmartStatus=ok", got.BlockDevices)
	}
	if len(got.StorageControllers) != 1 || got.StorageControllers[0].FirmwareVersion != "25.5.9.0001" {
		t.Errorf("StorageControllers = %+v, want 1 entry with FirmwareVersion=25.5.9.0001", got.StorageControllers)
	}
	if len(got.PSUs) != 1 || got.PSUs[0].Status == nil || got.PSUs[0].Status.Health != "OK" || got.PSUs[0].PowerCapacityWatts != 800 {
		t.Errorf("PSUs[0] = %+v, want Status.Health=OK PowerCapacityWatts=800", got.PSUs)
	}
	if len(got.TPMs) != 1 || got.TPMs[0].InterfaceType != "TPM2_0" {
		t.Errorf("TPMs[0] = %+v, want InterfaceType=TPM2_0", got.TPMs)
	}
	if len(got.GPUDevices) != 1 || got.GPUDevices[0].Model != "A100" {
		t.Errorf("GPUDevices[0] = %+v, want Model=A100", got.GPUDevices)
	}
}

func TestAttributesFromDeviceNil(t *testing.T) {
	if got := attributesFromDevice(nil, "redfish", nil); got != nil {
		t.Errorf("attributesFromDevice(nil, ...) = %+v, want nil", got)
	}
}

// TestProductStatusPostCode covers the device-level POST diagnostics. bmclib only
// populates PostCode on Device.Status (and only in the asrockrack driver), so it
// maps onto Product rather than BIOS, and a zero code — a successful POST — must
// survive rather than being dropped as an empty value.
func TestProductStatusPostCode(t *testing.T) {
	device := &common.Device{
		Common: common.Common{
			Vendor: "ASRockRack",
			Status: &common.Status{Health: "OK", PostCode: 0, PostCodeStatus: "boot complete"},
		},
	}

	got := attributesFromDevice(device, "asrockrack", nil)

	if got.Product == nil || got.Product.Status == nil {
		t.Fatalf("Product.Status = %+v, want POST diagnostics mapped", got.Product)
	}
	if got.Product.Status.PostCode == nil {
		t.Fatal("Product.Status.PostCode = nil, want a pointer to 0 (a successful POST must not be dropped)")
	}
	if *got.Product.Status.PostCode != 0 {
		t.Errorf("*Product.Status.PostCode = %d, want 0", *got.Product.Status.PostCode)
	}
	if got.Product.Status.PostCodeStatus != "boot complete" {
		t.Errorf("Product.Status.PostCodeStatus = %q, want %q", got.Product.Status.PostCodeStatus, "boot complete")
	}
}

// TestProductStatusPostCodeWithNoIdentityFields is a regression test: a driver
// (e.g. asrockrack, when its FRU 'board' lookup fails) can report POST
// diagnostics with no Vendor/Model/ProductName/Serial at all. productFromCommon
// must not drop the whole Product in that case — the POST code is the one thing
// this mapping goes out of its way to preserve (see TestProductStatusPostCode).
func TestProductStatusPostCodeWithNoIdentityFields(t *testing.T) {
	device := &common.Device{
		Common: common.Common{
			Status: &common.Status{Health: "OK", PostCode: 5, PostCodeStatus: "boot complete"},
		},
	}

	got := attributesFromDevice(device, "asrockrack", nil)

	if got.Product == nil || got.Product.Status == nil {
		t.Fatalf("Product.Status = %+v, want POST diagnostics mapped even with no identity fields set", got.Product)
	}
	if got.Product.Status.PostCode == nil || *got.Product.Status.PostCode != 5 {
		t.Errorf("Product.Status.PostCode = %v, want a pointer to 5", got.Product.Status.PostCode)
	}
}

// TestComponentStatusOmitsPostCode verifies the flip side: POST diagnostics are a
// device-level concept, so per-component statuses must not carry a meaningless
// postCode of 0.
func TestComponentStatusOmitsPostCode(t *testing.T) {
	device := &common.Device{
		BIOS: &common.BIOS{
			Common: common.Common{Vendor: "Dell Inc.", Status: &common.Status{Health: "OK", State: "Enabled"}},
		},
	}

	got := attributesFromDevice(device, "redfish", nil)

	if got.BIOS == nil || got.BIOS.Status == nil {
		t.Fatalf("BIOS.Status = %+v, want health/state mapped", got.BIOS)
	}
	if got.BIOS.Status.PostCode != nil {
		t.Errorf("BIOS.Status.PostCode = %v, want nil (POST code is device-level, not per-component)", *got.BIOS.Status.PostCode)
	}
}

// TestApplyOutOfBandAttributesNilDeviceNoPanic is a regression test: a provider
// implementation that returns (nil, nil) from Inventory() — no error, but also
// no device — must not panic applyOutOfBandAttributes's idempotency-guard
// comparison, and must still record LastUpdated. Without that,
// dueForInventoryRefresh treats this Hardware as never collected forever and
// retries the slow BMC round-trip on every subsequent reconcile instead of
// respecting inventoryRefreshInterval.
func TestApplyOutOfBandAttributesNilDeviceNoPanic(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := tinkerbell.AddToScheme(scheme); err != nil {
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
	if err := r.applyOutOfBandAttributes(context.Background(), hw, nil, "redfish"); err != nil {
		t.Fatalf("applyOutOfBandAttributes(nil device) error = %v, want nil", err)
	}

	got := outOfBandAttributes(hw)
	if got == nil || got.LastUpdated == nil {
		t.Fatalf("outOfBand = %+v, want LastUpdated set even with no device data, so the refresh gate advances", got)
	}
	if dueForInventoryRefresh(hw, &bmc.Machine{}) {
		t.Error("dueForInventoryRefresh() = true right after a (nil, nil) collection, want false")
	}
}

// TestApplyOutOfBandAttributesNilDevicePreservesExisting is a regression test: a
// provider that starts returning (nil, nil) from Inventory() after a previous
// successful collection must leave the stored component data alone (no data
// loss) but still advance LastUpdated to "now" — otherwise
// dueForInventoryRefresh treats this Hardware as never collected and retries
// the slow BMC round-trip on every subsequent reconcile.
func TestApplyOutOfBandAttributesNilDevicePreservesExisting(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := tinkerbell.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}
	collected := metav1.NewTime(metav1.Now().Add(-25 * time.Hour))
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hw", Namespace: "test-namespace"},
		Status: tinkerbell.HardwareStatus{
			Attributes: &tinkerbell.HardwareAttributes{
				OutOfBand: &tinkerbell.Attributes{
					LastUpdated:      &collected,
					CollectionMethod: "redfish",
					Product:          &tinkerbell.Product{SerialNumber: "SYS-SERIAL-1"},
				},
			},
		},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&tinkerbell.Hardware{}).
		WithObjects(hw).
		Build()

	r := &MachineReconciler{client: fakeClient}
	if err := r.applyOutOfBandAttributes(context.Background(), hw, nil, "redfish"); err != nil {
		t.Fatalf("applyOutOfBandAttributes(nil device) error = %v, want nil", err)
	}

	got := outOfBandAttributes(hw)
	if got == nil || got.Product == nil || got.Product.SerialNumber != "SYS-SERIAL-1" {
		t.Errorf("outOfBand = %+v, want the previously collected inventory left intact", got)
	}
	if got.LastUpdated == nil || !got.LastUpdated.After(collected.Time) {
		t.Errorf("outOfBand.LastUpdated = %v, want it advanced past the previous %v", got.LastUpdated, collected)
	}
}

// TestClearRefreshInventoryAnnotationRetries is a regression test: a
// transiently-failing Patch must be retried rather than silently leaving
// refreshInventoryAnnotation set on the server, which would otherwise force a
// full BMC inventory collection on every subsequent reconcile.
func TestClearRefreshInventoryAnnotationRetries(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := bmc.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to build scheme: %v", err)
	}
	bm := &bmc.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-bm",
			Namespace:   "test-namespace",
			Annotations: map[string]string{refreshInventoryAnnotation: "true"},
		},
	}

	var attempts int
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(bm).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				attempts++
				if attempts < 3 {
					return errors.New("injected transient failure")
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	r := &MachineReconciler{client: fakeClient, recorder: events.NewFakeRecorder(2)}
	r.clearRefreshInventoryAnnotation(context.Background(), logr.Discard(), bm)

	if attempts != 3 {
		t.Errorf("Patch called %d times, want 3 (2 injected failures then a success)", attempts)
	}
	if _, ok := bm.Annotations[refreshInventoryAnnotation]; ok {
		t.Error("refresh-inventory annotation still present after retries succeeded, want it cleared")
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

	invA := attributesFromDevice(a, "redfish", nil)
	invB := attributesFromDevice(b, "redfish", nil)

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

	invA := attributesFromDevice(a, "redfish", nil)
	invB := attributesFromDevice(b, "redfish", nil)

	if diff := cmp.Diff(invA, invB); diff != "" {
		t.Errorf("sorted inventories differ despite same logical content (-a +b):\n%s", diff)
	}
}

// TestSortDeviceDeterminism_TiedKeys is a regression test: components with no
// Serial/ID/Slot (so cpuKey/gpuKey/etc. all return "") must still sort into a
// single canonical order across polls that return them in different input
// orders, rather than being left in BMC-supplied (non-deterministic) order
// because their primary sort key ties.
func TestSortDeviceDeterminism_TiedKeys(t *testing.T) {
	build := func(order []string) *common.Device {
		gpus := make([]*common.GPU, 0, len(order))
		for _, vendor := range order {
			// No Serial/Model/ProductName set: gpuKey() returns "" for all of
			// these, so they tie on the primary key.
			gpus = append(gpus, &common.GPU{Common: common.Common{Vendor: vendor}})
		}
		return &common.Device{GPUs: gpus}
	}

	a := build([]string{"AMD", "NVIDIA", "Intel"})
	b := build([]string{"Intel", "AMD", "NVIDIA"})

	sortDevice(a)
	sortDevice(b)

	invA := attributesFromDevice(a, "redfish", nil)
	invB := attributesFromDevice(b, "redfish", nil)

	if diff := cmp.Diff(invA, invB); diff != "" {
		t.Errorf("sorted inventories differ despite same logical content with tied keys (-a +b):\n%s", diff)
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
			hw:   &tinkerbell.Hardware{Status: tinkerbell.HardwareStatus{Attributes: &tinkerbell.HardwareAttributes{OutOfBand: &tinkerbell.Attributes{LastUpdated: &stale}}}},
			bm:   &bmc.Machine{},
			want: true,
		},
		"fresh": {
			hw:   &tinkerbell.Hardware{Status: tinkerbell.HardwareStatus{Attributes: &tinkerbell.HardwareAttributes{OutOfBand: &tinkerbell.Attributes{LastUpdated: &fresh}}}},
			bm:   &bmc.Machine{},
			want: false,
		},
		"fresh but refresh annotation forces it": {
			hw: &tinkerbell.Hardware{Status: tinkerbell.HardwareStatus{Attributes: &tinkerbell.HardwareAttributes{OutOfBand: &tinkerbell.Attributes{LastUpdated: &fresh}}}},
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
