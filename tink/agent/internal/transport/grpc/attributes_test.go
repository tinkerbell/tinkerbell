package grpc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/attribute"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestToProto(t *testing.T) {
	tests := map[string]struct {
		input    *attribute.AllAttributes
		expected *proto.WorkerAttributes
	}{
		"Nil input": {
			input:    nil,
			expected: nil,
		},
		"Fully populated input": {
			input: &attribute.AllAttributes{
				CPU: &attribute.CPU{
					TotalCores:   toPtr(uint32(8)),
					TotalThreads: toPtr(uint32(16)),
					Processors: []*attribute.Processor{
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
				Memory: &attribute.Memory{
					Total:  toPtr("16KB"),
					Usable: toPtr("8KB"),
				},
				BlockDevices: []*attribute.Block{
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
				NetworkInterfaces: []*attribute.Network{
					{
						Name:                toPtr("eth0"),
						Mac:                 toPtr("00:1A:2B:3C:4D:5E"),
						Speed:               toPtr("1Gbps"),
						EnabledCapabilities: []string{"rx-checksum", "tx-checksum"},
					},
				},
			},
			expected: &proto.WorkerAttributes{
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
			input: &attribute.AllAttributes{
				CPU: &attribute.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
				},
			},
			expected: &proto.WorkerAttributes{
				Cpu: &proto.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
				},
			},
		},
		"All fields populated": {
			input: &attribute.AllAttributes{
				CPU: &attribute.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
					Processors: []*attribute.Processor{
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
				Memory: &attribute.Memory{
					Total:  toPtr("16KB"),
					Usable: toPtr("8KB"),
				},
				BlockDevices: []*attribute.Block{
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
				NetworkInterfaces: []*attribute.Network{
					{
						Name:                toPtr("eth0"),
						Mac:                 toPtr("00:1A:2B:3C:4D:5E"),
						Speed:               toPtr("1Gbps"),
						EnabledCapabilities: []string{"rx-checksum", "tx-checksum"},
					},
				},
				PCIDevices: []*attribute.PCI{
					{
						Vendor:  toPtr("Intel"),
						Product: toPtr("Ethernet"),
						Class:   toPtr("Ethernet controller"),
						Driver:  toPtr("e1000e"),
					},
				},
				GPUDevices: []*attribute.GPU{
					{
						Vendor:  toPtr("NVIDIA"),
						Product: toPtr("GeForce RTX 2080"),
						Class:   toPtr("VGA controller"),
						Driver:  toPtr("nouveau"),
					},
				},
				Chassis: &attribute.Chassis{
					Serial: toPtr("123456"),
					Vendor: toPtr("HP"),
				},
				BIOS: &attribute.BIOS{
					Vendor:  toPtr("American Megatrends Inc."),
					Version: toPtr("F.42"),
				},
				Product: &attribute.Product{
					Name:   toPtr("HP EliteDesk 800 G5"),
					Vendor: toPtr("HP"),
				},
				Baseboard: &attribute.Baseboard{
					Vendor:  toPtr("HP"),
					Product: toPtr("8606"),
					Version: toPtr("1.0"),
				},
			},
			expected: &proto.WorkerAttributes{
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
