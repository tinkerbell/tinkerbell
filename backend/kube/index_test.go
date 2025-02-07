package kube

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMACAddrs(t *testing.T) {
	tests := map[string]struct {
		hw   client.Object
		want []string
	}{
		"not a v1alpha1.Hardware object": {hw: &tinkerbell.Workflow{}, want: nil},
		"2 MACs": {hw: &tinkerbell.Hardware{
			Spec: tinkerbell.HardwareSpec{
				Interfaces: []tinkerbell.Interface{
					{
						DHCP: &tinkerbell.DHCP{
							MAC: "00:00:00:00:00:00",
						},
					},
					{
						DHCP: &tinkerbell.DHCP{
							MAC: "00:00:00:00:00:01",
						},
					},
					{
						DHCP: &tinkerbell.DHCP{},
					},
				},
			},
		}, want: []string{"00:00:00:00:00:00", "00:00:00:00:00:01"}},
		"no interfaces": {hw: &tinkerbell.Hardware{}, want: nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			macs := MACAddrs(tc.hw)
			if diff := cmp.Diff(macs, tc.want); diff != "" {
				t.Errorf("unexpected MACs (+want -got):\n%s", diff)
			}
		})
	}
}

func TestIPAddrs(t *testing.T) {
	tests := map[string]struct {
		hw   client.Object
		want []string
	}{
		"not a v1alpha1.Hardware object": {hw: &tinkerbell.Workflow{}, want: nil},
		"2 IPs": {hw: &tinkerbell.Hardware{
			Spec: tinkerbell.HardwareSpec{
				Interfaces: []tinkerbell.Interface{
					{
						DHCP: &tinkerbell.DHCP{
							IP: &tinkerbell.IP{
								Address: "192.168.2.1",
							},
						},
					},
					{
						DHCP: &tinkerbell.DHCP{
							IP: &tinkerbell.IP{
								Address: "192.168.2.2",
							},
						},
					},
					{
						DHCP: &tinkerbell.DHCP{},
					},
					{
						DHCP: &tinkerbell.DHCP{
							IP: &tinkerbell.IP{},
						},
					},
				},
			},
		}, want: []string{"192.168.2.1", "192.168.2.2"}},
		"no interfaces": {hw: &tinkerbell.Hardware{}, want: nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := IPAddrs(tc.hw)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("unexpected IPs (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWorkflowIndexFuncs(t *testing.T) {
	cases := []struct {
		name           string
		input          client.Object
		wantStateAddrs []string
	}{
		{
			"non workflow",
			&tinkerbell.Hardware{},
			nil,
		},
		{
			"empty workflow",
			&tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					State: "",
					Tasks: []tinkerbell.Task{},
				},
			},
			[]string{},
		},
		{
			"pending workflow",
			&tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					State: tinkerbell.WorkflowStatePending,
					Tasks: []tinkerbell.Task{
						{
							WorkerAddr: "worker1",
						},
					},
				},
			},
			[]string{"worker1"},
		},
		{
			"running workflow",
			&tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					State: tinkerbell.WorkflowStateRunning,
					Tasks: []tinkerbell.Task{
						{
							WorkerAddr: "worker1",
						},
						{
							WorkerAddr: "worker2",
						},
					},
				},
			},
			[]string{"worker1", "worker2"},
		},
		{
			"complete workflow",
			&tinkerbell.Workflow{
				Status: tinkerbell.WorkflowStatus{
					State: tinkerbell.WorkflowStateSuccess,
					Tasks: []tinkerbell.Task{
						{
							WorkerAddr: "worker1",
						},
					},
				},
			},
			[]string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStateAddrs := WorkflowByNonTerminalStateFunc(tc.input)
			if !reflect.DeepEqual(tc.wantStateAddrs, gotStateAddrs) {
				t.Errorf("Unexpected non-terminating workflow response: wanted %#v, got %#v", tc.wantStateAddrs, gotStateAddrs)
			}
		})
	}
}
