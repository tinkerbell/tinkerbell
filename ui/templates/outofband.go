package templates

import (
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

// OutOfBandAttributesFromStatus converts the typed
// Hardware.status.attributes.outOfBand field into the template's flattened
// OutOfBandAttributes type. Unlike AgentAttributes, which is parsed from a JSON
// annotation, this is a plain Go-to-Go mapping.
func OutOfBandAttributesFromStatus(attrs *tinkv1alpha1.Attributes) *OutOfBandAttributes {
	if attrs == nil {
		return nil
	}

	out := &OutOfBandAttributes{
		CollectionMethod:   attrs.CollectionMethod,
		Product:            productFromAPI(attrs.Product),
		BIOS:               biosFromAPI(attrs.BIOS),
		BMC:                bmcFromAPI(attrs.BMC),
		Baseboard:          baseboardFromAPI(attrs.Baseboard),
		Chassis:            chassisFromAPI(attrs.Chassis),
		StorageControllers: make([]OOBComponent, 0, len(attrs.StorageControllers)),
	}
	if attrs.LastUpdated != nil {
		out.LastUpdated = attrs.LastUpdated.Format("2006-01-02 15:04:05")
	}

	if attrs.CPU != nil {
		out.TotalCores = attrs.CPU.TotalCores
		out.TotalThreads = attrs.CPU.TotalThreads
		for _, c := range attrs.CPU.Sockets {
			out.CPUs = append(out.CPUs, OOBCPUSocket{
				Slot:            c.Slot,
				Vendor:          c.Vendor,
				Model:           c.Model,
				Cores:           c.Cores,
				Threads:         c.Threads,
				ClockSpeedMHz:   c.ClockSpeedMHz,
				SerialNumber:    c.SerialNumber,
				FirmwareVersion: c.FirmwareVersion,
			})
		}
	}

	if attrs.Memory != nil {
		out.TotalMemoryBytes = attrs.Memory.TotalBytes
		for _, m := range attrs.Memory.Modules {
			out.MemoryModules = append(out.MemoryModules, OOBMemoryModule{
				Slot:            m.Slot,
				Vendor:          m.Vendor,
				Model:           m.Model,
				SerialNumber:    m.SerialNumber,
				PartNumber:      m.PartNumber,
				SizeBytes:       m.SizeBytes,
				SpeedMHz:        m.SpeedMHz,
				FormFactor:      m.FormFactor,
				FirmwareVersion: m.FirmwareVersion,
			})
		}
	}

	for _, d := range attrs.BlockDevices {
		out.Drives = append(out.Drives, OOBBlockDevice{
			ControllerType:  d.ControllerType,
			DriveType:       d.DriveType,
			Vendor:          d.Vendor,
			Model:           d.Model,
			SerialNumber:    d.SerialNumber,
			WWN:             d.WWN,
			SizeBytes:       d.SizeBytes,
			SmartStatus:     d.SmartStatus,
			FirmwareVersion: d.FirmwareVersion,
			Status:          statusFromAPI(d.Status),
		})
	}

	for _, n := range attrs.NetworkInterfaces {
		nic := OOBNetworkInterface{
			Vendor:          n.Vendor,
			Model:           n.Model,
			SerialNumber:    n.SerialNumber,
			FirmwareVersion: n.FirmwareVersion,
		}
		for _, p := range n.Ports {
			nic.Ports = append(nic.Ports, OOBNetworkPort{
				PortID:     p.PortID,
				MAC:        p.MAC,
				LinkStatus: p.LinkStatus,
				SpeedMbps:  p.SpeedMbps,
				MTU:        p.MTU,
			})
		}
		out.NICs = append(out.NICs, nic)
	}

	for _, sc := range attrs.StorageControllers {
		out.StorageControllers = append(out.StorageControllers, OOBComponent{
			Vendor:          sc.Vendor,
			Model:           sc.Model,
			SerialNumber:    sc.SerialNumber,
			Description:     sc.Description,
			FirmwareVersion: sc.FirmwareVersion,
			Status:          statusFromAPI(sc.Status),
		})
	}

	for _, p := range attrs.PSUs {
		out.PSUs = append(out.PSUs, OOBPSU{
			Vendor:             p.Vendor,
			Model:              p.Model,
			SerialNumber:       p.SerialNumber,
			Description:        p.Description,
			FirmwareVersion:    p.FirmwareVersion,
			PowerCapacityWatts: p.PowerCapacityWatts,
			Status:             statusFromAPI(p.Status),
		})
	}

	for _, tpm := range attrs.TPMs {
		out.TPMs = append(out.TPMs, OOBTPM{
			Vendor:          tpm.Vendor,
			Model:           tpm.Model,
			SerialNumber:    tpm.SerialNumber,
			InterfaceType:   tpm.InterfaceType,
			FirmwareVersion: tpm.FirmwareVersion,
			Status:          statusFromAPI(tpm.Status),
		})
	}

	for _, g := range attrs.GPUDevices {
		out.GPUs = append(out.GPUs, OOBGPU{
			Vendor:          g.Vendor,
			Model:           g.Model,
			SerialNumber:    g.SerialNumber,
			Description:     g.Description,
			FirmwareVersion: g.FirmwareVersion,
			Status:          statusFromAPI(g.Status),
		})
	}

	return out
}

func statusFromAPI(s *tinkv1alpha1.ComponentStatus) OOBStatus {
	if s == nil {
		return OOBStatus{}
	}
	st := OOBStatus{
		Health:         s.Health,
		State:          s.State,
		PostCodeStatus: s.PostCodeStatus,
	}
	if s.PostCode != nil {
		st.PostCode = s.PostCode
	}
	return st
}

func productFromAPI(p *tinkv1alpha1.Product) OOBProduct {
	if p == nil {
		return OOBProduct{}
	}
	return OOBProduct{
		Name:         p.Name,
		Vendor:       p.Vendor,
		Model:        p.Model,
		SerialNumber: p.SerialNumber,
		Status:       statusFromAPI(p.Status),
	}
}

func biosFromAPI(b *tinkv1alpha1.BIOS) OOBBIOS {
	if b == nil {
		return OOBBIOS{}
	}
	return OOBBIOS{
		Vendor:          b.Vendor,
		Model:           b.Model,
		SerialNumber:    b.SerialNumber,
		FirmwareVersion: b.FirmwareVersion,
		ReleaseDate:     b.ReleaseDate,
		Status:          statusFromAPI(b.Status),
	}
}

func bmcFromAPI(b *tinkv1alpha1.BMC) OOBBMC {
	if b == nil {
		return OOBBMC{}
	}
	out := OOBBMC{
		Vendor:          b.Vendor,
		Model:           b.Model,
		SerialNumber:    b.SerialNumber,
		FirmwareVersion: b.FirmwareVersion,
		Status:          statusFromAPI(b.Status),
	}
	if b.NIC != nil {
		nic := OOBNetworkInterface{
			Vendor:          b.NIC.Vendor,
			Model:           b.NIC.Model,
			SerialNumber:    b.NIC.SerialNumber,
			FirmwareVersion: b.NIC.FirmwareVersion,
		}
		for _, p := range b.NIC.Ports {
			nic.Ports = append(nic.Ports, OOBNetworkPort{
				PortID:     p.PortID,
				MAC:        p.MAC,
				LinkStatus: p.LinkStatus,
				SpeedMbps:  p.SpeedMbps,
				MTU:        p.MTU,
			})
		}
		out.NIC = &nic
	}
	return out
}

func baseboardFromAPI(b *tinkv1alpha1.Baseboard) OOBComponent {
	if b == nil {
		return OOBComponent{}
	}
	return OOBComponent{
		Vendor:          b.Vendor,
		Model:           b.Model,
		SerialNumber:    b.SerialNumber,
		Description:     b.Description,
		FirmwareVersion: b.FirmwareVersion,
		Status:          statusFromAPI(b.Status),
	}
}

func chassisFromAPI(c *tinkv1alpha1.Chassis) OOBComponent {
	if c == nil {
		return OOBComponent{}
	}
	return OOBComponent{
		Vendor:       c.Vendor,
		Model:        c.Model,
		SerialNumber: c.SerialNumber,
	}
}
