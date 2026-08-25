package templates

import (
	"testing"

	"github.com/tinkerbell/tinkerbell/pkg/data"
)

func ptr[T any](v T) *T { return &v }

func TestAgentAttributesFromDataNil(t *testing.T) {
	if got := AgentAttributesFromData(nil); got != nil {
		t.Errorf("AgentAttributesFromData(nil) = %+v, want nil", got)
	}
}

func TestAgentAttributesFromData(t *testing.T) {
	attrs := &data.AgentAttributes{
		CPU: &data.CPU{
			TotalCores:   ptr(uint32(8)),
			TotalThreads: ptr(uint32(16)),
			Processors: []*data.Processor{
				{Vendor: ptr("GenuineIntel"), Model: ptr("i7"), Cores: ptr(uint32(8)), Threads: ptr(uint32(16)), Capabilities: []string{"fpu"}},
			},
		},
		Memory: &data.Memory{
			TotalBytes:  ptr(int64(34_359_738_368)), // 32 GiB
			UsableBytes: ptr(int64(30_064_771_072)), // 28 GiB
		},
		BlockDevices: []*data.Block{
			{Name: ptr("sda"), SizeBytes: ptr(int64(536_870_912_000)), PhysicalBlockSizeBytes: ptr(int64(512))}, // 500 GiB
		},
		NetworkInterfaces: []*data.Network{
			{Name: ptr("eno1"), Mac: ptr("aa:bb:cc:dd:ee:ff"), SpeedMbps: ptr(uint32(1000)), EnabledCapabilities: []string{"tso"}},
		},
		PCIDevices: []*data.PCI{
			{Vendor: ptr("Intel"), Product: ptr("Ethernet Controller")},
		},
		GPUDevices: []*data.GPU{
			{Vendor: ptr("NVIDIA"), Product: ptr("A100")},
		},
		Chassis:   &data.Chassis{Vendor: ptr("Dell"), Serial: ptr("CH1")},
		BIOS:      &data.BIOS{Vendor: ptr("Dell Inc.")},
		Baseboard: &data.Baseboard{Vendor: ptr("Dell")},
		Product:   &data.Product{Name: ptr("PowerEdge R750")},
	}

	got := AgentAttributesFromData(attrs)
	if got == nil {
		t.Fatal("AgentAttributesFromData() = nil, want populated")
	}
	if got.CPU.TotalCores != 8 || got.CPU.TotalThreads != 16 {
		t.Errorf("CPU = %+v, want TotalCores=8 TotalThreads=16", got.CPU)
	}
	if got.Memory.Total != "32 GiB" || got.Memory.Usable != "28 GiB" {
		t.Errorf("Memory = %+v, want Total=\"32 GiB\" Usable=\"28 GiB\"", got.Memory)
	}
	if len(got.BlockDevices) != 1 || got.BlockDevices[0].Size != "500 GiB" || got.BlockDevices[0].PhysicalBlockSize != "512 B" {
		t.Errorf("BlockDevices = %+v, want one entry Size=\"500 GiB\" PhysicalBlockSize=\"512 B\"", got.BlockDevices)
	}
	if len(got.NetworkInterfaces) != 1 || got.NetworkInterfaces[0].Speed != "1 Gbps" {
		t.Errorf("NetworkInterfaces = %+v, want one entry Speed=\"1 Gbps\"", got.NetworkInterfaces)
	}
}

func TestHumanizeBytes(t *testing.T) {
	tests := map[int64]string{
		0:              "",
		-5:             "",
		512:            "512 B",
		34_359_738_368: "32 GiB", // a real 32 GiB DIMM must render as a round number, not "34.4GB"
		1_572_864:      "1.5 MiB",
	}
	for input, want := range tests {
		if got := humanizeBytes(input); got != want {
			t.Errorf("humanizeBytes(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestHumanizeSpeedMbps(t *testing.T) {
	tests := map[uint32]string{
		0:      "",
		100:    "100 Mbps",
		1000:   "1 Gbps",
		2500:   "2.5 Gbps",
		100000: "100 Gbps",
	}
	for input, want := range tests {
		if got := humanizeSpeedMbps(input); got != want {
			t.Errorf("humanizeSpeedMbps(%d) = %q, want %q", input, got, want)
		}
	}
}
