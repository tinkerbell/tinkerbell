package grpc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
)

func TestConvert(t *testing.T) {
	tests := map[string]struct {
		input *proto.AgentAttributes
		want  *data.AgentAttributes
	}{
		"nil input": {
			input: nil,
			want:  nil,
		},
		"empty input": {
			input: &proto.AgentAttributes{},
			want:  data.NewAgentAttributes(),
		},
		"complete input": {
			input: &proto.AgentAttributes{
				Cpu: &proto.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
					Processors: []*proto.Processor{
						{
							Id:           toPtr(uint32(0)),
							Cores:        toPtr(uint32(4)),
							Threads:      toPtr(uint32(8)),
							Vendor:       toPtr("GenuineIntel"),
							Model:        toPtr("11th Gen Intel(R) Core(TM) i5-1145G7E @ 2.60GHz"),
							Capabilities: []string{"fpu"},
						},
					},
				},
				Memory: &proto.Memory{
					Total:  toPtr("32GB"),
					Usable: toPtr("31GB"),
				},
				Block: []*proto.Block{
					{
						Name:              toPtr("nvme0n1"),
						ControllerType:    toPtr("NVMe"),
						DriveType:         toPtr("SSD"),
						Size:              toPtr("239GB"),
						PhysicalBlockSize: toPtr("512B"),
						Vendor:            toPtr("unknown"),
						Model:             toPtr("KINGSTON ABCDEF-01"),
						Wwn:               toPtr("18b6db91-d83b-4172-97d4-27c38550cce6"),
					},
				},
				Network: []*proto.Network{
					{
						Name:                toPtr("eno1"),
						Mac:                 toPtr("de:ad:be:ef:00:00"),
						Speed:               toPtr("1000Mb/s"),
						EnabledCapabilities: []string{"auto-negotiation"},
					},
				},
				Pci: []*proto.PCI{
					{
						Vendor:  toPtr("Intel Corporation"),
						Product: toPtr("11th Gen Core Processor PCIe Controller"),
						Class:   toPtr("Bridge"),
						Driver:  toPtr("pcieport"),
					},
				},
				Gpu: []*proto.GPU{
					{
						Vendor:  toPtr("NVIDIA Corporation"),
						Product: toPtr("GP107 [GeForce GTX 1050 Ti]"),
						Class:   toPtr("Display controller"),
						Driver:  toPtr("GP107"),
					},
				},
				Chassis: &proto.Chassis{
					Serial: toPtr("123456"),
					Vendor: toPtr("dell"),
				},
				Bios: &proto.BIOS{
					Vendor:      toPtr("American Megatrends International, LLC."),
					Version:     toPtr("11.2233"),
					ReleaseDate: toPtr("12/13/2021"),
				},
				Baseboard: &proto.Baseboard{
					Vendor:       toPtr("example vendor"),
					Product:      toPtr("ABC-DEF"),
					Version:      toPtr("1234"),
					SerialNumber: toPtr("123-456-789"),
				},
				Product: &proto.Product{
					Name:         toPtr("abcd123"),
					Vendor:       toPtr("example vendor"),
					SerialNumber: toPtr("xyz-123"),
				},
			},
			want: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
					Processors: []*data.Processor{
						{
							ID:           toPtr(uint32(0)),
							Cores:        toPtr(uint32(4)),
							Threads:      toPtr(uint32(8)),
							Vendor:       toPtr("GenuineIntel"),
							Model:        toPtr("11th Gen Intel(R) Core(TM) i5-1145G7E @ 2.60GHz"),
							Capabilities: []string{"fpu"},
						},
					},
				},
				Memory: &data.Memory{
					Total:  toPtr("32GB"),
					Usable: toPtr("31GB"),
				},
				BlockDevices: []*data.Block{
					{
						Name:              toPtr("nvme0n1"),
						ControllerType:    toPtr("NVMe"),
						DriveType:         toPtr("SSD"),
						Size:              toPtr("239GB"),
						PhysicalBlockSize: toPtr("512B"),
						Vendor:            toPtr("unknown"),
						Model:             toPtr("KINGSTON ABCDEF-01"),
						Wwn:               toPtr("18b6db91-d83b-4172-97d4-27c38550cce6"),
					},
				},
				NetworkInterfaces: []*data.Network{
					{
						Name:                toPtr("eno1"),
						Mac:                 toPtr("de:ad:be:ef:00:00"),
						Speed:               toPtr("1000Mb/s"),
						EnabledCapabilities: []string{"auto-negotiation"},
					},
				},
				PCIDevices: []*data.PCI{
					{
						Vendor:  toPtr("Intel Corporation"),
						Product: toPtr("11th Gen Core Processor PCIe Controller"),
						Class:   toPtr("Bridge"),
						Driver:  toPtr("pcieport"),
					},
				},
				GPUDevices: []*data.GPU{
					{
						Vendor:  toPtr("NVIDIA Corporation"),
						Product: toPtr("GP107 [GeForce GTX 1050 Ti]"),
						Class:   toPtr("Display controller"),
						Driver:  toPtr("GP107"),
					},
				},
				Chassis: &data.Chassis{
					Serial: toPtr("123456"),
					Vendor: toPtr("dell"),
				},
				BIOS: &data.BIOS{
					Vendor:      toPtr("American Megatrends International, LLC."),
					Version:     toPtr("11.2233"),
					ReleaseDate: toPtr("12/13/2021"),
				},
				Baseboard: &data.Baseboard{
					Vendor:       toPtr("example vendor"),
					Product:      toPtr("ABC-DEF"),
					Version:      toPtr("1234"),
					SerialNumber: toPtr("123-456-789"),
				},
				Product: &data.Product{
					Name:         toPtr("abcd123"),
					Vendor:       toPtr("example vendor"),
					SerialNumber: toPtr("xyz-123"),
				},
			},
		},
		"partial input - only CPU": {
			input: &proto.AgentAttributes{
				Cpu: &proto.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
					Processors: []*proto.Processor{
						{
							Id:           toPtr(uint32(0)),
							Cores:        toPtr(uint32(4)),
							Threads:      toPtr(uint32(8)),
							Vendor:       toPtr("GenuineIntel"),
							Model:        toPtr("11th Gen Intel(R) Core(TM) i5-1145G7E @ 2.60GHz"),
							Capabilities: []string{"fpu"},
						},
					},
				},
			},
			want: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
					Processors: []*data.Processor{
						{
							ID:           toPtr(uint32(0)),
							Cores:        toPtr(uint32(4)),
							Threads:      toPtr(uint32(8)),
							Vendor:       toPtr("GenuineIntel"),
							Model:        toPtr("11th Gen Intel(R) Core(TM) i5-1145G7E @ 2.60GHz"),
							Capabilities: []string{"fpu"},
						},
					},
				},
				Memory:            &data.Memory{},
				BlockDevices:      []*data.Block{},
				NetworkInterfaces: []*data.Network{},
				PCIDevices:        []*data.PCI{},
				GPUDevices:        []*data.GPU{},
				Chassis:           &data.Chassis{},
				BIOS:              &data.BIOS{},
				Baseboard:         &data.Baseboard{},
				Product:           &data.Product{},
			},
		},
		"partial input - only Memory": {
			input: &proto.AgentAttributes{
				Memory: &proto.Memory{
					Total:  toPtr("32GB"),
					Usable: toPtr("31GB"),
				},
			},
			want: &data.AgentAttributes{
				CPU: &data.CPU{},
				Memory: &data.Memory{
					Total:  toPtr("32GB"),
					Usable: toPtr("31GB"),
				},
				BlockDevices:      []*data.Block{},
				NetworkInterfaces: []*data.Network{},
				PCIDevices:        []*data.PCI{},
				GPUDevices:        []*data.GPU{},
				Chassis:           &data.Chassis{},
				BIOS:              &data.BIOS{},
				Baseboard:         &data.Baseboard{},
				Product:           &data.Product{},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := convert(tc.input)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("convert() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
