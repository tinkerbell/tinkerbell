package templates

import (
	"strconv"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

// AgentAttributesFromInBand converts the shared Attributes schema, as reported
// by the Tink Agent and stored at Hardware.status.attributes.inBand, into the
// shape the "In-Band Attributes" section of the Hardware detail page renders.
func AgentAttributesFromInBand(attrs *tinkv1alpha1.Attributes) *AgentAttributes {
	if attrs == nil {
		return nil
	}

	out := &AgentAttributes{}

	if attrs.CPU != nil {
		out.CPU.TotalCores = int(attrs.CPU.TotalCores)
		out.CPU.TotalThreads = int(attrs.CPU.TotalThreads)
		for i, socket := range attrs.CPU.Sockets {
			// Slot holds the Agent-reported processor ID as a string (in-band) or a
			// physical socket label like "CPU1" (out-of-band, not numeric); fall back
			// to the array index when it doesn't parse as a number.
			id := i
			if n, err := strconv.Atoi(socket.Slot); err == nil {
				id = n
			}
			out.CPU.Processors = append(out.CPU.Processors, AgentProcessor{
				ID:           id,
				Cores:        int(socket.Cores),
				Threads:      int(socket.Threads),
				Vendor:       socket.Vendor,
				Model:        socket.Model,
				Capabilities: socket.Capabilities,
			})
		}
	}

	if attrs.Memory != nil {
		out.Memory.Total = humanizeBytes(attrs.Memory.TotalBytes)
		out.Memory.Usable = humanizeBytes(attrs.Memory.UsableBytes)
	}

	for _, dev := range attrs.BlockDevices {
		out.BlockDevices = append(out.BlockDevices, AgentBlockDevice{
			Name:              dev.Name,
			Size:              humanizeBytes(dev.SizeBytes),
			ControllerType:    dev.ControllerType,
			DriveType:         dev.DriveType,
			PhysicalBlockSize: humanizeBytes(dev.PhysicalBlockSizeBytes),
			Vendor:            dev.Vendor,
			Model:             dev.Model,
			WWN:               dev.WWN,
			SerialNumber:      dev.SerialNumber,
		})
	}

	for _, iface := range attrs.NetworkInterfaces {
		if len(iface.Ports) == 0 {
			out.NetworkInterfaces = append(out.NetworkInterfaces, AgentNetworkInterface{Name: iface.Name})
			continue
		}
		for _, port := range iface.Ports {
			out.NetworkInterfaces = append(out.NetworkInterfaces, AgentNetworkInterface{
				Name:                iface.Name,
				MAC:                 port.MAC,
				Speed:               humanizeSpeedMbps(port.SpeedMbps),
				EnabledCapabilities: port.EnabledCapabilities,
			})
		}
	}

	for _, pci := range attrs.PCIDevices {
		out.PCIDevices = append(out.PCIDevices, AgentPCIDevice{
			Vendor:  pci.Vendor,
			Product: pci.Model,
			Class:   pci.Class,
			Driver:  pci.Driver,
		})
	}

	for _, gpu := range attrs.GPUDevices {
		out.GPUDevices = append(out.GPUDevices, AgentGPUDevice{
			Vendor:  gpu.Vendor,
			Product: gpu.Model,
			Class:   gpu.Class,
			Driver:  gpu.Driver,
		})
	}

	if attrs.Chassis != nil {
		out.Chassis.Vendor = attrs.Chassis.Vendor
		out.Chassis.Serial = attrs.Chassis.SerialNumber
	}

	if attrs.BIOS != nil {
		out.BIOS.Vendor = attrs.BIOS.Vendor
		out.BIOS.Version = attrs.BIOS.FirmwareVersion
		out.BIOS.ReleaseDate = attrs.BIOS.ReleaseDate
	}

	if attrs.Baseboard != nil {
		out.Baseboard.Vendor = attrs.Baseboard.Vendor
		out.Baseboard.Product = attrs.Baseboard.Model
		out.Baseboard.Version = attrs.Baseboard.FirmwareVersion
		out.Baseboard.SerialNumber = attrs.Baseboard.SerialNumber
	}

	if attrs.Product != nil {
		out.Product.Name = attrs.Product.Name
		out.Product.Vendor = attrs.Product.Vendor
		out.Product.SerialNumber = attrs.Product.SerialNumber
	}

	return out
}

// humanizeBytes and humanizeSpeedMbps are shared with agent_attributes.go.
