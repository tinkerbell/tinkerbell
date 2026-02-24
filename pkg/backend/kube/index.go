package kube

import (
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IndexType string

const (
	IndexTypeMACAddr         IndexType = MACAddrIndex
	IndexTypeIPAddr          IndexType = IPAddrIndex
	IndexTypeHardwareName    IndexType = "hardware.metadata.name"
	IndexTypeMachineName     IndexType = "machine.metadata.name"
	IndexTypeWorkflowAgentID IndexType = WorkflowAgentIDIndex
	IndexTypeHardwareAgentID IndexType = HardwareAgentIDIndex
	IndexTypeInstanceID      IndexType = InstanceIDIndex

	// MACAddrIndex is an index used with a controller-runtime client to lookup hardware by MAC.
	MACAddrIndex = ".Spec.Interfaces.MAC"

	// IPAddrIndex is an index used with a controller-runtime client to lookup hardware by IP.
	IPAddrIndex = ".Spec.Interfaces.DHCP.IP"

	// NameIndex is an index used with a controller-runtime client to lookup objects by name.
	NameIndex = ".metadata.name"

	// WorkflowAgentIDIndex is an index used with a controller-runtime client to lookup workflows by their status agent id.
	WorkflowAgentIDIndex = ".status.agentID"

	// HardwareAgentIDIndex is an index used with a controller-runtime client to lookup hardware by their spec agent id.
	HardwareAgentIDIndex = ".spec.agentID"

	// InstanceIDIndex is an index used with a controller-runtime client to lookup hardware by its metadata instance id.
	InstanceIDIndex = ".Spec.Metadata.Instance.ID" // #nosec G101 - This is a field path, not a credential

)

// Indexes that are currently known.
var Indexes = map[IndexType]Index{
	IndexTypeMACAddr: {
		Obj:          &tinkerbell.Hardware{},
		Field:        MACAddrIndex,
		ExtractValue: MACAddrs,
	},
	IndexTypeIPAddr: {
		Obj:          &tinkerbell.Hardware{},
		Field:        IPAddrIndex,
		ExtractValue: IPAddrs,
	},
	IndexTypeHardwareName: {
		Obj:          &tinkerbell.Hardware{},
		Field:        NameIndex,
		ExtractValue: HardwareName,
	},
	IndexTypeMachineName: {
		Obj:          &bmc.Machine{},
		Field:        NameIndex,
		ExtractValue: MachineName,
	},
	IndexTypeWorkflowAgentID: {
		Obj:          &tinkerbell.Workflow{},
		Field:        WorkflowAgentIDIndex,
		ExtractValue: WorkflowAgentID,
	},
	IndexTypeHardwareAgentID: {
		Obj:          &tinkerbell.Hardware{},
		Field:        HardwareAgentIDIndex,
		ExtractValue: HardwareAgentID,
	},
	IndexTypeInstanceID: {
		Obj:          &tinkerbell.Hardware{},
		Field:        InstanceIDIndex,
		ExtractValue: InstanceID,
	},
}

// MACAddrs returns a list of MAC addresses for a Hardware object.
func MACAddrs(obj client.Object) []string {
	hw, ok := obj.(*tinkerbell.Hardware)
	if !ok {
		return nil
	}
	return GetMACs(hw)
}

// GetMACs retrieves all MACs associated with h.
func GetMACs(h *tinkerbell.Hardware) []string {
	var macs []string
	for _, i := range h.Spec.Interfaces {
		if i.DHCP != nil && i.DHCP.MAC != "" {
			macs = append(macs, i.DHCP.MAC)
		}
	}

	return macs
}

// IPAddrs returns a list of IP addresses for a Hardware object.
func IPAddrs(obj client.Object) []string {
	hw, ok := obj.(*tinkerbell.Hardware)
	if !ok {
		return nil
	}
	return GetIPs(hw)
}

// GetIPs retrieves all IP addresses.
func GetIPs(h *tinkerbell.Hardware) []string {
	var ips []string
	for _, i := range h.Spec.Interfaces {
		if i.DHCP != nil && i.DHCP.IP != nil && i.DHCP.IP.Address != "" {
			ips = append(ips, i.DHCP.IP.Address)
		}
	}
	return ips
}

func HardwareName(obj client.Object) []string {
	hw, ok := obj.(*tinkerbell.Hardware)
	if !ok {
		return nil
	}
	return []string{hw.Name}
}

func MachineName(obj client.Object) []string {
	m, ok := obj.(*bmc.Machine)
	if !ok {
		return nil
	}
	return []string{m.Name}
}

func WorkflowAgentID(obj client.Object) []string {
	wf, ok := obj.(*tinkerbell.Workflow)
	if !ok {
		return nil
	}
	if wf.Status.AgentID == "" {
		return []string{}
	}
	return []string{wf.Status.AgentID}
}

func HardwareAgentID(obj client.Object) []string {
	hw, ok := obj.(*tinkerbell.Hardware)
	if !ok {
		return nil
	}
	if hw.Spec.AgentID == "" {
		return []string{}
	}
	return []string{hw.Spec.AgentID}
}

func InstanceID(obj client.Object) []string {
	hw, ok := obj.(*tinkerbell.Hardware)
	if !ok {
		return nil
	}
	if hw.Spec.Metadata == nil || hw.Spec.Metadata.Instance == nil || hw.Spec.Metadata.Instance.ID == "" {
		return []string{}
	}
	return []string{hw.Spec.Metadata.Instance.ID}
}
