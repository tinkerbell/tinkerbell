package attribute

import (
	"fmt"
	"math"

	"github.com/ccoveille/go-safecast"
	"github.com/go-logr/logr"
	"github.com/jaypipes/ghw"
	"github.com/jaypipes/ghw/pkg/block"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

func DiscoverAll(l logr.Logger) *data.AgentAttributes {
	return &data.AgentAttributes{
		CPU:               DiscoverCPU(l),
		Memory:            DiscoverMemory(l),
		BlockDevices:      DiscoverBlockDevices(l),
		NetworkInterfaces: DiscoverNetworks(l),
		PCIDevices:        DiscoverPCI(l),
		GPUDevices:        DiscoverGPU(l),
		Chassis:           DiscoverChassis(l),
		BIOS:              DiscoverBIOS(l),
		Baseboard:         DiscoverBaseboard(l),
		Product:           DiscoverProduct(l),
	}
}

func DiscoverCPU(l logr.Logger) *data.CPU {
	cpu, err := ghw.CPU(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting cpu info", "error", err)
		return nil
	}
	if cpu == nil {
		return new(data.CPU)
	}
	var processors []*data.Processor
	for _, p := range cpu.Processors {
		if p == nil {
			continue
		}
		id, err := safecast.ToUint32(p.ID)
		if err != nil {
			id = uint32(0)
		}
		processors = append(processors, &data.Processor{
			ID:           toPtr(id),
			Cores:        toPtr(p.TotalCores),
			Threads:      toPtr(p.TotalHardwareThreads),
			Vendor:       toPtr(p.Vendor),
			Model:        toPtr(p.Model),
			Capabilities: p.Capabilities,
		})
	}

	return &data.CPU{
		TotalCores:   toPtr(cpu.TotalCores),
		TotalThreads: toPtr(cpu.TotalHardwareThreads),
		Processors:   processors,
	}
}

func DiscoverMemory(l logr.Logger) *data.Memory {
	memory, err := ghw.Memory(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting memory info", "error", err)
		return nil
	}
	if memory == nil {
		return new(data.Memory)
	}

	return &data.Memory{
		Total:  toPtr(humanReadable(memory.TotalPhysicalBytes)),
		Usable: toPtr(humanReadable(memory.TotalUsableBytes)),
	}
}

func DiscoverBlockDevices(l logr.Logger) []*data.Block {
	b, err := ghw.Block(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting block info", "error", err)
		return nil
	}
	var blockDevices []*data.Block
	for _, d := range b.Disks {
		if d == nil {
			continue
		}
		if d.StorageController != block.StorageControllerLoop && d.StorageController != block.StorageControllerUnknown {
			blockDevices = append(blockDevices, &data.Block{
				Name:              toPtr(d.Name),
				ControllerType:    toPtr(d.StorageController.String()),
				DriveType:         toPtr(d.DriveType.String()),
				Size:              toPtr(humanReadable(d.SizeBytes)),
				PhysicalBlockSize: toPtr(humanReadable(d.PhysicalBlockSizeBytes)),
				Vendor:            toPtr(d.Vendor),
				Model:             toPtr(d.Model),
			})
		}
	}
	return blockDevices
}

func DiscoverNetworks(l logr.Logger) []*data.Network {
	net, err := ghw.Network(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting network info", "error", err)
		return nil
	}
	var nics []*data.Network
	for _, n := range net.NICs {
		if n == nil {
			continue
		}
		nics = append(nics, &data.Network{
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
	return nics
}

func DiscoverPCI(l logr.Logger) []*data.PCI {
	p, err := ghw.PCI(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting pci info", "error", err)
		return nil
	}
	var pciDevices []*data.PCI
	for _, d := range p.Devices {
		if d == nil {
			continue
		}
		var valueFound bool
		dev := &data.PCI{}
		if d.Vendor != nil {
			dev.Vendor = toPtr(d.Vendor.Name)
			valueFound = true
		}
		if d.Product != nil {
			dev.Product = toPtr(d.Product.Name)
			valueFound = true
		}
		if d.Class != nil {
			dev.Class = toPtr(d.Class.Name)
			valueFound = true
		}
		if d.Driver != "" {
			dev.Driver = toPtr(d.Driver)
			valueFound = true
		}
		if !valueFound {
			continue
		}

		pciDevices = append(pciDevices, dev)
	}
	return pciDevices
}

func DiscoverGPU(l logr.Logger) []*data.GPU {
	g, err := ghw.GPU(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting gpu info", "error", err)
		return nil
	}
	var gpus []*data.GPU
	for _, gc := range g.GraphicsCards {
		if gc == nil {
			continue
		}
		if gc.DeviceInfo == nil {
			continue
		}
		var valueFound bool
		card := &data.GPU{}
		if gc.DeviceInfo.Vendor != nil {
			card.Vendor = toPtr(gc.DeviceInfo.Vendor.Name)
			valueFound = true
		}
		if gc.DeviceInfo.Product != nil {
			card.Product = toPtr(gc.DeviceInfo.Product.Name)
			valueFound = true
		}
		if gc.DeviceInfo.Class != nil {
			card.Class = toPtr(gc.DeviceInfo.Class.Name)
			valueFound = true
		}
		if gc.DeviceInfo.Driver != "" {
			card.Driver = toPtr(gc.DeviceInfo.Driver)
			valueFound = true
		}
		if !valueFound {
			continue
		}

		gpus = append(gpus, card)
	}
	return gpus
}

func DiscoverChassis(l logr.Logger) *data.Chassis {
	chass, err := ghw.Chassis(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting chassis info", "error", err)
		return nil
	}
	if chass == nil {
		return new(data.Chassis)
	}
	return &data.Chassis{
		Serial: toPtr(chass.SerialNumber),
		Vendor: toPtr(chass.Vendor),
	}
}

func DiscoverBIOS(l logr.Logger) *data.BIOS {
	bio, err := ghw.BIOS(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting bios info", "error", err)
		return nil
	}
	if bio == nil {
		return new(data.BIOS)
	}
	return &data.BIOS{
		Vendor:      toPtr(bio.Vendor),
		Version:     toPtr(bio.Version),
		ReleaseDate: toPtr(bio.Date),
	}
}

func DiscoverBaseboard(l logr.Logger) *data.Baseboard {
	baseboard, err := ghw.Baseboard(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting baseboard info", "error", err)
		return nil
	}
	if baseboard == nil {
		return new(data.Baseboard)
	}
	return &data.Baseboard{
		Vendor:       toPtr(baseboard.Vendor),
		Product:      toPtr(baseboard.Product),
		Version:      toPtr(baseboard.Version),
		SerialNumber: toPtr(baseboard.SerialNumber),
	}
}

func DiscoverProduct(l logr.Logger) *data.Product {
	product, err := ghw.Product(ghw.WithDisableWarnings())
	if err != nil {
		l.V(1).Info("error getting product info", "error", err)
		return nil
	}
	if product == nil {
		return new(data.Product)
	}
	return &data.Product{
		Name:         toPtr(product.Name),
		Vendor:       toPtr(product.Vendor),
		SerialNumber: toPtr(product.SerialNumber),
	}
}

func toPtr[T any](v T) *T {
	return &v
}

type byteSize interface {
	~uint64 | ~int64
}

// humanReadable is a function to convert bytes to a human readable format.
// 512 -> 512B, 1024 -> 1KB, 1024*1024 -> 1MB, etc.
func humanReadable[T byteSize](byts T) string {
	var tpbs string
	if byts > 0 {
		tpb := int64(byts)
		unit, unitString := amountString(tpb)
		tpb = int64(math.Ceil(float64(byts) / float64(unit)))
		t, err := safecast.ToUint64(tpb)
		if err != nil {
			t = uint64(0)
		}
		tpbs = fmt.Sprintf("%v%s", t, unitString)
	}

	return tpbs
}

var (
	kb int64 = 1024
	mb       = kb * 1024
	gb       = mb * 1024
	tb       = gb * 1024
	pb       = tb * 1024
	eb       = pb * 1024
)

// amountString returns a string representation of the
// amount with an amount suffix corresponding to the nearest kibibit.
//
// For example, amountString(1022) == "1022". amountString(1024) == "1KB", etc.
func amountString(size int64) (int64, string) {
	switch {
	case size < kb:
		return 1, "B"
	case size < mb:
		return kb, "KB"
	case size < gb:
		return mb, "MB"
	case size < tb:
		return gb, "GB"
	case size < pb:
		return tb, "TB"
	case size < eb:
		return pb, "PB"
	default:
		return eb, "EB"
	}
}
