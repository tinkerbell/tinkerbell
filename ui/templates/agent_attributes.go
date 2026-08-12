package templates

import (
	"fmt"
	"math"

	"github.com/dustin/go-humanize"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

// AgentAttributesFromData converts the Agent-reported attributes (as sent over
// gRPC and stored verbatim as the tinkerbell.org/agent-attributes annotation)
// into the shape this page renders. Byte counts and link speeds are collected
// as exact numbers precisely so formatting them for display happens only
// here, not at collection time.
func AgentAttributesFromData(attrs *data.AgentAttributes) *AgentAttributes {
	if attrs == nil {
		return nil
	}

	out := &AgentAttributes{
		CPU:       cpuFromData(attrs.CPU),
		Memory:    memoryFromData(attrs.Memory),
		Chassis:   chassisFromData(attrs.Chassis),
		BIOS:      biosFromData(attrs.BIOS),
		Baseboard: baseboardFromData(attrs.Baseboard),
		Product:   productFromData(attrs.Product),
	}

	for _, b := range attrs.BlockDevices {
		if b == nil {
			continue
		}
		out.BlockDevices = append(out.BlockDevices, blockDeviceFromData(b))
	}

	for _, n := range attrs.NetworkInterfaces {
		if n == nil {
			continue
		}
		out.NetworkInterfaces = append(out.NetworkInterfaces, networkInterfaceFromData(n))
	}

	for _, p := range attrs.PCIDevices {
		if p == nil {
			continue
		}
		out.PCIDevices = append(out.PCIDevices, pciDeviceFromData(p))
	}

	for _, g := range attrs.GPUDevices {
		if g == nil {
			continue
		}
		out.GPUDevices = append(out.GPUDevices, gpuDeviceFromData(g))
	}

	return out
}

func cpuFromData(cpu *data.CPU) AgentCPU {
	out := AgentCPU{}
	if cpu == nil {
		return out
	}
	if cpu.TotalCores != nil {
		out.TotalCores = int(*cpu.TotalCores)
	}
	if cpu.TotalThreads != nil {
		out.TotalThreads = int(*cpu.TotalThreads)
	}
	for _, p := range cpu.Processors {
		if p == nil {
			continue
		}
		proc := AgentProcessor{Capabilities: p.Capabilities}
		if p.ID != nil {
			proc.ID = int(*p.ID)
		}
		if p.Cores != nil {
			proc.Cores = int(*p.Cores)
		}
		if p.Threads != nil {
			proc.Threads = int(*p.Threads)
		}
		if p.Vendor != nil {
			proc.Vendor = *p.Vendor
		}
		if p.Model != nil {
			proc.Model = *p.Model
		}
		out.Processors = append(out.Processors, proc)
	}
	return out
}

func memoryFromData(mem *data.Memory) AgentMemory {
	out := AgentMemory{}
	if mem == nil {
		return out
	}
	if mem.TotalBytes != nil {
		out.Total = humanizeBytes(*mem.TotalBytes)
	}
	if mem.UsableBytes != nil {
		out.Usable = humanizeBytes(*mem.UsableBytes)
	}
	return out
}

func blockDeviceFromData(b *data.Block) AgentBlockDevice {
	dev := AgentBlockDevice{}
	if b.Name != nil {
		dev.Name = *b.Name
	}
	if b.ControllerType != nil {
		dev.ControllerType = *b.ControllerType
	}
	if b.DriveType != nil {
		dev.DriveType = *b.DriveType
	}
	if b.SizeBytes != nil {
		dev.Size = humanizeBytes(*b.SizeBytes)
	}
	if b.PhysicalBlockSizeBytes != nil {
		dev.PhysicalBlockSize = humanizeBytes(*b.PhysicalBlockSizeBytes)
	}
	if b.Vendor != nil {
		dev.Vendor = *b.Vendor
	}
	if b.Model != nil {
		dev.Model = *b.Model
	}
	if b.WWN != nil {
		dev.WWN = *b.WWN
	}
	if b.SerialNumber != nil {
		dev.SerialNumber = *b.SerialNumber
	}
	return dev
}

func networkInterfaceFromData(n *data.Network) AgentNetworkInterface {
	iface := AgentNetworkInterface{EnabledCapabilities: n.EnabledCapabilities}
	if n.Name != nil {
		iface.Name = *n.Name
	}
	if n.Mac != nil {
		iface.MAC = *n.Mac
	}
	if n.SpeedMbps != nil {
		iface.Speed = humanizeSpeedMbps(*n.SpeedMbps)
	}
	return iface
}

// deviceIdentityFromData dereferences the vendor/product/class/driver fields
// shared by data.PCI and data.GPU - identical shapes with no common named
// type to dispatch on - so pciDeviceFromData/gpuDeviceFromData don't each
// repeat the same four nil-checks.
func deviceIdentityFromData(vendor, product, class, driver *string) (v, p, c, d string) {
	if vendor != nil {
		v = *vendor
	}
	if product != nil {
		p = *product
	}
	if class != nil {
		c = *class
	}
	if driver != nil {
		d = *driver
	}
	return v, p, c, d
}

func pciDeviceFromData(p *data.PCI) AgentPCIDevice {
	vendor, product, class, driver := deviceIdentityFromData(p.Vendor, p.Product, p.Class, p.Driver)
	return AgentPCIDevice{Vendor: vendor, Product: product, Class: class, Driver: driver}
}

func gpuDeviceFromData(g *data.GPU) AgentGPUDevice {
	vendor, product, class, driver := deviceIdentityFromData(g.Vendor, g.Product, g.Class, g.Driver)
	return AgentGPUDevice{Vendor: vendor, Product: product, Class: class, Driver: driver}
}

func chassisFromData(c *data.Chassis) AgentChassis {
	out := AgentChassis{}
	if c == nil {
		return out
	}
	if c.Serial != nil {
		out.Serial = *c.Serial
	}
	if c.Vendor != nil {
		out.Vendor = *c.Vendor
	}
	return out
}

func biosFromData(b *data.BIOS) AgentBIOS {
	out := AgentBIOS{}
	if b == nil {
		return out
	}
	if b.Vendor != nil {
		out.Vendor = *b.Vendor
	}
	if b.Version != nil {
		out.Version = *b.Version
	}
	if b.ReleaseDate != nil {
		out.ReleaseDate = *b.ReleaseDate
	}
	return out
}

func baseboardFromData(b *data.Baseboard) AgentBaseboard {
	out := AgentBaseboard{}
	if b == nil {
		return out
	}
	if b.Vendor != nil {
		out.Vendor = *b.Vendor
	}
	if b.Product != nil {
		out.Product = *b.Product
	}
	if b.Version != nil {
		out.Version = *b.Version
	}
	if b.SerialNumber != nil {
		out.SerialNumber = *b.SerialNumber
	}
	return out
}

func productFromData(p *data.Product) AgentProduct {
	out := AgentProduct{}
	if p == nil {
		return out
	}
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

// humanizeBytes formats a byte count using decimal (1000-based) units, e.g.
// "32 GiB", matching the binary (1024-based) units hardware spec sheets use
// for RAM/disk capacity. Zero or negative counts (unreported) render blank.
func humanizeBytes(n int64) string {
	if n <= 0 {
		return ""
	}
	return humanize.IBytes(uint64(n))
}

// humanizeSpeedMbps formats a link speed in Mbps, switching to Gbps above 1000.
func humanizeSpeedMbps(speedMbps uint32) string {
	if speedMbps == 0 {
		return ""
	}
	if speedMbps < 1000 {
		return fmt.Sprintf("%d Mbps", speedMbps)
	}
	gbps := float64(speedMbps) / 1000
	if gbps == math.Trunc(gbps) {
		return fmt.Sprintf("%d Gbps", int64(gbps))
	}
	return fmt.Sprintf("%.1f Gbps", gbps)
}
