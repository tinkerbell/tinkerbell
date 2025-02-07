package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var runtimescheme = runtime.NewScheme()

// TestTime is a static time that can be used for testing.
var TestTime = NewFrozenTimeUnix(1637361793)

func init() {
	_ = clientgoscheme.AddToScheme(runtimescheme)
	_ = tinkerbell.AddToScheme(runtimescheme)
}

func GetFakeClientBuilder() *fake.ClientBuilder {
	return fake.NewClientBuilder().WithScheme(
		runtimescheme,
	).WithRuntimeObjects(
		&tinkerbell.Hardware{}, &tinkerbell.Template{}, &tinkerbell.Workflow{},
	)
}

var minimalTemplate = `version: "0.1"
name: debian
global_timeout: 1800
tasks:
  - name: "os-installation"
    worker: "{{.device_1}}"
    volumes:
      - /dev:/dev
      - /dev/console:/dev/console
      - /lib/firmware:/lib/firmware:ro
    actions:
      - name: "stream-debian-image"
        image: quay.io/tinkerbell-actions/image2disk:v1.0.0
        timeout: 600
        environment:
          DEST_DISK: /dev/nvme0n1
          # Hegel IP
          IMG_URL: "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz"
          COMPRESSED: true`

var templateWithDiskTemplate = `version: "0.1"
name: debian
global_timeout: 1800
tasks:
  - name: "os-installation"
    worker: "{{.device_1}}"
    volumes:
      - /dev:/dev
      - /dev/console:/dev/console
      - /lib/firmware:/lib/firmware:ro
    actions:
      - name: "stream-debian-image"
        image: quay.io/tinkerbell-actions/image2disk:v1.0.0
        timeout: 600
        environment:
          DEST_DISK: {{ index .Hardware.Disks 0 }}
          # Hegel IP
          IMG_URL: "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz"
          COMPRESSED: true
      - name: "action to test templating"
        image: alpine
        timeout: 600
        environment:
          USER_DATA: {{ .Hardware.UserData }}
          VENDOR_DATA: {{ .Hardware.VendorData }}
          METADATA: {{ .Hardware.Metadata.State }}`

func TestHandleHardwareAllowPXE(t *testing.T) {
	tests := map[string]struct {
		OriginalHardware *tinkerbell.Hardware
		WantHardware     *tinkerbell.Hardware
		WantError        error
		AllowPXE         bool
	}{
		"before workflow": {
			OriginalHardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "machine1",
					Namespace:       "default",
					ResourceVersion: "1000",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "3c:ec:ef:4c:4f:54",
							},
							Netboot: &tinkerbell.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			},
			WantHardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "machine1",
					Namespace:       "default",
					ResourceVersion: "1001",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "3c:ec:ef:4c:4f:54",
							},
							Netboot: &tinkerbell.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
			AllowPXE: true,
		},
		"after workflow": {
			OriginalHardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "machine1",
					Namespace:       "default",
					ResourceVersion: "1000",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "3c:ec:ef:4c:4f:54",
							},
							Netboot: &tinkerbell.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
			WantHardware: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "machine1",
					Namespace:       "default",
					ResourceVersion: "1001",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "3c:ec:ef:4c:4f:54",
							},
							Netboot: &tinkerbell.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			fakeClient := GetFakeClientBuilder().WithRuntimeObjects(tt.OriginalHardware).Build()
			wf := &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow1",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					HardwareRef: "machine1",
				},
			}
			err := setAllowPXE(context.Background(), fakeClient, wf, nil, tt.AllowPXE)

			got := &tinkerbell.Hardware{}
			if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(tt.OriginalHardware), got); err != nil {
				t.Fatalf("failed to get hardware after update: %v", err)
			}
			if diff := cmp.Diff(tt.WantError, err, cmp.Comparer(func(a, b error) bool {
				return a.Error() == b.Error()
			})); diff != "" {
				t.Errorf("error type: %T", err)
				t.Fatalf("unexpected error diff: %s", diff)
			}

			if diff := cmp.Diff(tt.WantHardware, got); diff != "" {
				t.Fatalf("unexpected hardware diff: %s", diff)
			}
		})
	}
}

