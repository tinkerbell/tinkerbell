package grpc

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"quamina.net/go/quamina"
)

func TestGetAction(t *testing.T) {
	cases := map[string]struct {
		workflow *v1alpha1.Workflow
		request  *proto.ActionRequest
		want     *proto.ActionResponse
		wantErr  error
	}{
		"successful second Action in Task": {
			request: &proto.ActionRequest{
				WorkerId: toPtr("machine-mac-1"),
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Status: v1alpha1.WorkflowStatus{
					State: v1alpha1.WorkflowStateRunning,
					CurrentState: &v1alpha1.CurrentState{
						WorkerID:   "machine-mac-1",
						TaskID:     "provision",
						ActionID:   "stream",
						State:      v1alpha1.WorkflowStateSuccess,
						ActionName: "stream",
					},
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							ID:         "provision",
							Actions: []v1alpha1.Action{
								{
									Name:              "stream",
									Image:             "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:           300,
									State:             v1alpha1.WorkflowStateSuccess,
									ExecutionStart:    nil,
									ExecutionDuration: "30s",
									ID:                "stream",
								},
								{
									Name:    "kexec",
									Image:   "quay.io/tinkerbell-actions/kexec:v1.0.0",
									Timeout: 5,
									State:   v1alpha1.WorkflowStatePending,
									ID:      "kexec",
								},
							},
						},
					},
				},
			},
			want: &proto.ActionResponse{
				WorkflowId:  toPtr("default/machine1"),
				WorkerId:    toPtr("machine-mac-1"),
				TaskId:      toPtr("provision"),
				ActionId:    toPtr("kexec"),
				Name:        toPtr("kexec"),
				Image:       toPtr("quay.io/tinkerbell-actions/kexec:v1.0.0"),
				Timeout:     toPtr(int64(5)),
				Environment: []string{},
				Pid:         new(string),
			},
			wantErr: nil,
		},
		"successful first Action in Task": {
			request: &proto.ActionRequest{
				WorkerId: toPtr("machine-mac-1"),
			},
			want: &proto.ActionResponse{
				WorkflowId:  toPtr("default/machine1"),
				WorkerId:    toPtr("machine-mac-1"),
				TaskId:      new(string),
				ActionId:    new(string),
				Name:        toPtr("stream"),
				Image:       toPtr("quay.io/tinkerbell-actions/image2disk:v1.0.0"),
				Timeout:     toPtr(int64(300)),
				Environment: []string{},
				Pid:         new(string),
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Status: v1alpha1.WorkflowStatus{
					State:         v1alpha1.WorkflowStateRunning,
					GlobalTimeout: 600,
					Tasks: []v1alpha1.Task{
						{
							Name:       "provision",
							WorkerAddr: "machine-mac-1",
							Actions: []v1alpha1.Action{
								{
									Name:              "stream",
									Image:             "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout:           300,
									State:             v1alpha1.WorkflowStatePending,
									ExecutionStart:    nil,
									ExecutionDuration: "30s",
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := &Handler{
				Logger:            logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, nil)),
				BackendReadWriter: &mockBackendReadWriter{workflow: tc.workflow},
				NowFunc:           func() time.Time { return time.Time{} },
				RetryOptions:      []backoff.RetryOption{backoff.WithMaxTries(1)},
			}

			resp, gotErr := server.GetAction(context.Background(), tc.request)
			compareErrors(t, gotErr, tc.wantErr)
			if tc.want == nil {
				return
			}

			if diff := cmp.Diff(resp, tc.want, cmpopts.IgnoreUnexported(proto.ActionResponse{})); diff != "" {
				t.Errorf("unexpected difference:\n%v", diff)
			}
		})
	}
}

// compareErrors is a helper function for comparing an error value and a desired error.
func compareErrors(t *testing.T, got, want error) {
	t.Helper()
	if got != nil {
		if want == nil {
			t.Fatalf(`Got unexpected error: %v"`, got)
		} else if got.Error() != want.Error() {
			t.Fatalf(`Got unexpected error: got "%v" wanted "%v"`, got, want)
		}
		return
	}
	if got == nil && want != nil {
		t.Fatalf("Missing expected error: %v", want)
	}
}

type mockBackendReadWriter struct {
	workflow *v1alpha1.Workflow
}

func (m *mockBackendReadWriter) Read(_ context.Context, _, _ string) (*v1alpha1.Workflow, error) {
	return m.workflow, nil
}

func (m *mockBackendReadWriter) ReadAll(_ context.Context, _ string) ([]v1alpha1.Workflow, error) {
	if m.workflow != nil {
		return []v1alpha1.Workflow{*m.workflow}, nil
	}
	return []v1alpha1.Workflow{}, nil
}

func (m *mockBackendReadWriter) Update(_ context.Context, _ *v1alpha1.Workflow) error {
	return nil
}

type mockBackendReadWriterForReport struct {
	workflow *v1alpha1.Workflow
	writeErr error
}

func (m *mockBackendReadWriterForReport) Read(_ context.Context, _, _ string) (*v1alpha1.Workflow, error) {
	if m.workflow == nil {
		return nil, errors.New("workflow not found")
	}
	return m.workflow, nil
}

func (m *mockBackendReadWriterForReport) ReadAll(_ context.Context, _ string) ([]v1alpha1.Workflow, error) {
	return nil, nil
}

func (m *mockBackendReadWriterForReport) Update(_ context.Context, _ *v1alpha1.Workflow) error {
	return m.writeErr
}

func TestReportActionStatus(t *testing.T) {
	tests := map[string]struct {
		request      *proto.ActionStatusRequest
		workflow     *v1alpha1.Workflow
		writeErr     error
		expectedResp *proto.ActionStatusResponse
		expectedErr  error
	}{
		"success": {
			request: &proto.ActionStatusRequest{
				WorkflowId:        toPtr("default/workflow1"),
				TaskId:            toPtr("task1"),
				ActionId:          toPtr("action1"),
				ActionState:       toPtr(proto.ActionStatusRequest_SUCCESS),
				ExecutionStart:    timestamppb.New(time.Now()),
				ExecutionDuration: toPtr("30s"),
				Message: &proto.ActionMessage{
					Message: toPtr("Action completed successfully"),
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow1",
					Namespace: "default",
				},
				Status: v1alpha1.WorkflowStatus{
					Tasks: []v1alpha1.Task{
						{
							ID: "task1",
							Actions: []v1alpha1.Action{
								{
									ID:    "action1",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			writeErr:     nil,
			expectedErr:  nil,
			expectedResp: &proto.ActionStatusResponse{},
		},
		"write error": {
			request: &proto.ActionStatusRequest{
				WorkflowId:        toPtr("default/workflow6"),
				TaskId:            toPtr("task1"),
				ActionId:          toPtr("action1"),
				ActionState:       toPtr(proto.ActionStatusRequest_SUCCESS),
				ExecutionStart:    timestamppb.New(time.Now()),
				ExecutionDuration: toPtr("30s"),
				Message: &proto.ActionMessage{
					Message: toPtr("Action completed successfully"),
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow1",
					Namespace: "default",
				},
				Status: v1alpha1.WorkflowStatus{
					Tasks: []v1alpha1.Task{
						{
							ID: "task1",
							Actions: []v1alpha1.Action{
								{
									ID:    "action1",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			writeErr:    errors.New("write error"),
			expectedErr: status.Errorf(codes.Internal, "error writing report status: write error"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &Handler{
				BackendReadWriter: &mockBackendReadWriterForReport{
					workflow: tc.workflow,
					writeErr: tc.writeErr,
				},
				RetryOptions: []backoff.RetryOption{backoff.WithMaxTries(1)},
			}

			resp, err := handler.ReportActionStatus(context.Background(), tc.request)

			if diff := cmp.Diff(tc.expectedResp, resp, protocmp.Transform()); diff != "" {
				t.Errorf("unexpected response (-want +got):\n%s", diff)
			}

			if tc.expectedErr != nil {
				if err == nil || err.Error() != tc.expectedErr.Error() {
					t.Errorf("unexpected error: \ngot:  %v\nwant: %v", err, tc.expectedErr)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

var rawJSON = `{
    "cpu": {
        "total_cores": 4,
        "total_threads": 8,
        "processors": [
            {
                "id": 0,
                "cores": 4,
                "threads": 8,
                "vendor": "GenuineIntel",
                "model": "11th Gen Intel(R) Core(TM) i5-1145G7E @ 2.60GHz",
                "capabilities": [
                    "fpu",
                    "vme",
                    "de",
                    "pse",
                    "tsc",
                    "msr",
                    "pae",
                    "mce",
                    "cx8",
                    "apic",
                    "sep",
                    "mtrr",
                    "pge",
                    "mca",
                    "cmov",
                    "pat",
                    "pse36",
                    "clflush",
                    "dts",
                    "acpi",
                    "mmx",
                    "fxsr",
                    "sse",
                    "sse2",
                    "ss",
                    "ht",
                    "tm",
                    "pbe",
                    "syscall",
                    "nx",
                    "pdpe1gb",
                    "rdtscp",
                    "lm",
                    "constant_tsc",
                    "art",
                    "arch_perfmon",
                    "pebs",
                    "bts",
                    "rep_good",
                    "nopl",
                    "xtopology",
                    "nonstop_tsc",
                    "cpuid",
                    "aperfmperf",
                    "tsc_known_freq",
                    "pni",
                    "pclmulqdq",
                    "dtes64",
                    "monitor",
                    "ds_cpl",
                    "vmx",
                    "smx",
                    "est",
                    "tm2",
                    "ssse3",
                    "sdbg",
                    "fma",
                    "cx16",
                    "xtpr",
                    "pdcm",
                    "pcid",
                    "sse4_1",
                    "sse4_2",
                    "x2apic",
                    "movbe",
                    "popcnt",
                    "tsc_deadline_timer",
                    "aes",
                    "xsave",
                    "avx",
                    "f16c",
                    "rdrand",
                    "lahf_lm",
                    "abm",
                    "3dnowprefetch",
                    "cpuid_fault",
                    "epb",
                    "cat_l2",
                    "invpcid_single",
                    "cdp_l2",
                    "ssbd",
                    "ibrs",
                    "ibpb",
                    "stibp",
                    "ibrs_enhanced",
                    "tpr_shadow",
                    "vnmi",
                    "flexpriority",
                    "ept",
                    "vpid",
                    "ept_ad",
                    "fsgsbase",
                    "tsc_adjust",
                    "bmi1",
                    "avx2",
                    "smep",
                    "bmi2",
                    "erms",
                    "invpcid",
                    "rdt_a",
                    "avx512f",
                    "avx512dq",
                    "rdseed",
                    "adx",
                    "smap",
                    "avx512ifma",
                    "clflushopt",
                    "clwb",
                    "intel_pt",
                    "avx512cd",
                    "sha_ni",
                    "avx512bw",
                    "avx512vl",
                    "xsaveopt",
                    "xsavec",
                    "xgetbv1",
                    "xsaves",
                    "split_lock_detect",
                    "dtherm",
                    "ida",
                    "arat",
                    "pln",
                    "pts",
                    "avx512vbmi",
                    "umip",
                    "pku",
                    "ospke",
                    "avx512_vbmi2",
                    "gfni",
                    "vaes",
                    "vpclmulqdq",
                    "avx512_vnni",
                    "avx512_bitalg",
                    "tme",
                    "avx512_vpopcntdq",
                    "rdpid",
                    "movdiri",
                    "movdir64b",
                    "fsrm",
                    "avx512_vp2intersect",
                    "md_clear",
                    "flush_l1d",
                    "arch_capabilities"
                ]
            }
        ]
    },
    "memory": {
        "total": "32GB",
        "usable": "31GB"
    },
    "block": [
        {
            "name": "nvme0n1",
            "controller_type": "NVMe",
            "drive_type": "SSD",
            "size": "239GB",
            "physical_block_size": "512B",
            "vendor": "unknown",
            "model": "KINGSTON OM8PDP3256B-A01"
        }
    ],
    "network": [
        {
            "name": "eno1",
            "mac": "a8:a1:59:d0:e2:52",
            "speed": "1000Mb/s",
            "enabled_capabilities": [
                "auto-negotiation",
                "rx-checksumming",
                "tx-checksumming",
                "tx-checksum-ip-generic",
                "scatter-gather",
                "tx-scatter-gather",
                "tcp-segmentation-offload",
                "tx-tcp-segmentation",
                "tx-tcp6-segmentation",
                "generic-segmentation-offload",
                "generic-receive-offload",
                "rx-vlan-offload",
                "tx-vlan-offload",
                "receive-hashing",
                "highdma"
            ]
        }
    ],
    "pci": [
        {
            "vendor": "Intel Corporation",
            "product": "11th Gen Core Processor Host Bridge/DRAM Registers",
            "class": "Bridge",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "TigerLake-LP GT2 [Iris Xe Graphics]",
            "class": "Display controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "TigerLake-LP Dynamic Tuning Processor Participant",
            "class": "Signal processing controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "11th Gen Core Processor PCIe Controller",
            "class": "Bridge",
            "driver": "pcieport"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Thunderbolt 4 PCI Express Root Port #2",
            "class": "Bridge",
            "driver": "pcieport"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Thunderbolt 4 PCI Express Root Port #3",
            "class": "Bridge",
            "driver": "pcieport"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tigerlake Telemetry Aggregator Driver",
            "class": "Signal processing controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Thunderbolt 4 USB Controller",
            "class": "Serial bus controller",
            "driver": "xhci_hcd"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Thunderbolt 4 NHI #1",
            "class": "Serial bus controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP USB 3.2 Gen 2x1 xHCI Host Controller",
            "class": "Serial bus controller",
            "driver": "xhci_hcd"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Shared SRAM",
            "class": "Memory controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Serial IO I2C Controller #0",
            "class": "Serial bus controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Serial IO I2C Controller #2",
            "class": "Serial bus controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Serial IO I2C Controller #3",
            "class": "Serial bus controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Management Engine Interface",
            "class": "Communication controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Active Management Technology - SOL",
            "class": "Communication controller",
            "driver": "serial"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Serial IO I2C Controller #4",
            "class": "Serial bus controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Serial IO I2C Controller #5",
            "class": "Serial bus controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "unknown",
            "class": "Bridge",
            "driver": "pcieport"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tigerlake PCH-LP PCI Express Root Port #6",
            "class": "Bridge",
            "driver": "pcieport"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP PCI Express Root Port #8",
            "class": "Bridge",
            "driver": "pcieport"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Serial IO UART Controller #0",
            "class": "Communication controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Serial IO SPI Controller #1",
            "class": "Serial bus controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP LPC Controller",
            "class": "Bridge",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP Smart Sound Technology Audio Controller",
            "class": "Multimedia controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP SMBus Controller",
            "class": "Serial bus controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Tiger Lake-LP SPI Controller",
            "class": "Serial bus controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Ethernet Connection (13) I219-LM",
            "class": "Network controller",
            "driver": "e1000e"
        },
        {
            "vendor": "Kingston Technology Company, Inc.",
            "product": "OM3PDP3 NVMe SSD",
            "class": "Mass storage controller",
            "driver": "nvme"
        },
        {
            "vendor": "Intel Corporation",
            "product": "Wi-Fi 6 AX200",
            "class": "Network controller",
            "driver": ""
        },
        {
            "vendor": "Intel Corporation",
            "product": "Ethernet Controller I225-LM",
            "class": "Network controller",
            "driver": ""
        }
    ],
    "chassis": {
        "serial": "To Be Filled By O.E.M.",
        "vendor": "To Be Filled By O.E.M."
    },
    "bios": {
        "vendor": "American Megatrends International, LLC.",
        "version": "P1.50J",
        "release_date": "12/13/2021"
    },
    "baseboard": {
        "vendor": "ASRock",
        "product": "NUC-TGL",
        "version": ""
    },
    "product": {
        "name": "LLN11CRv5",
        "vendor": "Simply NUC"
    }
}`

func TestRules(t *testing.T) {
	pattern1 := `{"memory": {"usable": [{"regexp": "([3-9][0-9])(GB|TB)"}]}}`

	q, err := quamina.New()
	if err != nil {
		t.Fatalf("failed to create quamina instance: %v", err)
	}
	if err := q.AddPattern(pattern1, pattern1); err != nil {
		t.Fatalf("failed to add pattern: %v", err)
	}
	matches, err := q.MatchesForEvent([]byte(rawJSON))
	if err != nil {
		t.Fatalf("failed to match patterns: %v", err)
	}
	for _, match := range matches {
		t.Logf("found a match: %v", match)
	}
	t.Fail()

}
