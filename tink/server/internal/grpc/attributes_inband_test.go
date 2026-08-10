package grpc

import (
	"testing"

	"github.com/tinkerbell/tinkerbell/pkg/data"
)

func ptr[T any](v T) *T { return &v }

func TestInBandAttributesFromAgentNil(t *testing.T) {
	if got := inBandAttributesFromAgent(nil); got != nil {
		t.Errorf("inBandAttributesFromAgent(nil) = %+v, want nil", got)
	}
}

func TestInBandAttributesFromAgent(t *testing.T) {
	attrs := &data.AgentAttributes{
		CPU: &data.CPU{
			TotalCores:   ptr(uint32(8)),
			TotalThreads: ptr(uint32(16)),
			Processors: []*data.Processor{
				{Vendor: ptr("GenuineIntel"), Model: ptr("i7"), Cores: ptr(uint32(8)), Threads: ptr(uint32(16)), Capabilities: []string{"fpu", "vme"}},
			},
		},
		BlockDevices: []*data.Block{
			{Name: ptr("nvme0n1"), Vendor: ptr("Samsung"), SerialNumber: ptr("S1")},
		},
		NetworkInterfaces: []*data.Network{
			{Name: ptr("eno1"), Mac: ptr("aa:bb:cc:dd:ee:ff"), Speed: ptr("1000Mb/s"), EnabledCapabilities: []string{"tso"}},
		},
		PCIDevices: []*data.PCI{
			{Vendor: ptr("Intel"), Product: ptr("Ethernet Controller"), Class: ptr("0200"), Driver: ptr("ixgbe")},
		},
		GPUDevices: []*data.GPU{
			{Vendor: ptr("NVIDIA"), Product: ptr("A100"), Class: ptr("0300"), Driver: ptr("nvidia")},
		},
		Chassis:   &data.Chassis{Vendor: ptr("Dell"), Serial: ptr("CH1")},
		BIOS:      &data.BIOS{Vendor: ptr("Dell Inc."), Version: ptr("2.10.2"), ReleaseDate: ptr("01/01/2024")},
		Baseboard: &data.Baseboard{Vendor: ptr("Dell"), Product: ptr("0ABC123"), Version: ptr("A01"), SerialNumber: ptr("BB1")},
		Product:   &data.Product{Name: ptr("PowerEdge R750"), Vendor: ptr("Dell Inc."), SerialNumber: ptr("SN1")},
	}

	got := inBandAttributesFromAgent(attrs)
	if got == nil {
		t.Fatal("inBandAttributesFromAgent() = nil, want populated")
	}

	if got.CPU == nil || got.CPU.TotalCores != 8 || got.CPU.TotalThreads != 16 {
		t.Errorf("CPU = %+v, want TotalCores=8 TotalThreads=16", got.CPU)
	}
	if len(got.CPU.Sockets) != 1 || got.CPU.Sockets[0].Model != "i7" || len(got.CPU.Sockets[0].Capabilities) != 2 {
		t.Errorf("CPU.Sockets = %+v, want one entry with Model=i7 and 2 capabilities", got.CPU.Sockets)
	}
	if got.Memory != nil {
		t.Errorf("Memory = %+v, want nil (no exact byte count available from the Agent)", got.Memory)
	}

	if len(got.BlockDevices) != 1 || got.BlockDevices[0].Name != "nvme0n1" || got.BlockDevices[0].SerialNumber != "S1" {
		t.Errorf("BlockDevices = %+v, want one entry with Name=nvme0n1 SerialNumber=S1", got.BlockDevices)
	}
	if got.BlockDevices[0].SizeBytes != 0 {
		t.Errorf("BlockDevices[0].SizeBytes = %d, want 0 (no exact byte count available from the Agent)", got.BlockDevices[0].SizeBytes)
	}

	if len(got.NetworkInterfaces) != 1 {
		t.Fatalf("NetworkInterfaces = %+v, want one entry", got.NetworkInterfaces)
	}
	nic := got.NetworkInterfaces[0]
	if nic.Name != "eno1" || len(nic.Ports) != 1 {
		t.Fatalf("NetworkInterfaces[0] = %+v, want Name=eno1 with one synthesized port", nic)
	}
	if nic.Ports[0].MAC != "aa:bb:cc:dd:ee:ff" || nic.Ports[0].SpeedMbps != 1000 {
		t.Errorf("NetworkInterfaces[0].Ports[0] = %+v, want MAC=aa:bb:cc:dd:ee:ff SpeedMbps=1000", nic.Ports[0])
	}
	if len(nic.Ports[0].EnabledCapabilities) != 1 || nic.Ports[0].EnabledCapabilities[0] != "tso" {
		t.Errorf("NetworkInterfaces[0].Ports[0].EnabledCapabilities = %v, want [tso]", nic.Ports[0].EnabledCapabilities)
	}

	if len(got.PCIDevices) != 1 || got.PCIDevices[0].Model != "Ethernet Controller" {
		t.Errorf("PCIDevices = %+v, want one entry with Model=Ethernet Controller (mapped from PCI.Product)", got.PCIDevices)
	}
	if len(got.GPUDevices) != 1 || got.GPUDevices[0].Model != "A100" {
		t.Errorf("GPUDevices = %+v, want one entry with Model=A100 (mapped from GPU.Product)", got.GPUDevices)
	}

	if got.Chassis == nil || got.Chassis.Vendor != "Dell" || got.Chassis.SerialNumber != "CH1" {
		t.Errorf("Chassis = %+v, want Vendor=Dell SerialNumber=CH1", got.Chassis)
	}
	if got.BIOS == nil || got.BIOS.FirmwareVersion != "2.10.2" || got.BIOS.ReleaseDate != "01/01/2024" {
		t.Errorf("BIOS = %+v, want FirmwareVersion=2.10.2 ReleaseDate=01/01/2024 (mapped from BIOS.Version)", got.BIOS)
	}
	if got.Baseboard == nil || got.Baseboard.Model != "0ABC123" || got.Baseboard.FirmwareVersion != "A01" {
		t.Errorf("Baseboard = %+v, want Model=0ABC123 FirmwareVersion=A01 (mapped from Baseboard.Product/Version)", got.Baseboard)
	}
	if got.Product == nil || got.Product.Name != "PowerEdge R750" || got.Product.SerialNumber != "SN1" {
		t.Errorf("Product = %+v, want Name=PowerEdge R750 SerialNumber=SN1", got.Product)
	}

	// CollectionMethod and LastUpdated aren't Agent-collected fields; the caller
	// (updateHardwareWithAttributes) sets them, not this mapping function.
	if got.CollectionMethod != "" {
		t.Errorf("CollectionMethod = %q, want empty (set by the caller)", got.CollectionMethod)
	}
	if got.LastUpdated != nil {
		t.Errorf("LastUpdated = %v, want nil (set by the caller)", got.LastUpdated)
	}
}

func TestInBandAttributesFromAgentEmpty(t *testing.T) {
	// An AgentAttributes with no sub-structs set at all must not panic.
	got := inBandAttributesFromAgent(&data.AgentAttributes{})
	if got == nil {
		t.Fatal("inBandAttributesFromAgent() = nil, want populated")
	}
	if got.CPU != nil || got.Chassis != nil || got.BIOS != nil || got.Baseboard != nil || got.Product != nil {
		t.Errorf("got = %+v, want all optional subtrees nil", got)
	}
}

func TestParseSpeedMbps(t *testing.T) {
	tests := map[string]uint32{
		"1000":     1000,
		"1000Mb/s": 1000,
		"25000":    25000,
		"":         0,
		"unknown":  0,
		"10 Gb/s":  10,
	}
	for input, want := range tests {
		if got := parseSpeedMbps(input); got != want {
			t.Errorf("parseSpeedMbps(%q) = %d, want %d", input, got, want)
		}
	}
}
