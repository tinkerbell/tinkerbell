package attribute

import (
	"math"

	"github.com/jaypipes/ghw"
	"github.com/jaypipes/ghw/pkg/block"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
)

// Proto populates the proto.WorkerAttributes struct with the attributes of the worker
// using the ghw library.
func Proto() *proto.WorkerAttributes {
	return &proto.WorkerAttributes{
		Cpu:       cpu(),
		Memory:    memory(),
		Block:     blockDevices(),
		Network:   network(),
		Pci:       pci(),
		Gpu:       gpu(),
		Chassis:   chassis(),
		Bios:      bios(),
		Baseboard: baseboard(),
		Product:   product(),
	}
}

func cpu() *proto.CPU {
	cpu, err := ghw.CPU()
	if err != nil {
		return nil
	}
	var processors []*proto.Processor
	for _, p := range cpu.Processors {
		processors = append(processors, &proto.Processor{
			Id:           toPtr(uint32(p.ID)),
			Cores:        toPtr(p.TotalCores),
			Threads:      toPtr(p.TotalHardwareThreads),
			Vendor:       toPtr(p.Vendor),
			Model:        toPtr(p.Model),
			Capabilities: p.Capabilities,
		})
	}

	return &proto.CPU{
		TotalCores:   toPtr(cpu.TotalCores),
		TotalThreads: toPtr(cpu.TotalHardwareThreads),
		Processors:   processors,
	}
}

func memory() *proto.Memory {
	memory, err := ghw.Memory()
	if err != nil {
		return nil
	}

	return &proto.Memory{
		Total:  toPtr(toGB(memory.TotalPhysicalBytes)),
		Usable: toPtr(toGB(memory.TotalUsableBytes)),
	}
}

func blockDevices() []*proto.Block {
	b, err := ghw.Block()
	if err != nil {
		return nil
	}
	var blockDevices []*proto.Block
	for _, d := range b.Disks {
		if d.StorageController != block.STORAGE_CONTROLLER_LOOP {
			blockDevices = append(blockDevices, &proto.Block{
				Name:              toPtr(d.Name),
				ControllerType:    toPtr(d.StorageController.String()),
				DriveType:         toPtr(d.DriveType.String()),
				Size:              toPtr(toGB(d.SizeBytes)),
				PhysicalBlockSize: toPtr(d.PhysicalBlockSizeBytes),
				Vendor:            toPtr(d.Vendor),
				Model:             toPtr(d.Model),
			})
		}
	}
	return blockDevices
}

func network() []*proto.Network {
	net, err := ghw.Network()
	if err != nil {
		return nil
	}
	var nics []*proto.Network
	for _, n := range net.NICs {
		if !n.IsVirtual {
			nics = append(nics, &proto.Network{
				Name:  toPtr(n.Name),
				Mac:   toPtr(n.MACAddress),
				Speed: toPtr(n.Speed),
				EnabledCapabilities: func() []string {
					var capabilities []string
					for _, c := range n.Capabilities {
						if c.IsEnabled {
							capabilities = append(capabilities, c.Name)
						}
					}
					return capabilities
				}(),
			})
		}
	}
	return nics
}

func pci() []*proto.PCI {
	pci, err := ghw.PCI()
	if err != nil {
		return nil
	}
	var pciDevices []*proto.PCI
	for _, d := range pci.Devices {
		pciDevices = append(pciDevices, &proto.PCI{
			Vendor:  toPtr(d.Vendor.Name),
			Product: toPtr(d.Product.Name),
			Class:   toPtr(d.Class.Name),
			Driver:  toPtr(d.Driver),
		})
	}
	return pciDevices
}

func gpu() []*proto.GPU {
	gpu, err := ghw.GPU()
	if err != nil {
		return nil
	}
	var gpus []*proto.GPU
	for _, d := range gpu.GraphicsCards {
		gpus = append(gpus, &proto.GPU{
			Vendor:  toPtr(d.DeviceInfo.Vendor.Name),
			Product: toPtr(d.DeviceInfo.Product.Name),
			Class:   toPtr(d.DeviceInfo.Class.Name),
			Driver:  toPtr(d.DeviceInfo.Driver),
		})
	}
	return gpus
}

func chassis() *proto.Chassis {
	chassis, err := ghw.Chassis()
	if err != nil {
		return nil
	}
	return &proto.Chassis{
		Serial: toPtr(chassis.SerialNumber),
		Vendor: toPtr(chassis.Vendor),
	}
}

func bios() *proto.BIOS {
	bios, err := ghw.BIOS()
	if err != nil {
		return nil
	}
	return &proto.BIOS{
		Vendor:      toPtr(bios.Vendor),
		Version:     toPtr(bios.Version),
		ReleaseDate: toPtr(bios.Date),
	}
}

func baseboard() *proto.Baseboard {
	baseboard, err := ghw.Baseboard()
	if err != nil {
		return nil
	}
	return &proto.Baseboard{
		Vendor:  toPtr(baseboard.Vendor),
		Product: toPtr(baseboard.Product),
		Version: toPtr(baseboard.Version),
	}
}

func product() *proto.Product {
	product, err := ghw.Product()
	if err != nil {
		return nil
	}
	return &proto.Product{
		Name:   toPtr(product.Name),
		Vendor: toPtr(product.Vendor),
	}
}

func toPtr[T any](v T) *T {
	return &v
}

type ByteSize interface {
	~uint64 | ~int64
}

// Generic function to convert megabytes to GB format
func toGB[T ByteSize](mbytes T) uint64 {
	var tpbs uint64
	if mbytes > 0 {
		tpb := int64(mbytes)
		unit, _ := AmountString(tpb)
		tpb = int64(math.Ceil(float64(mbytes) / float64(unit)))
		tpbs = uint64(tpb)
	}

	return tpbs
}

var (
	KB int64 = 1024
	MB       = KB * 1024
	GB       = MB * 1024
	TB       = GB * 1024
	PB       = TB * 1024
	EB       = PB * 1024
)

// AmountString returns a string representation of the amount with an amount
// suffix corresponding to the nearest kibibit.
//
// For example, AmountString(1022) == "1022). AmountString(1024) == "1KB", etc
func AmountString(size int64) (int64, string) {
	switch {
	case size < MB:
		return KB, "KB"
	case size < GB:
		return MB, "MB"
	case size < TB:
		return GB, "GB"
	case size < PB:
		return TB, "TB"
	case size < EB:
		return PB, "PB"
	default:
		return EB, "EB"
	}
}
