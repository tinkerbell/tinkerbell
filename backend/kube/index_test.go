package kube

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/tinkerbell/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMACAddrs(t *testing.T) {
	tests := map[string]struct {
		hw   client.Object
		want []string
	}{
		"not a v1alpha1.Hardware object": {hw: &v1alpha1.Workflow{}, want: nil},
		"2 MACs": {hw: &v1alpha1.Hardware{
			Spec: v1alpha1.HardwareSpec{
				Interfaces: []v1alpha1.Interface{
					{
						DHCP: &v1alpha1.DHCP{
							MAC: "00:00:00:00:00:00",
						},
					},
					{
						DHCP: &v1alpha1.DHCP{
							MAC: "00:00:00:00:00:01",
						},
					},
					{
						DHCP: &v1alpha1.DHCP{},
					},
				},
			},
		}, want: []string{"00:00:00:00:00:00", "00:00:00:00:00:01"}},
		"no interfaces": {hw: &v1alpha1.Hardware{}, want: nil},
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
		"not a v1alpha1.Hardware object": {hw: &v1alpha1.Workflow{}, want: nil},
		"2 IPs": {hw: &v1alpha1.Hardware{
			Spec: v1alpha1.HardwareSpec{
				Interfaces: []v1alpha1.Interface{
					{
						DHCP: &v1alpha1.DHCP{
							IP: &v1alpha1.IP{
								Address: "192.168.2.1",
							},
						},
					},
					{
						DHCP: &v1alpha1.DHCP{
							IP: &v1alpha1.IP{
								Address: "192.168.2.2",
							},
						},
					},
					{
						DHCP: &v1alpha1.DHCP{},
					},
					{
						DHCP: &v1alpha1.DHCP{
							IP: &v1alpha1.IP{},
						},
					},
				},
			},
		}, want: []string{"192.168.2.1", "192.168.2.2"}},
		"no interfaces": {hw: &v1alpha1.Hardware{}, want: nil},
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
			&v1alpha1.Hardware{},
			nil,
		},
		{
			"empty workflow",
			&v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State: "",
					Tasks: []v1alpha1.Task{},
				},
			},
			[]string{},
		},
		{
			"pending workflow",
			&v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State: v1alpha1.WorkflowStatePending,
					Tasks: []v1alpha1.Task{
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
			&v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State: v1alpha1.WorkflowStateRunning,
					Tasks: []v1alpha1.Task{
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
			&v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					State: v1alpha1.WorkflowStateSuccess,
					Tasks: []v1alpha1.Task{
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
