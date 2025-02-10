package kube

import (
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IndexType string

const (
	IndexTypeMACAddr                    IndexType = MACAddrIndex
	IndexTypeIPAddr                     IndexType = IPAddrIndex
	IndexTypeWorkflowByNonTerminalState IndexType = WorkflowByNonTerminalState
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
	if !(wf.Status.State == v1alpha1.WorkflowStateRunning || wf.Status.State == v1alpha1.WorkflowStatePending) {
		return resp
	}
	for _, task := range wf.Status.Tasks {
		if task.WorkerAddr != "" {
			resp = append(resp, task.WorkerAddr)
		}
	}

	return resp
}
