package grpc

import (
	"math"
	"strconv"
	"strings"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

// inBandAttributesFromAgent converts the Agent-reported attributes (as sent
// over gRPC) into the shape shared with the out-of-band collection path
// (status.attributes.inBand). It's a reshaping of data already collected by
// the Agent, not a superset: some Attributes fields have no analog in
// data.AgentAttributes today (e.g. per-DIMM memory detail, drive firmware/SMART
// status, network port link state) and are simply left unset. CollectionMethod
// and LastUpdated aren't Agent-collected fields, so they're not set here; the
// caller fills them in at write time.
func inBandAttributesFromAgent(attrs *data.AgentAttributes) *tinkerbell.Attributes {
	if attrs == nil {
		return nil
	}

	out := &tinkerbell.Attributes{
		Chassis:   chassisFromAgent(attrs.Chassis),
		BIOS:      biosFromAgent(attrs.BIOS),
		Baseboard: baseboardFromAgent(attrs.Baseboard),
		Product:   productFromAgent(attrs.Product),
	}

	if attrs.CPU != nil {
		cpu := &tinkerbell.CPU{}
		if attrs.CPU.TotalCores != nil {
			cpu.TotalCores = *attrs.CPU.TotalCores
		}
		if attrs.CPU.TotalThreads != nil {
			cpu.TotalThreads = *attrs.CPU.TotalThreads
		}
		for _, p := range attrs.CPU.Processors {
			if p == nil {
				continue
			}
			socket := tinkerbell.CPUSocket{Capabilities: p.Capabilities}
			if p.Vendor != nil {
				socket.Vendor = *p.Vendor
			}
			if p.Model != nil {
				socket.Model = *p.Model
			}
			if p.Cores != nil {
				socket.Cores = *p.Cores
			}
			if p.Threads != nil {
				socket.Threads = *p.Threads
			}
			cpu.Sockets = append(cpu.Sockets, socket)
		}
		out.CPU = cpu
	}

	// attrs.Memory.Total/Usable are human-readable strings (e.g. "32GB") with no
	// exact byte count available from the Agent today, so Memory is left unset
	// entirely rather than populating it with a misleadingly precise-looking
	// TotalBytes/UsableBytes of 0.

	for _, b := range attrs.BlockDevices {
		if b == nil {
			continue
		}
		dev := tinkerbell.BlockDevice{}
		if b.Name != nil {
			dev.Name = *b.Name
		}
		if b.ControllerType != nil {
			dev.ControllerType = *b.ControllerType
		}
		if b.DriveType != nil {
			dev.DriveType = *b.DriveType
		}
		if b.Vendor != nil {
			dev.Vendor = *b.Vendor
		}
		if b.Model != nil {
			dev.Model = *b.Model
		}
		if b.SerialNumber != nil {
			dev.SerialNumber = *b.SerialNumber
		}
		if b.WWN != nil {
			dev.WWN = *b.WWN
		}
		// b.Size/PhysicalBlockSize are human-readable strings with no exact byte
		// count available from the Agent today; SizeBytes/PhysicalBlockSizeBytes
		// are left unset (0) rather than lossily parsed.
		out.BlockDevices = append(out.BlockDevices, dev)
	}

	for _, n := range attrs.NetworkInterfaces {
		if n == nil {
			continue
		}
		iface := tinkerbell.NetworkInterface{}
		if n.Name != nil {
			iface.Name = *n.Name
		}
		port := tinkerbell.NetworkPort{EnabledCapabilities: n.EnabledCapabilities}
		if n.Mac != nil {
			port.MAC = *n.Mac
		}
		if n.Speed != nil {
			port.SpeedMbps = parseSpeedMbps(*n.Speed)
		}
		iface.Ports = []tinkerbell.NetworkPort{port}
		out.NetworkInterfaces = append(out.NetworkInterfaces, iface)
	}

	for _, p := range attrs.PCIDevices {
		if p == nil {
			continue
		}
		dev := tinkerbell.PCIDevice{}
		if p.Vendor != nil {
			dev.Vendor = *p.Vendor
		}
		if p.Product != nil {
			dev.Model = *p.Product
		}
		if p.Class != nil {
			dev.Class = *p.Class
		}
		if p.Driver != nil {
			dev.Driver = *p.Driver
		}
		out.PCIDevices = append(out.PCIDevices, dev)
	}

	for _, g := range attrs.GPUDevices {
		if g == nil {
			continue
		}
		dev := tinkerbell.GPUDevice{}
		if g.Vendor != nil {
			dev.Vendor = *g.Vendor
		}
		if g.Product != nil {
			dev.Model = *g.Product
		}
		if g.Class != nil {
			dev.Class = *g.Class
		}
		if g.Driver != nil {
			dev.Driver = *g.Driver
		}
		out.GPUDevices = append(out.GPUDevices, dev)
	}

	return out
}

// parseSpeedMbps extracts a leading integer Mbps value from a ghw-reported
// speed string, which varies in format depending on how the Agent collected it
// (e.g. "1000" from sysfs, or "1000Mb/s" from ethtool). Returns 0 if no leading
// digits are present.
func parseSpeedMbps(speed string) uint32 {
	digits := strings.TrimLeftFunc(speed, func(r rune) bool { return r < '0' || r > '9' })
	end := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' })
	unit := ""
	if end != -1 {
		unit = digits[end:]
		digits = digits[:end]
	}
	v, err := strconv.ParseUint(digits, 10, 32)
	if err != nil {
		return 0
	}
	if strings.Contains(strings.ToLower(unit), "gb") {
		v *= 1000
	}
	if v > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(v)
}

func chassisFromAgent(c *data.Chassis) *tinkerbell.Chassis {
	if c == nil {
		return nil
	}
	out := &tinkerbell.Chassis{}
	if c.Vendor != nil {
		out.Vendor = *c.Vendor
	}
	if c.Serial != nil {
		out.SerialNumber = *c.Serial
	}
	return out
}

func biosFromAgent(b *data.BIOS) *tinkerbell.BIOS {
	if b == nil {
		return nil
	}
	out := &tinkerbell.BIOS{}
	if b.Vendor != nil {
		out.Vendor = *b.Vendor
	}
	if b.Version != nil {
		out.FirmwareVersion = *b.Version
	}
	if b.ReleaseDate != nil {
		out.ReleaseDate = *b.ReleaseDate
	}
	return out
}

func baseboardFromAgent(b *data.Baseboard) *tinkerbell.Baseboard {
	if b == nil {
		return nil
	}
	out := &tinkerbell.Baseboard{}
	if b.Vendor != nil {
		out.Vendor = *b.Vendor
	}
	if b.Product != nil {
		out.Model = *b.Product
	}
	if b.Version != nil {
		out.FirmwareVersion = *b.Version
	}
	if b.SerialNumber != nil {
		out.SerialNumber = *b.SerialNumber
	}
	return out
}

func productFromAgent(p *data.Product) *tinkerbell.Product {
	if p == nil {
		return nil
	}
	out := &tinkerbell.Product{}
	if p.Name != nil {
		out.Name = *p.Name
	}
	if p.Vendor != nil {
		out.Vendor = *p.Vendor
	}
	if p.SerialNumber != nil {
		out.SerialNumber = *p.SerialNumber
	}
	return out
}
