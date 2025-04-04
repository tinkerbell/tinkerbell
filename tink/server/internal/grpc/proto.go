package grpc

import (
	"fmt"

	"github.com/tinkerbell/tinkerbell/pkg/proto"
)

const labelSeparator = "-"

func flattenAttributes(attr *proto.AgentAttributes) map[string]string {
	flattened := make(map[string]string)
	if attr == nil {
		return flattened
	}
	// CPU
	if attr.Cpu != nil {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/cpu%stotalCores", labelSeparator), attr.Cpu.TotalCores)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/cpu%stotalThreads", labelSeparator), attr.Cpu.TotalThreads)
		for i, core := range attr.Cpu.Processors {
			addNonNil(flattened, fmt.Sprintf("tinkerbell.org/cpu%s%d%[1]sid", labelSeparator, i), core.Id)
			addNonNil(flattened, fmt.Sprintf("tinkerbell.org/cpu%s%d%[1]scores", labelSeparator, i), core.Cores)
			addNonNil(flattened, fmt.Sprintf("tinkerbell.org/cpu%s%d%[1]sthreads", labelSeparator, i), core.Threads)
			addNonNil(flattened, fmt.Sprintf("tinkerbell.org/cpu%s%d%[1]svendor", labelSeparator, i), core.Vendor)
			addNonNil(flattened, fmt.Sprintf("tinkerbell.org/cpu%s%d%[1]smodel", labelSeparator, i), core.Model)
		}
	}

	// Memory
	if attr.Memory != nil {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/memory%stotal", labelSeparator), attr.Memory.Total)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/memory%susable", labelSeparator), attr.Memory.Usable)
	}

	// BlockDevices
	for i, block := range attr.Block {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/block%s%d%[1]sname", labelSeparator, i), block.Name)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/block%s%d%[1]scontrollerType", labelSeparator, i), block.ControllerType)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/block%s%d%[1]sdriveType", labelSeparator, i), block.DriveType)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/block%s%d%[1]ssize", labelSeparator, i), block.Size)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/block%s%d%[1]sphysicalBlockSize", labelSeparator, i), block.PhysicalBlockSize)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/block%s%d%[1]svendor", labelSeparator, i), block.Vendor)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/block%s%d%[1]smodel", labelSeparator, i), block.Model)
	}

	// NetworkInterfaces
	for i, network := range attr.Network {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/network%s%d%[1]sname", labelSeparator, i), network.Name)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/network%s%d%[1]smac", labelSeparator, i), network.Mac)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/network%s%d%[1]sspeed", labelSeparator, i), network.Speed)
		for j, capability := range network.EnabledCapabilities {
			addNonNil(flattened, fmt.Sprintf("tinkerbell.org/network%s%d%[1]scapability%[1]s%d", labelSeparator, i, j), &capability)
		}
	}

	// PCIDevices
	for i, pci := range attr.Pci {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/pci%s%d%[1]svendor", labelSeparator, i), pci.Vendor)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/pci%s%d%[1]sproduct", labelSeparator, i), pci.Product)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/pci%s%d%[1]sclass", labelSeparator, i), pci.Class)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/pci%s%d%[1]sdriver", labelSeparator, i), pci.Driver)
	}

	// GPUDevices
	for i, gpu := range attr.Gpu {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/gpu%s%d%[1]svendor", labelSeparator, i), gpu.Vendor)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/gpu%s%d%[1]sproduct", labelSeparator, i), gpu.Product)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/gpu%s%d%[1]sclass", labelSeparator, i), gpu.Class)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/gpu%s%d%[1]sdriver", labelSeparator, i), gpu.Driver)
	}

	// Chassis
	if attr.Chassis != nil {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/chassis%sserial", labelSeparator), attr.Chassis.Serial)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/chassis%svendor", labelSeparator), attr.Chassis.Vendor)
	}

	// BIOS
	if attr.Bios != nil {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/bios%svendor", labelSeparator), attr.Bios.Vendor)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/bios%sversion", labelSeparator), attr.Bios.Version)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/bios%sreleaseDate", labelSeparator), attr.Bios.ReleaseDate)
	}

	// Baseboard
	if attr.Baseboard != nil {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/baseboard%svendor", labelSeparator), attr.Baseboard.Vendor)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/baseboard%sproduct", labelSeparator), attr.Baseboard.Product)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/baseboard%sversion", labelSeparator), attr.Baseboard.Version)
	}

	// Product
	if attr.Product != nil {
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/product%sname", labelSeparator), attr.Product.Name)
		addNonNil(flattened, fmt.Sprintf("tinkerbell.org/product%svendor", labelSeparator), attr.Product.Vendor)
	}

	return flattened
}

type anyAttribute interface {
	~string | *string | uint32 | *uint32
}

func addNonNil[a anyAttribute](m map[string]string, key string, value *a) {
	if value != nil {
		l, err := makeValidLabel(fmt.Sprintf("%v", *value))
		if err != nil {
			return
		}
		m[key] = fmt.Sprintf("%v", l)
	}
}
