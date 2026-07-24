package templates_test

import (
	"testing"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBMCInventoryFromStatusNil(t *testing.T) {
	if got := templates.BMCInventoryFromStatus(nil); got != nil {
		t.Errorf("BMCInventoryFromStatus(nil) = %+v, want nil", got)
	}
}

func TestBMCInventoryFromStatus(t *testing.T) {
	now := metav1.Now()
	inv := &tinkv1alpha1.BMCInventory{
		LastUpdated:      &now,
		CollectionMethod: "redfish",
		BIOS: &tinkv1alpha1.BMCFirmwareComponent{
			Vendor:            "Dell Inc.",
			FirmwareInstalled: "2.10.2",
			Status:            &tinkv1alpha1.BMCStatus{Health: "OK"},
		},
		CPUs: []tinkv1alpha1.BMCCPUComponent{
			{Vendor: "Intel", Model: "Xeon Gold 6248R", Cores: 24, Threads: 48},
		},
		PSUs: []tinkv1alpha1.BMCPSUComponent{
			{Vendor: "Dell", Status: &tinkv1alpha1.BMCStatus{Health: "OK"}, PowerCapacityWatts: 800},
		},
		NICs: []tinkv1alpha1.BMCNICComponent{
			{
				Vendor: "Broadcom",
				Ports: []tinkv1alpha1.BMCNICPort{
					{PortID: "1", MACAddress: "aa:bb:cc:dd:ee:ff", LinkStatus: "Up", SpeedMbps: 25000},
					{PortID: "2", MACAddress: "aa:bb:cc:dd:ee:00", LinkStatus: "Down"},
				},
			},
		},
		StorageControllers: []tinkv1alpha1.BMCComponent{
			{Vendor: "Broadcom", Model: "HBA 9500-8i", FirmwareInstalled: "24.16.0"},
		},
	}

	got := templates.BMCInventoryFromStatus(inv)
	if got == nil {
		t.Fatal("BMCInventoryFromStatus() = nil, want populated")
	}
	if got.CollectionMethod != "redfish" {
		t.Errorf("CollectionMethod = %q, want redfish", got.CollectionMethod)
	}
	if got.LastUpdated == "" {
		t.Error("LastUpdated is empty, want a formatted timestamp")
	}
	if got.BIOS.Vendor != "Dell Inc." || got.BIOS.FirmwareInstalled != "2.10.2" {
		t.Errorf("BIOS = %+v, want Vendor=Dell Inc. FirmwareInstalled=2.10.2", got.BIOS)
	}
	if got.BIOS.Status.Health != "OK" {
		t.Errorf("BIOS.Status.Health = %q, want OK", got.BIOS.Status.Health)
	}
	if len(got.CPUs) != 1 || got.CPUs[0].Cores != 24 {
		t.Errorf("CPUs = %+v, want one entry with Cores=24", got.CPUs)
	}
	if len(got.PSUs) != 1 || got.PSUs[0].Status.Health != "OK" || got.PSUs[0].PowerCapacityWatts != 800 {
		t.Errorf("PSUs = %+v, want one entry with Status.Health=OK PowerCapacityWatts=800", got.PSUs)
	}
	if len(got.NICs) != 1 || len(got.NICs[0].Ports) != 2 {
		t.Fatalf("NICs = %+v, want one NIC with 2 ports", got.NICs)
	}
	if macs := got.NICs[0].MACAddresses(); macs != "aa:bb:cc:dd:ee:ff, aa:bb:cc:dd:ee:00" {
		t.Errorf("MACAddresses() = %q, want both ports joined", macs)
	}
	if speeds := got.NICs[0].PortSpeeds(); speeds != "25000 Mbps" {
		t.Errorf("PortSpeeds() = %q, want only the port with a nonzero speed", speeds)
	}
	if len(got.StorageControllers) != 1 || got.StorageControllers[0].FirmwareInstalled != "24.16.0" {
		t.Errorf("StorageControllers = %+v, want one entry with FirmwareInstalled=24.16.0", got.StorageControllers)
	}
}

func TestBMCInventoryFromStatusEmptyComponents(t *testing.T) {
	// A BMCInventory with only the top-level fields set (no BIOS/BMC/Mainboard)
	// must not panic — this is the common case per the data-completeness
	// findings (IPMI-only/ASRockRack drivers report much less than Redfish).
	got := templates.BMCInventoryFromStatus(&tinkv1alpha1.BMCInventory{CollectionMethod: "asrockrack"})
	if got == nil {
		t.Fatal("BMCInventoryFromStatus() = nil, want populated")
	}
	if got.BIOS.Vendor != "" {
		t.Errorf("BIOS.Vendor = %q, want empty", got.BIOS.Vendor)
	}
}
