package templates

import (
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

// BMCInventoryFromStatus converts the typed Hardware.status.bmcInventory field
// into the template's flattened BMCInventory type. Unlike AgentAttributes,
// which is parsed from a JSON annotation, this is a plain Go-to-Go mapping —
// inv is already a typed value once PR-1 of the OOB-discovery plan has landed.
func BMCInventoryFromStatus(inv *tinkv1alpha1.BMCInventory) *BMCInventory {
	if inv == nil {
		return nil
	}

	out := &BMCInventory{
		CollectionMethod: inv.CollectionMethod,
		Product:          productFromStatus(inv.Product),
		BIOS:             firmwareComponentFromStatus(inv.BIOS),
		BMC:              firmwareComponentFromStatus(inv.BMC),
		Mainboard:        componentFromStatus(inv.Mainboard),
	}
	if inv.LastUpdated != nil {
		out.LastUpdated = inv.LastUpdated.Format("2006-01-02 15:04:05")
	}

	for _, c := range inv.CPUs {
		out.CPUs = append(out.CPUs, BMCCPUComponent{
			Vendor:            c.Vendor,
			Model:             c.Model,
			SerialNumber:      c.SerialNumber,
			Slot:              c.Slot,
			Cores:             c.Cores,
			Threads:           c.Threads,
			ClockSpeedMHz:     c.ClockSpeedMHz,
			FirmwareInstalled: c.FirmwareInstalled,
		})
	}
	for _, m := range inv.Memory {
		out.Memory = append(out.Memory, BMCMemoryComponent{
			Vendor:            m.Vendor,
			Model:             m.Model,
			SerialNumber:      m.SerialNumber,
			Slot:              m.Slot,
			SizeBytes:         m.SizeBytes,
			SpeedMHz:          m.SpeedMHz,
			FormFactor:        m.FormFactor,
			PartNumber:        m.PartNumber,
			FirmwareInstalled: m.FirmwareInstalled,
		})
	}
	for _, n := range inv.NICs {
		nic := BMCNICComponent{
			Vendor:            n.Vendor,
			Model:             n.Model,
			SerialNumber:      n.SerialNumber,
			FirmwareInstalled: n.FirmwareInstalled,
		}
		for _, p := range n.Ports {
			nic.Ports = append(nic.Ports, BMCNICPort{
				PortID:     p.PortID,
				MACAddress: p.MACAddress,
				LinkStatus: p.LinkStatus,
				SpeedMbps:  p.SpeedMbps,
				MTU:        p.MTU,
			})
		}
		out.NICs = append(out.NICs, nic)
	}
	for _, d := range inv.Drives {
		out.Drives = append(out.Drives, BMCDriveComponent{
			Vendor:            d.Vendor,
			Model:             d.Model,
			SerialNumber:      d.SerialNumber,
			WWN:               d.WWN,
			SizeBytes:         d.SizeBytes,
			Type:              d.Type,
			SmartStatus:       d.SmartStatus,
			FirmwareInstalled: d.FirmwareInstalled,
		})
	}
	for _, sc := range inv.StorageControllers {
		out.StorageControllers = append(out.StorageControllers, componentFromStatusValue(sc))
	}
	for _, p := range inv.PSUs {
		out.PSUs = append(out.PSUs, BMCPSUComponent{
			Vendor:             p.Vendor,
			Model:              p.Model,
			SerialNumber:       p.SerialNumber,
			Description:        p.Description,
			Status:             statusFromAPI(p.Status),
			PowerCapacityWatts: p.PowerCapacityWatts,
		})
	}
	for _, tpm := range inv.TPMs {
		out.TPMs = append(out.TPMs, componentFromStatusValue(tpm))
	}
	for _, g := range inv.GPUs {
		out.GPUs = append(out.GPUs, componentFromStatusValue(g))
	}

	return out
}

func productFromStatus(p *tinkv1alpha1.BMCProduct) BMCProduct {
	if p == nil {
		return BMCProduct{}
	}
	return BMCProduct{
		Vendor:       p.Vendor,
		Model:        p.Model,
		ProductName:  p.ProductName,
		SerialNumber: p.SerialNumber,
		Status:       statusFromAPI(p.Status),
	}
}

func statusFromAPI(s *tinkv1alpha1.BMCStatus) BMCStatus {
	if s == nil {
		return BMCStatus{}
	}
	return BMCStatus{
		Health:         s.Health,
		State:          s.State,
		PostCode:       s.PostCode,
		PostCodeStatus: s.PostCodeStatus,
	}
}

func firmwareComponentFromStatus(c *tinkv1alpha1.BMCFirmwareComponent) BMCFirmwareComponent {
	if c == nil {
		return BMCFirmwareComponent{}
	}
	return BMCFirmwareComponent{
		Vendor:            c.Vendor,
		Model:             c.Model,
		SerialNumber:      c.SerialNumber,
		FirmwareInstalled: c.FirmwareInstalled,
		Status:            statusFromAPI(c.Status),
	}
}

func componentFromStatus(c *tinkv1alpha1.BMCComponent) BMCComponent {
	if c == nil {
		return BMCComponent{}
	}
	return componentFromStatusValue(*c)
}

func componentFromStatusValue(c tinkv1alpha1.BMCComponent) BMCComponent {
	return BMCComponent{
		Vendor:            c.Vendor,
		Model:             c.Model,
		SerialNumber:      c.SerialNumber,
		Description:       c.Description,
		FirmwareInstalled: c.FirmwareInstalled,
		Status:            statusFromAPI(c.Status),
	}
}
