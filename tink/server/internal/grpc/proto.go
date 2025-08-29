package grpc

import (
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
)

func convert(pAttr *proto.AgentAttributes) *data.AgentAttributes {
	if pAttr == nil {
		return nil
	}
	dAttr := data.NewAgentAttributes()
	// CPU
	if pAttr.Cpu != nil {
		dAttr.CPU.TotalCores = pAttr.Cpu.TotalCores
		dAttr.CPU.TotalThreads = pAttr.Cpu.TotalThreads
		for _, core := range pAttr.Cpu.Processors {
			dAttr.CPU.Processors = append(dAttr.CPU.Processors, &data.Processor{
				ID:           core.Id,
				Cores:        core.Cores,
				Threads:      core.Threads,
				Vendor:       core.Vendor,
				Model:        core.Model,
				Capabilities: core.Capabilities,
			})
		}
	}
	// Memory
	if pAttr.Memory != nil {
		dAttr.Memory.Total = pAttr.Memory.Total
		dAttr.Memory.Usable = pAttr.Memory.Usable
	}
	// BlockDevices
	for _, block := range pAttr.Block {
		dAttr.BlockDevices = append(dAttr.BlockDevices, &data.Block{
			Name:              block.Name,
			ControllerType:    block.ControllerType,
			DriveType:         block.DriveType,
			Size:              block.Size,
			PhysicalBlockSize: block.PhysicalBlockSize,
			Vendor:            block.Vendor,
			Model:             block.Model,
			WWN:               block.Wwn,
			SerialNumber:      block.SerialNumber,
		})
	}
	// NetworkInterfaces
	for _, network := range pAttr.Network {
		dAttr.NetworkInterfaces = append(dAttr.NetworkInterfaces, &data.Network{
			Name:                network.Name,
			Mac:                 network.Mac,
			Speed:               network.Speed,
			EnabledCapabilities: network.EnabledCapabilities,
		})
	}
	// PCIDevices
	for _, pci := range pAttr.Pci {
		dAttr.PCIDevices = append(dAttr.PCIDevices, &data.PCI{
			Vendor:  pci.Vendor,
			Product: pci.Product,
			Class:   pci.Class,
			Driver:  pci.Driver,
		})
	}
	// GPUDevices
	for _, gpu := range pAttr.Gpu {
		dAttr.GPUDevices = append(dAttr.GPUDevices, &data.GPU{
			Vendor:  gpu.Vendor,
			Product: gpu.Product,
			Class:   gpu.Class,
			Driver:  gpu.Driver,
		})
	}
	// Chassis
	if pAttr.Chassis != nil {
		dAttr.Chassis.Serial = pAttr.Chassis.Serial
		dAttr.Chassis.Vendor = pAttr.Chassis.Vendor
	}
	// BIOS
	if pAttr.Bios != nil {
		dAttr.BIOS.Vendor = pAttr.Bios.Vendor
		dAttr.BIOS.Version = pAttr.Bios.Version
		dAttr.BIOS.ReleaseDate = pAttr.Bios.ReleaseDate
	}
	// Baseboard
	if pAttr.Baseboard != nil {
		dAttr.Baseboard.Vendor = pAttr.Baseboard.Vendor
		dAttr.Baseboard.Product = pAttr.Baseboard.Product
		dAttr.Baseboard.Version = pAttr.Baseboard.Version
		dAttr.Baseboard.SerialNumber = pAttr.Baseboard.SerialNumber
	}
	// Product
	if pAttr.Product != nil {
		dAttr.Product.Name = pAttr.Product.Name
		dAttr.Product.Vendor = pAttr.Product.Vendor
		dAttr.Product.SerialNumber = pAttr.Product.SerialNumber
	}

	return dAttr
}