func TestReconcile(t *testing.T) {
	cases := []struct {
		name         string
		seedTemplate *tinkerbell.Template
		seedWorkflow *tinkerbell.Workflow
		seedHardware *tinkerbell.Hardware
		req          reconcile.Request
		want         reconcile.Result
		wantWflow    *tinkerbell.Workflow
		wantErr      error
	}{
		{
			name: "DoesNotExist",
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "notreal",
					Namespace: "default",
				},
			},
			want: reconcile.Result{},
			wantWflow: &tinkerbell.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "999",
				},
			},
			wantErr: nil,
		},
		{
			name: "NewWorkflow",
			seedTemplate: &tinkerbell.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.TemplateSpec{
					Data: &minimalTemplate,
				},
				Status: tinkerbell.TemplateStatus{},
			},
			seedWorkflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{},
			},
			seedHardware: &tinkerbell.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							Netboot: &tinkerbell.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &tinkerbell.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &tinkerbell.IP{
									Address: "172.16.10.100",
									Gateway: "172.16.10.1",
									Netmask: "255.255.255.0",
								},
								LeaseTime:   86400,
								MAC:         "3c:ec:ef:4c:4f:54",
								NameServers: []string{},
								UEFI:        true,
							},
						},
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "debian",
					Namespace: "default",
				},
			},
			want: reconcile.Result{},
			wantWflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{
					State:             tinkerbell.WorkflowStatePending,
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					Conditions: []tinkerbell.WorkflowCondition{
						{Type: tinkerbell.TemplateRenderedSuccess, Status: metav1.ConditionTrue, Reason: "Complete", Message: "template rendered successfully"},
					},
					Tasks: []tinkerbell.Task{
						{
							Name: "os-installation",

							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []tinkerbell.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 600,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: tinkerbell.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "MalformedWorkflow",
			seedTemplate: &tinkerbell.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.TemplateSpec{
					Data: &[]string{`version: "0.1"
					name: debian
global_timeout: 1800
tasks:
	- name: "os-installation"
		worker: "{{.device_1}}"`}[0],
				},
				Status: tinkerbell.TemplateStatus{},
			},
			seedWorkflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{},
			},
			seedHardware: &tinkerbell.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							Netboot: &tinkerbell.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &tinkerbell.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &tinkerbell.IP{
									Address: "172.16.10.100",
									Gateway: "172.16.10.1",
									Netmask: "255.255.255.0",
								},
								LeaseTime:   86400,
								MAC:         "3c:ec:ef:4c:4f:54",
								NameServers: []string{},
								UEFI:        true,
							},
						},
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "debian",
					Namespace: "default",
				},
			},
			want: reconcile.Result{},
			wantWflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{
					State:         tinkerbell.WorkflowStatePending,
					GlobalTimeout: 1800,
					Tasks: []tinkerbell.Task{
						{
							Name: "os-installation",

							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []tinkerbell.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 600,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: tinkerbell.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("found character that cannot start any token"),
		},
		{
			name: "MissingTemplate",
			seedTemplate: &tinkerbell.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy",
					Namespace: "default",
				},
				Spec:   tinkerbell.TemplateSpec{},
				Status: tinkerbell.TemplateStatus{},
			},
			seedWorkflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian", // doesn't exist
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{},
			},
			seedHardware: &tinkerbell.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							Netboot: &tinkerbell.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &tinkerbell.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &tinkerbell.IP{
									Address: "172.16.10.100",
									Gateway: "172.16.10.1",
									Netmask: "255.255.255.0",
								},
								LeaseTime:   86400,
								MAC:         "3c:ec:ef:4c:4f:54",
								NameServers: []string{},
								UEFI:        true,
							},
						},
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "debian",
					Namespace: "default",
				},
			},
			want: reconcile.Result{},
			wantWflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "999",
				},
			},
			wantErr: errors.New("no template found: name=debian; namespace=default"),
		},
		{
			name: "TimedOutWorkflow",
			seedTemplate: &tinkerbell.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.TemplateSpec{
					Data: &minimalTemplate,
				},
				Status: tinkerbell.TemplateStatus{},
			},
			seedWorkflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{
					State:         tinkerbell.WorkflowStateRunning,
					GlobalTimeout: 600,
					Tasks: []tinkerbell.Task{
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []tinkerbell.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status:    tinkerbell.WorkflowStateRunning,
									StartedAt: TestTime.MetaV1BeforeSec(601),
								},
							},
						},
					},
				},
			},
			seedHardware: &tinkerbell.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							Netboot: &tinkerbell.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &tinkerbell.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &tinkerbell.IP{
									Address: "172.16.10.100",
									Gateway: "172.16.10.1",
									Netmask: "255.255.255.0",
								},
								LeaseTime:   86400,
								MAC:         "3c:ec:ef:4c:4f:54",
								NameServers: []string{},
								UEFI:        true,
							},
						},
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "debian",
					Namespace: "default",
				},
			},
			want: reconcile.Result{},
			wantWflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{
					State:         tinkerbell.WorkflowStateTimeout,
					CurrentAction: "stream-debian-image",
					GlobalTimeout: 600,
					Tasks: []tinkerbell.Task{
						{
							Name:       "os-installation",
							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []tinkerbell.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 60,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status:    tinkerbell.WorkflowStateTimeout,
									StartedAt: TestTime.MetaV1BeforeSec(601),
									Seconds:   601,
									Message:   "Action timed out",
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "ErrorGettingHardwareRef",
			seedTemplate: &tinkerbell.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.TemplateSpec{
					Data: &minimalTemplate,
				},
				Status: tinkerbell.TemplateStatus{},
			},
			seedWorkflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareRef: "i_dont_exist",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "debian",
					Namespace: "default",
				},
			},
			want: reconcile.Result{},
			wantWflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{
					State:         tinkerbell.WorkflowStatePending,
					GlobalTimeout: 1800,
					Tasks: []tinkerbell.Task{
						{
							Name: "os-installation",

							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []tinkerbell.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 600,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: tinkerbell.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			wantErr: errors.New("hardware not found: name=i_dont_exist; namespace=default"),
		},
		{
			name: "SuccessWithHardwareRef",
			seedHardware: &tinkerbell.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: tinkerbell.HardwareSpec{
					Disks: []tinkerbell.Disk{
						{Device: "/dev/nvme0n1"},
					},
					Interfaces: []tinkerbell.Interface{
						{
							Netboot: &tinkerbell.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &tinkerbell.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &tinkerbell.IP{
									Address: "172.16.10.100",
									Gateway: "172.16.10.1",
									Netmask: "255.255.255.0",
								},
								LeaseTime:   86400,
								MAC:         "3c:ec:ef:4c:4f:54",
								NameServers: []string{},
								UEFI:        true,
							},
						},
					},
					UserData:   valueToPointer("user-data"),
					Metadata:   &tinkerbell.HardwareMetadata{State: "active"},
					VendorData: valueToPointer("vendor-data"),
				},
			},
			seedTemplate: &tinkerbell.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.TemplateSpec{
					Data: &templateWithDiskTemplate,
				},
				Status: tinkerbell.TemplateStatus{},
			},
			seedWorkflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareRef: "machine1",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "debian",
					Namespace: "default",
				},
			},
			want: reconcile.Result{},
			wantWflow: &tinkerbell.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: tinkerbell.WorkflowSpec{
					TemplateRef: "debian",
					HardwareRef: "machine1",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: tinkerbell.WorkflowStatus{
					State:             tinkerbell.WorkflowStatePending,
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					Conditions: []tinkerbell.WorkflowCondition{
						{Type: tinkerbell.TemplateRenderedSuccess, Status: metav1.ConditionTrue, Reason: "Complete", Message: "template rendered successfully"},
					},
					Tasks: []tinkerbell.Task{
						{
							Name: "os-installation",

							WorkerAddr: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []tinkerbell.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 600,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									Status: tinkerbell.WorkflowStatePending,
								},
								{
									Name:    "action to test templating",
									Image:   "alpine",
									Timeout: 600,
									Environment: map[string]string{
										"USER_DATA":   "user-data",
										"VENDOR_DATA": "vendor-data",
										"METADATA":    "active",
									},
									Status: tinkerbell.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tc := range cases {
		kc := GetFakeClientBuilder()
		if tc.seedHardware != nil {
			kc = kc.WithObjects(tc.seedHardware)
		}
		if tc.seedTemplate != nil {
			kc = kc.WithObjects(tc.seedTemplate)
		}
		if tc.seedWorkflow != nil {
			kc = kc.WithObjects(tc.seedWorkflow)
			kc = kc.WithStatusSubresource(tc.seedWorkflow)
		}
		controller := &Reconciler{
			client:  kc.Build(),
			nowFunc: TestTime.Now,
		}

		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := controller.Reconcile(context.Background(), tc.req)
			if gotErr != nil {
				if tc.wantErr == nil {
					t.Errorf(`Got unexpected error: %v"`, gotErr)
				} else if !strings.Contains(gotErr.Error(), tc.wantErr.Error()) {
					t.Errorf(`Got unexpected error: got "%v" wanted "%v"`, gotErr, tc.wantErr)
				}
				return
			}
			if gotErr == nil && tc.wantErr != nil {
				t.Errorf("Missing expected error: %v", tc.wantErr)
				return
			}
			if tc.want != got {
				t.Errorf("Got unexpected result. Wanted %v, got %v", tc.want, got)
				// Don't return, also check the modified object
			}
			wflow := &tinkerbell.Workflow{}
			err := controller.client.Get(
				context.Background(),
				client.ObjectKey{Name: tc.wantWflow.Name, Namespace: tc.wantWflow.Namespace},
				wflow)
			if err != nil {
				t.Errorf("Error finding desired workflow: %v", err)
				return
			}

			if diff := cmp.Diff(tc.wantWflow, wflow, cmpopts.IgnoreFields(tinkerbell.WorkflowCondition{}, "Time")); diff != "" {
				t.Errorf("unexpected difference:\n%v", diff)
			}
		})
	}
}

// NewFrozenTime returns a FrozenTime for a given unix second.
func NewFrozenTimeUnix(unix int64) *FrozenTime {
	return &FrozenTime{t: time.Unix(unix, 0).UTC()}
}

// FrozenTime is a type for testing out fake times.
type FrozenTime struct {
	t time.Time
}

// Now never changes.
func (f *FrozenTime) Now() time.Time { return f.t }

// Before Now() by int64 seconds.
func (f *FrozenTime) BeforeSec(s int64) time.Time {
	return f.Now().Add(time.Duration(-s) * time.Second)
}

func (f *FrozenTime) MetaV1BeforeSec(s int64) *metav1.Time {
	t := metav1.NewTime(f.BeforeSec(s))
	return &t
}
