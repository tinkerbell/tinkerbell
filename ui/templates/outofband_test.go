package templates_test

import (
	"testing"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestOutOfBandAttributesFromStatusNil(t *testing.T) {
	if got := templates.OutOfBandAttributesFromStatus(nil); got != nil {
		t.Errorf("OutOfBandAttributesFromStatus(nil) = %+v, want nil", got)
	}
}

func TestOutOfBandAttributesFromStatus(t *testing.T) {
	now := metav1.Now()
	postCode := int32(0)
	attrs := &tinkv1alpha1.Attributes{
		LastUpdated:      &now,
		CollectionMethod: "redfish",
		BIOS: &tinkv1alpha1.BIOS{
			Vendor:          "Dell Inc.",
			FirmwareVersion: "2.10.2",
			Status:          &tinkv1alpha1.ComponentStatus{Health: "OK", PostCode: &postCode, PostCodeStatus: "success"},
		},
		CPU: &tinkv1alpha1.CPU{
			TotalCores:   24,
			TotalThreads: 48,
			Sockets: []tinkv1alpha1.CPUSocket{
				{Vendor: "Intel", Model: "Xeon Gold 6248R", Cores: 24, Threads: 48},
			},
		},
		PSUs: []tinkv1alpha1.PSU{
			{Vendor: "Dell", Status: &tinkv1alpha1.ComponentStatus{Health: "OK"}, PowerCapacityWatts: 800},
		},
		NetworkInterfaces: []tinkv1alpha1.NetworkInterface{
			{
				Vendor: "Broadcom",
				Ports: []tinkv1alpha1.NetworkPort{
					{PortID: "1", MAC: "aa:bb:cc:dd:ee:ff", LinkStatus: "Up", SpeedMbps: 25000},
					{PortID: "2", MAC: "aa:bb:cc:dd:ee:00", LinkStatus: "Down"},
				},
			},
		},
		StorageControllers: []tinkv1alpha1.StorageController{
			{Vendor: "Broadcom", Model: "HBA 9500-8i", FirmwareVersion: "24.16.0"},
		},
	}

	got := templates.OutOfBandAttributesFromStatus(attrs)
	if got == nil {
		t.Fatal("OutOfBandAttributesFromStatus() = nil, want populated")
	}
	if got.CollectionMethod != "redfish" {
		t.Errorf("CollectionMethod = %q, want redfish", got.CollectionMethod)
	}
	if got.LastUpdated == "" {
		t.Error("LastUpdated is empty, want a formatted timestamp")
	}
	if got.BIOS.Vendor != "Dell Inc." || got.BIOS.FirmwareVersion != "2.10.2" {
		t.Errorf("BIOS = %+v, want Vendor=Dell Inc. FirmwareVersion=2.10.2", got.BIOS)
	}
	if got.BIOS.Status.Health != "OK" {
		t.Errorf("BIOS.Status.Health = %q, want OK", got.BIOS.Status.Health)
	}
	if got.BIOS.Status.PostCode == nil || *got.BIOS.Status.PostCode != 0 {
		t.Errorf("BIOS.Status.PostCode = %v, want a non-nil pointer to 0", got.BIOS.Status.PostCode)
	}
	if got.TotalCores != 24 || got.TotalThreads != 48 {
		t.Errorf("TotalCores/TotalThreads = %d/%d, want 24/48", got.TotalCores, got.TotalThreads)
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
	if len(got.StorageControllers) != 1 || got.StorageControllers[0].FirmwareVersion != "24.16.0" {
		t.Errorf("StorageControllers = %+v, want one entry with FirmwareVersion=24.16.0", got.StorageControllers)
	}
}

func TestOutOfBandAttributesFromStatusEmptyComponents(t *testing.T) {
	// Attributes with only the top-level fields set (no BIOS/BMC/Baseboard) must
	// not panic — this is the common case per the data-completeness findings in
	// #844 (IPMI-only/ASRockRack drivers report much less than Redfish).
	got := templates.OutOfBandAttributesFromStatus(&tinkv1alpha1.Attributes{CollectionMethod: "asrockrack"})
	if got == nil {
		t.Fatal("OutOfBandAttributesFromStatus() = nil, want populated")
	}
	if got.BIOS.Vendor != "" {
		t.Errorf("BIOS.Vendor = %q, want empty", got.BIOS.Vendor)
	}
}

func TestOutOfBandAttributesFromStatusBMCNIC(t *testing.T) {
	attrs := &tinkv1alpha1.Attributes{
		BMC: &tinkv1alpha1.BMC{
			Vendor: "Dell",
			NIC: &tinkv1alpha1.NetworkInterface{
				Ports: []tinkv1alpha1.NetworkPort{{MAC: "aa:bb:cc:dd:ee:ff"}},
			},
		},
	}

	got := templates.OutOfBandAttributesFromStatus(attrs)
	if got.BMC.NIC == nil {
		t.Fatal("BMC.NIC = nil, want populated")
	}
	if macs := got.BMC.NIC.MACAddresses(); macs != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("BMC.NIC.MACAddresses() = %q, want aa:bb:cc:dd:ee:ff", macs)
	}
}
