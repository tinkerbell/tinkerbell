package attribute

import (
	"math"
	"strconv"
	"strings"

	"github.com/ccoveille/go-safecast/v2"
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
		id, err := safecast.Convert[uint32](p.ID)
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
		TotalBytes:  toPtr(memory.TotalPhysicalBytes),
		UsableBytes: toPtr(memory.TotalUsableBytes),
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
			sizeBytes, _ := safecast.Convert[int64](d.SizeBytes)
			physicalBlockSizeBytes, _ := safecast.Convert[int64](d.PhysicalBlockSizeBytes)
			blockDevices = append(blockDevices, &data.Block{
				Name:                   toPtr(d.Name),
				ControllerType:         toPtr(d.StorageController.String()),
				DriveType:              toPtr(d.DriveType.String()),
				SizeBytes:              toPtr(sizeBytes),
				PhysicalBlockSizeBytes: toPtr(physicalBlockSizeBytes),
				Vendor:                 toPtr(d.Vendor),
				Model:                  toPtr(d.Model),
				WWN:                    toPtr(d.WWN),
				SerialNumber:           toPtr(d.SerialNumber),
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
			Name:      toPtr(n.Name),
			Mac:       toPtr(n.MACAddress),
			SpeedMbps: toPtr(parseSpeedMbps(n.Speed)),
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

// parseSpeedMbps extracts a leading integer Mbps value from a ghw-reported
// speed string, which varies in format depending on how ghw collected it
// (e.g. "1000" from sysfs, "1000Mb/s" from ethtool, or "-1" from sysfs when
// the link speed is unknown/down). Returns 0 if no leading digits are
// present, or if speed is negative (sysfs's unknown-speed sentinel - a plain
// digit-strip would otherwise turn "-1" into a fabricated 1). Values that
// overflow uint32 after unit scaling clamp to math.MaxUint32 rather than
// wrapping.
func parseSpeedMbps(speed string) uint32 {
	trimmed := strings.TrimSpace(speed)
	if strings.HasPrefix(trimmed, "-") {
		return 0
	}
	end := strings.IndexFunc(trimmed, func(r rune) bool { return r < '0' || r > '9' })
	digits, unit := trimmed, ""
	if end != -1 {
		digits, unit = trimmed[:end], trimmed[end:]
	}
	v, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return 0
	}
	switch {
	case strings.Contains(unit, "GB"):
		// Gigabytes/sec (case-sensitive: "GB", not "Gb") - 8 bits per byte.
		v *= 8000
	case strings.Contains(unit, "MB"):
		v *= 8
	case strings.Contains(strings.ToLower(unit), "gb"):
		// Gigabits/sec, e.g. ethtool's "10 Gb/s".
		v *= 1000
	}
	if v > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(v)
}
