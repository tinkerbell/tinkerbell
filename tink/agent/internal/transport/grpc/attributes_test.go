package grpc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestToProto(t *testing.T) {
	tests := map[string]struct {
		input    *data.AgentAttributes
		expected *proto.AgentAttributes
	}{
		"Nil input": {
			input:    nil,
			expected: nil,
		},
		"Fully populated input": {
			input: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(8)),
					TotalThreads: toPtr(uint32(16)),
					Processors: []*data.Processor{
						{
							ID:           toPtr(uint32(1)),
							Cores:        toPtr(uint32(4)),
							Threads:      toPtr(uint32(8)),
							Vendor:       toPtr("Intel"),
							Model:        toPtr("i7"),
							Capabilities: []string{"sse4.2", "avx2"},
						},
					},
				},
				Memory: &data.Memory{
					Total:  toPtr("16KB"),
					Usable: toPtr("8KB"),
				},
				BlockDevices: []*data.Block{
					{
						Name:              toPtr("sda"),
						ControllerType:    toPtr("SATA"),
						DriveType:         toPtr("SSD"),
						Size:              toPtr("512B"),
						PhysicalBlockSize: toPtr("4KB"),
						Vendor:            toPtr("Samsung"),
						Model:             toPtr("EVO860"),
					},
				},
				NetworkInterfaces: []*data.Network{
					{
						Name:                toPtr("eth0"),
						Mac:                 toPtr("00:1A:2B:3C:4D:5E"),
						Speed:               toPtr("1Gbps"),
						EnabledCapabilities: []string{"rx-checksum", "tx-checksum"},
					},
				},
			},
			expected: &proto.AgentAttributes{
				Cpu: &proto.CPU{
					TotalCores:   toPtr(uint32(8)),
					TotalThreads: toPtr(uint32(16)),
					Processors: []*proto.Processor{
						{
							Id:           toPtr(uint32(1)),
							Cores:        toPtr(uint32(4)),
							Threads:      toPtr(uint32(8)),
							Vendor:       toPtr("Intel"),
							Model:        toPtr("i7"),
							Capabilities: []string{"sse4.2", "avx2"},
						},
					},
				},
				Memory: &proto.Memory{
					Total:  toPtr("16KB"),
					Usable: toPtr("8KB"),
				},
				Block: []*proto.Block{
					{
						Name:              toPtr("sda"),
						ControllerType:    toPtr("SATA"),
						DriveType:         toPtr("SSD"),
						Size:              toPtr("512B"),
						PhysicalBlockSize: toPtr("4KB"),
						Vendor:            toPtr("Samsung"),
						Model:             toPtr("EVO860"),
					},
				},
				Network: []*proto.Network{
					{
						Name:                toPtr("eth0"),
						Mac:                 toPtr("00:1A:2B:3C:4D:5E"),
						Speed:               toPtr("1Gbps"),
						EnabledCapabilities: []string{"rx-checksum", "tx-checksum"},
					},
				},
			},
		},
		"Partially populated input (only CPU)": {
			input: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
				},
			},
			expected: &proto.AgentAttributes{
				Cpu: &proto.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
				},
			},
		},
		"All fields populated": {
			input: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
					Processors: []*data.Processor{
						{
							ID:           toPtr(uint32(1)),
							Cores:        toPtr(uint32(4)),
							Threads:      toPtr(uint32(8)),
							Vendor:       toPtr("Intel"),
							Model:        toPtr("i7"),
							Capabilities: []string{"sse4.2", "avx2"},
						},
					},
				},
				Memory: &data.Memory{
					Total:  toPtr("16KB"),
					Usable: toPtr("8KB"),
				},
				BlockDevices: []*data.Block{
					{
						Name:              toPtr("sda"),
						ControllerType:    toPtr("SATA"),
						DriveType:         toPtr("SSD"),
						Size:              toPtr("512B"),
						PhysicalBlockSize: toPtr("4KB"),
						Vendor:            toPtr("Samsung"),
						Model:             toPtr("EVO860"),
					},
				},
				NetworkInterfaces: []*data.Network{
					{
						Name:                toPtr("eth0"),
						Mac:                 toPtr("00:1A:2B:3C:4D:5E"),
						Speed:               toPtr("1Gbps"),
						EnabledCapabilities: []string{"rx-checksum", "tx-checksum"},
					},
				},
				PCIDevices: []*data.PCI{
					{
						Vendor:  toPtr("Intel"),
						Product: toPtr("Ethernet"),
						Class:   toPtr("Ethernet controller"),
						Driver:  toPtr("e1000e"),
					},
				},
				GPUDevices: []*data.GPU{
					{
						Vendor:  toPtr("NVIDIA"),
						Product: toPtr("GeForce RTX 2080"),
						Class:   toPtr("VGA controller"),
						Driver:  toPtr("nouveau"),
					},
				},
				Chassis: &data.Chassis{
					Serial: toPtr("123456"),
					Vendor: toPtr("HP"),
				},
				BIOS: &data.BIOS{
					Vendor:  toPtr("American Megatrends Inc."),
					Version: toPtr("F.42"),
				},
				Product: &data.Product{
					Name:   toPtr("HP EliteDesk 800 G5"),
					Vendor: toPtr("HP"),
				},
				Baseboard: &data.Baseboard{
					Vendor:  toPtr("HP"),
					Product: toPtr("8606"),
					Version: toPtr("1.0"),
				},
			},
			expected: &proto.AgentAttributes{
				Cpu: &proto.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
					Processors: []*proto.Processor{
						{
							Id:           toPtr(uint32(1)),
							Cores:        toPtr(uint32(4)),
							Threads:      toPtr(uint32(8)),
							Vendor:       toPtr("Intel"),
							Model:        toPtr("i7"),
							Capabilities: []string{"sse4.2", "avx2"},
						},
					},
				},
				Memory: &proto.Memory{
					Total:  toPtr("16KB"),
					Usable: toPtr("8KB"),
				},
				Block: []*proto.Block{
					{
						Name:              toPtr("sda"),
						ControllerType:    toPtr("SATA"),
						DriveType:         toPtr("SSD"),
						Size:              toPtr("512B"),
						PhysicalBlockSize: toPtr("4KB"),
						Vendor:            toPtr("Samsung"),
						Model:             toPtr("EVO860"),
					},
				},
				Network: []*proto.Network{
					{
						Name:                toPtr("eth0"),
						Mac:                 toPtr("00:1A:2B:3C:4D:5E"),
						Speed:               toPtr("1Gbps"),
						EnabledCapabilities: []string{"rx-checksum", "tx-checksum"},
					},
				},
				Pci: []*proto.PCI{
					{
						Vendor:  toPtr("Intel"),
						Product: toPtr("Ethernet"),
						Class:   toPtr("Ethernet controller"),
						Driver:  toPtr("e1000e"),
					},
				},
				Gpu: []*proto.GPU{
					{
						Vendor:  toPtr("NVIDIA"),
						Product: toPtr("GeForce RTX 2080"),
						Class:   toPtr("VGA controller"),
						Driver:  toPtr("nouveau"),
					},
				},
				Chassis: &proto.Chassis{
					Serial: toPtr("123456"),
					Vendor: toPtr("HP"),
				},
				Bios: &proto.BIOS{
					Vendor:  toPtr("American Megatrends Inc."),
					Version: toPtr("F.42"),
				},
				Product: &proto.Product{
					Name:   toPtr("HP EliteDesk 800 G5"),
					Vendor: toPtr("HP"),
				},
				Baseboard: &proto.Baseboard{
					Vendor:  toPtr("HP"),
					Product: toPtr("8606"),
					Version: toPtr("1.0"),
				},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := ToProto(tt.input)
			if diff := cmp.Diff(result, tt.expected, protocmp.Transform()); diff != "" {
				t.Errorf("ToProto() = %v", diff)
			}
		})
	}
}
