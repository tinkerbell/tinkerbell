package templates

import (
	"testing"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestAgentAttributesFromInBandNil(t *testing.T) {
	if got := AgentAttributesFromInBand(nil); got != nil {
		t.Errorf("AgentAttributesFromInBand(nil) = %+v, want nil", got)
	}
}

func TestAgentAttributesFromInBandEmpty(t *testing.T) {
	got := AgentAttributesFromInBand(&tinkv1alpha1.Attributes{})
	if got == nil {
		t.Fatal("AgentAttributesFromInBand() = nil, want populated")
	}
	if got.CPU.TotalCores != 0 || len(got.CPU.Processors) != 0 {
		t.Errorf("CPU = %+v, want zero value", got.CPU)
	}
	if got.Memory.Total != "" || got.Memory.Usable != "" {
		t.Errorf("Memory = %+v, want empty strings", got.Memory)
	}
}

func TestAgentAttributesFromInBand(t *testing.T) {
	attrs := &tinkv1alpha1.Attributes{
		CPU: &tinkv1alpha1.CPU{
			TotalCores:   8,
			TotalThreads: 16,
			Sockets: []tinkv1alpha1.CPUSocket{
				{Vendor: "GenuineIntel", Model: "i7", Cores: 8, Threads: 16, Capabilities: []string{"fpu", "vme"}},
			},
		},
		Memory: &tinkv1alpha1.Memory{
			TotalBytes:  8_000_000_000,
			UsableBytes: 7_500_000_000,
		},
		BlockDevices: []tinkv1alpha1.BlockDevice{
			{Name: "nvme0n1", Vendor: "Samsung", SerialNumber: "S1", SizeBytes: 20_000_000_000},
		},
		NetworkInterfaces: []tinkv1alpha1.NetworkInterface{
			{
				Name: "eno1",
				Ports: []tinkv1alpha1.NetworkPort{
					{MAC: "aa:bb:cc:dd:ee:ff", SpeedMbps: 100, EnabledCapabilities: []string{"tso"}},
				},
			},
			{
				Name: "eno2",
				Ports: []tinkv1alpha1.NetworkPort{
					{MAC: "aa:bb:cc:dd:ee:00", SpeedMbps: 25000},
				},
			},
		},
		PCIDevices: []tinkv1alpha1.PCIDevice{
			{Vendor: "Intel", Model: "Ethernet Controller", Class: "0200", Driver: "ixgbe"},
		},
		GPUDevices: []tinkv1alpha1.GPUDevice{
			{Vendor: "NVIDIA", Model: "A100", Class: "0300", Driver: "nvidia"},
		},
		Chassis:   &tinkv1alpha1.Chassis{Vendor: "Dell", SerialNumber: "CH1"},
		BIOS:      &tinkv1alpha1.BIOS{Vendor: "Dell Inc.", FirmwareVersion: "2.10.2", ReleaseDate: "01/01/2024"},
		Baseboard: &tinkv1alpha1.Baseboard{Vendor: "Dell", Model: "0ABC123", FirmwareVersion: "A01", SerialNumber: "BB1"},
		Product:   &tinkv1alpha1.Product{Name: "PowerEdge R750", Vendor: "Dell Inc.", SerialNumber: "SN1"},
	}

	got := AgentAttributesFromInBand(attrs)
	if got == nil {
		t.Fatal("AgentAttributesFromInBand() = nil, want populated")
	}

	if got.CPU.TotalCores != 8 || got.CPU.TotalThreads != 16 {
		t.Errorf("CPU = %+v, want TotalCores=8 TotalThreads=16", got.CPU)
	}
	if len(got.CPU.Processors) != 1 || got.CPU.Processors[0].Model != "i7" || len(got.CPU.Processors[0].Capabilities) != 2 {
		t.Errorf("CPU.Processors = %+v, want one entry with Model=i7 and 2 capabilities", got.CPU.Processors)
	}

	if got.Memory.Total != "8GB" || got.Memory.Usable != "7.5GB" {
		t.Errorf("Memory = %+v, want Total=8GB Usable=7.5GB", got.Memory)
	}

	if len(got.BlockDevices) != 1 || got.BlockDevices[0].Name != "nvme0n1" || got.BlockDevices[0].Size != "20GB" {
		t.Errorf("BlockDevices = %+v, want one entry with Name=nvme0n1 Size=20GB", got.BlockDevices)
	}

	if len(got.NetworkInterfaces) != 2 {
		t.Fatalf("NetworkInterfaces = %+v, want two entries", got.NetworkInterfaces)
	}
	if got.NetworkInterfaces[0].MAC != "aa:bb:cc:dd:ee:ff" || got.NetworkInterfaces[0].Speed != "100 Mbps" {
		t.Errorf("NetworkInterfaces[0] = %+v, want MAC=aa:bb:cc:dd:ee:ff Speed=\"100 Mbps\"", got.NetworkInterfaces[0])
	}
	if len(got.NetworkInterfaces[0].EnabledCapabilities) != 1 || got.NetworkInterfaces[0].EnabledCapabilities[0] != "tso" {
		t.Errorf("NetworkInterfaces[0].EnabledCapabilities = %v, want [tso]", got.NetworkInterfaces[0].EnabledCapabilities)
	}
	if got.NetworkInterfaces[1].Speed != "25 Gbps" {
		t.Errorf("NetworkInterfaces[1].Speed = %q, want \"25 Gbps\"", got.NetworkInterfaces[1].Speed)
	}

	if len(got.PCIDevices) != 1 || got.PCIDevices[0].Product != "Ethernet Controller" {
		t.Errorf("PCIDevices = %+v, want one entry with Product=Ethernet Controller (mapped from PCIDevice.Model)", got.PCIDevices)
	}
	if len(got.GPUDevices) != 1 || got.GPUDevices[0].Product != "A100" {
		t.Errorf("GPUDevices = %+v, want one entry with Product=A100 (mapped from GPUDevice.Model)", got.GPUDevices)
	}

	if got.Chassis.Vendor != "Dell" || got.Chassis.Serial != "CH1" {
		t.Errorf("Chassis = %+v, want Vendor=Dell Serial=CH1", got.Chassis)
	}
	if got.BIOS.Vendor != "Dell Inc." || got.BIOS.Version != "2.10.2" || got.BIOS.ReleaseDate != "01/01/2024" {
		t.Errorf("BIOS = %+v, want Vendor=Dell Inc. Version=2.10.2 ReleaseDate=01/01/2024", got.BIOS)
	}
	if got.Baseboard.Product != "0ABC123" || got.Baseboard.Version != "A01" {
		t.Errorf("Baseboard = %+v, want Product=0ABC123 Version=A01 (mapped from Baseboard.Model/FirmwareVersion)", got.Baseboard)
	}
	if got.Product.Name != "PowerEdge R750" || got.Product.SerialNumber != "SN1" {
		t.Errorf("Product = %+v, want Name=PowerEdge R750 SerialNumber=SN1", got.Product)
	}
}

func TestHumanizeBytes(t *testing.T) {
	tests := map[int64]string{
		0:              "",
		-1:             "",
		999:            "999B",
		8_000_000_000:  "8GB",
		7_500_000_000:  "7.5GB",
		20_000_000_000: "20GB",
	}
	for input, want := range tests {
		if got := humanizeBytes(input); got != want {
			t.Errorf("humanizeBytes(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestHumanizeSpeedMbps(t *testing.T) {
	tests := map[uint32]string{
		0:     "",
		1:     "1 Mbps",
		999:   "999 Mbps",
		1000:  "1 Gbps",
		25000: "25 Gbps",
		1500:  "1.5 Gbps",
	}
	for input, want := range tests {
		if got := humanizeSpeedMbps(input); got != want {
			t.Errorf("humanizeSpeedMbps(%d) = %q, want %q", input, got, want)
		}
	}
}
