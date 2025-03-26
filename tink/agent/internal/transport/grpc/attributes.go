package grpc

import (
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/attribute"
)

// ToProto converts an attribute.AllAttributes to a proto.WorkerAttributes.
func ToProto(a *attribute.AllAttributes) *proto.WorkerAttributes {
	if a == nil {
		return nil
	}
	result := &proto.WorkerAttributes{}
	if a.CPU != nil {
		result.Cpu = &proto.CPU{
			TotalCores:   a.CPU.TotalCores,
			TotalThreads: a.CPU.TotalThreads,
		}
		for _, p := range a.CPU.Processors {
			result.Cpu.Processors = append(result.Cpu.Processors, &proto.Processor{
				Id:           p.ID,
				Cores:        p.Cores,
				Threads:      p.Threads,
				Vendor:       p.Vendor,
				Model:        p.Model,
				Capabilities: p.Capabilities,
			})
		}
	}

	if a.Memory != nil {
		result.Memory = &proto.Memory{
			Total:  a.Memory.Total,
			Usable: a.Memory.Usable,
		}
	}

	for _, block := range a.BlockDevices {
		result.Block = append(result.Block, &proto.Block{
			Name:              block.Name,
			ControllerType:    block.ControllerType,
			DriveType:         block.DriveType,
			Size:              block.Size,
			PhysicalBlockSize: block.PhysicalBlockSize,
			Vendor:            block.Vendor,
			Model:             block.Model,
		})
	}

	for _, nic := range a.NetworkInterfaces {
		result.Network = append(result.Network, &proto.Network{
			Name:                nic.Name,
			Mac:                 nic.Mac,
			Speed:               nic.Speed,
			EnabledCapabilities: nic.EnabledCapabilities,
		})
	}

	for _, p := range a.PCIDevices {
		result.Pci = append(result.Pci, &proto.PCI{
			Vendor:  p.Vendor,
			Product: p.Product,
			Class:   p.Class,
			Driver:  p.Driver,
		})
	}

	for _, g := range a.GPUDevices {
		result.Gpu = append(result.Gpu, &proto.GPU{
			Vendor:  g.Vendor,
			Product: g.Product,
			Class:   g.Class,
			Driver:  g.Driver,
		})
	}

	if a.Chassis != nil {
		result.Chassis = &proto.Chassis{
			Serial: a.Chassis.Serial,
			Vendor: a.Chassis.Vendor,
		}
	}

	if a.BIOS != nil {
		result.Bios = &proto.BIOS{
			Vendor:      a.BIOS.Vendor,
			Version:     a.BIOS.Version,
			ReleaseDate: a.BIOS.ReleaseDate,
		}
	}

	if a.Baseboard != nil {
		result.Baseboard = &proto.Baseboard{
			Vendor:  a.Baseboard.Vendor,
			Product: a.Baseboard.Product,
			Version: a.Baseboard.Version,
		}
	}

	if a.Product != nil {
		result.Product = &proto.Product{
			Name:   a.Product.Name,
			Vendor: a.Product.Vendor,
		}
	}

	return result
}
