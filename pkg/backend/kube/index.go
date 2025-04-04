package kube

import (
	"github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IndexType string

const (
	IndexTypeMACAddr                    IndexType = MACAddrIndex
	IndexTypeIPAddr                     IndexType = IPAddrIndex
	IndexTypeWorkflowByNonTerminalState IndexType = WorkflowByNonTerminalState
	IndexTypeHardwareName               IndexType = "hardware.metadata.name"
	IndexTypeMachineName                IndexType = "machine.metadata.name"
)

// Indexes that are currently known.
var Indexes = map[IndexType]Index{
	IndexTypeMACAddr: {
		Obj:          &v1alpha1.Hardware{},
		Field:        MACAddrIndex,
		ExtractValue: MACAddrs,
	},
	IndexTypeIPAddr: {
		Obj:          &v1alpha1.Hardware{},
		Field:        IPAddrIndex,
		ExtractValue: IPAddrs,
	},
	IndexTypeWorkflowByNonTerminalState: {
		Obj:          &v1alpha1.Workflow{},
		Field:        WorkflowByNonTerminalState,
		ExtractValue: WorkflowByNonTerminalStateFunc,
	},
	IndexTypeHardwareName: {
		Obj:          &v1alpha1.Hardware{},
		Field:        HardwareNameIndex,
		ExtractValue: HardwareNameFunc,
	},
	IndexTypeMachineName: {
		Obj:          &bmc.Machine{},
		Field:        MachineNameIndex,
		ExtractValue: MachineNameFunc,
	},
}

// MACAddrIndex is an index used with a controller-runtime client to lookup hardware by MAC.
const MACAddrIndex = ".Spec.Interfaces.MAC"

// MACAddrs returns a list of MAC addresses for a Hardware object.
func MACAddrs(obj client.Object) []string {
	hw, ok := obj.(*v1alpha1.Hardware)
	if !ok {
		return nil
	}
	return GetMACs(hw)
}

// GetMACs retrieves all MACs associated with h.
func GetMACs(h *v1alpha1.Hardware) []string {
	var macs []string
	for _, i := range h.Spec.Interfaces {
		if i.DHCP != nil && i.DHCP.MAC != "" {
			macs = append(macs, i.DHCP.MAC)
		}
	}

	return macs
}

// IPAddrIndex is an index used with a controller-runtime client to lookup hardware by IP.
const IPAddrIndex = ".Spec.Interfaces.DHCP.IP"

// IPAddrs returns a list of IP addresses for a Hardware object.
func IPAddrs(obj client.Object) []string {
	hw, ok := obj.(*v1alpha1.Hardware)
	if !ok {
		return nil
	}
	return GetIPs(hw)
}

// GetIPs retrieves all IP addresses.
func GetIPs(h *v1alpha1.Hardware) []string {
	var ips []string
	for _, i := range h.Spec.Interfaces {
		if i.DHCP != nil && i.DHCP.IP != nil && i.DHCP.IP.Address != "" {
			ips = append(ips, i.DHCP.IP.Address)
		}
	}
	return ips
}

// WorkflowByNonTerminalState is the index name for retrieving workflows in a non-terminal state.
const WorkflowByNonTerminalState = ".status.state.nonTerminalWorker"

// WorkflowByNonTerminalStateFunc inspects obj - which must be a Workflow - for a Pending or
// Running state. If in either Pending or Running it returns a list of worker addresses.
func WorkflowByNonTerminalStateFunc(obj client.Object) []string {
	wf, ok := obj.(*v1alpha1.Workflow)
	if !ok {
		return nil
	}

	resp := []string{}
	if wf.Status.State != v1alpha1.WorkflowStateRunning && wf.Status.State != v1alpha1.WorkflowStatePending {
		return resp
	}
	for _, task := range wf.Status.Tasks {
		if task.AgentID != "" {
			resp = append(resp, task.AgentID)
		}
	}

	return resp
}

// NameIndex is an index used with a controller-runtime client to lookup objects by name.
const HardwareNameIndex = ".metadata.name"

func HardwareNameFunc(obj client.Object) []string {
	hw, ok := obj.(*v1alpha1.Hardware)
	if !ok {
		return nil
	}
	return []string{hw.Name}
}

const MachineNameIndex = ".metadata.name"

func MachineNameFunc(obj client.Object) []string {
	m, ok := obj.(*bmc.Machine)
	if !ok {
		return nil
	}
	return []string{m.Name}
}
