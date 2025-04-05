package grpc

import (
	"fmt"

	"github.com/tinkerbell/tinkerbell/pkg/proto"
)

const (
	labelSeparator = "-"
	labelPrefix    = "tinkerbell.org"
)

type flattenOpts struct {
	separator, prefix string
}

func flattenAttributes(attr *proto.AgentAttributes, opts *flattenOpts) map[string]string {
	flattened := make(map[string]string)
	if attr == nil {
		return flattened
	}
	if opts == nil {
		opts = &flattenOpts{
			separator: labelSeparator,
			prefix:    labelPrefix,
		}
	}
	// CPU
	if attr.Cpu != nil {
		addNonNil(flattened, fmt.Sprintf("%s/cpu%stotalCores", opts.prefix, opts.separator), attr.Cpu.TotalCores)
		addNonNil(flattened, fmt.Sprintf("%s/cpu%stotalThreads", opts.prefix, opts.separator), attr.Cpu.TotalThreads)
		for i, core := range attr.Cpu.Processors {
			addNonNil(flattened, fmt.Sprintf("%s/cpu%s%d%[2]sid", opts.prefix, opts.separator, i), core.Id)
			addNonNil(flattened, fmt.Sprintf("%s/cpu%s%d%[2]scores", opts.prefix, opts.separator, i), core.Cores)
			addNonNil(flattened, fmt.Sprintf("%s/cpu%s%d%[2]sthreads", opts.prefix, opts.separator, i), core.Threads)
			addNonNil(flattened, fmt.Sprintf("%s/cpu%s%d%[2]svendor", opts.prefix, opts.separator, i), core.Vendor)
			addNonNil(flattened, fmt.Sprintf("%s/cpu%s%d%[2]smodel", opts.prefix, opts.separator, i), core.Model)
		}
	}

	// Memory
	if attr.Memory != nil {
		addNonNil(flattened, fmt.Sprintf("%s/memory%stotal", opts.prefix, opts.separator), attr.Memory.Total)
		addNonNil(flattened, fmt.Sprintf("%s/memory%susable", opts.prefix, opts.separator), attr.Memory.Usable)
	}

	// BlockDevices
	for i, block := range attr.Block {
		addNonNil(flattened, fmt.Sprintf("%s/block%s%d%[2]sname", opts.prefix, opts.separator, i), block.Name)
		addNonNil(flattened, fmt.Sprintf("%s/block%s%d%[2]scontrollerType", opts.prefix, opts.separator, i), block.ControllerType)
		addNonNil(flattened, fmt.Sprintf("%s/block%s%d%[2]sdriveType", opts.prefix, opts.separator, i), block.DriveType)
		addNonNil(flattened, fmt.Sprintf("%s/block%s%d%[2]ssize", opts.prefix, opts.separator, i), block.Size)
		addNonNil(flattened, fmt.Sprintf("%s/block%s%d%[2]sphysicalBlockSize", opts.prefix, opts.separator, i), block.PhysicalBlockSize)
		addNonNil(flattened, fmt.Sprintf("%s/block%s%d%[2]svendor", opts.prefix, opts.separator, i), block.Vendor)
		addNonNil(flattened, fmt.Sprintf("%s/block%s%d%[2]smodel", opts.prefix, opts.separator, i), block.Model)
	}

	// NetworkInterfaces
	for i, network := range attr.Network {
		addNonNil(flattened, fmt.Sprintf("%s/network%s%d%[2]sname", opts.prefix, opts.separator, i), network.Name)
		addNonNil(flattened, fmt.Sprintf("%s/network%s%d%[2]smac", opts.prefix, opts.separator, i), network.Mac)
		addNonNil(flattened, fmt.Sprintf("%s/network%s%d%[2]sspeed", opts.prefix, opts.separator, i), network.Speed)
		for j, capability := range network.EnabledCapabilities {
			addNonNil(flattened, fmt.Sprintf("%s/network%s%d%[2]scapability%[2]s%d", opts.prefix, opts.separator, i, j), &capability)
		}
	}

	// PCIDevices
	for i, pci := range attr.Pci {
		addNonNil(flattened, fmt.Sprintf("%s/pci%s%d%[2]svendor", opts.prefix, opts.separator, i), pci.Vendor)
		addNonNil(flattened, fmt.Sprintf("%s/pci%s%d%[2]sproduct", opts.prefix, opts.separator, i), pci.Product)
		addNonNil(flattened, fmt.Sprintf("%s/pci%s%d%[2]sclass", opts.prefix, opts.separator, i), pci.Class)
		addNonNil(flattened, fmt.Sprintf("%s/pci%s%d%[2]sdriver", opts.prefix, opts.separator, i), pci.Driver)
	}

	// GPUDevices
	for i, gpu := range attr.Gpu {
		addNonNil(flattened, fmt.Sprintf("%s/gpu%s%d%[2]svendor", opts.prefix, opts.separator, i), gpu.Vendor)
		addNonNil(flattened, fmt.Sprintf("%s/gpu%s%d%[2]sproduct", opts.prefix, opts.separator, i), gpu.Product)
		addNonNil(flattened, fmt.Sprintf("%s/gpu%s%d%[2]sclass", opts.prefix, opts.separator, i), gpu.Class)
		addNonNil(flattened, fmt.Sprintf("%s/gpu%s%d%[2]sdriver", opts.prefix, opts.separator, i), gpu.Driver)
	}

	// Chassis
	if attr.Chassis != nil {
		addNonNil(flattened, fmt.Sprintf("%s/chassis%sserial", opts.prefix, opts.separator), attr.Chassis.Serial)
		addNonNil(flattened, fmt.Sprintf("%s/chassis%svendor", opts.prefix, opts.separator), attr.Chassis.Vendor)
	}

	// BIOS
	if attr.Bios != nil {
		addNonNil(flattened, fmt.Sprintf("%s/bios%svendor", opts.prefix, opts.separator), attr.Bios.Vendor)
		addNonNil(flattened, fmt.Sprintf("%s/bios%sversion", opts.prefix, opts.separator), attr.Bios.Version)
		addNonNil(flattened, fmt.Sprintf("%s/bios%sreleaseDate", opts.prefix, opts.separator), attr.Bios.ReleaseDate)
	}

	// Baseboard
	if attr.Baseboard != nil {
		addNonNil(flattened, fmt.Sprintf("%s/baseboard%svendor", opts.prefix, opts.separator), attr.Baseboard.Vendor)
		addNonNil(flattened, fmt.Sprintf("%s/baseboard%sproduct", opts.prefix, opts.separator), attr.Baseboard.Product)
		addNonNil(flattened, fmt.Sprintf("%s/baseboard%sversion", opts.prefix, opts.separator), attr.Baseboard.Version)
	}

	// Product
	if attr.Product != nil {
		addNonNil(flattened, fmt.Sprintf("%s/product%sname", opts.prefix, opts.separator), attr.Product.Name)
		addNonNil(flattened, fmt.Sprintf("%s/product%svendor", opts.prefix, opts.separator), attr.Product.Vendor)
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
