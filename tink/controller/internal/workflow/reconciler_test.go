package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	_ = api.AddToSchemeTinkerbell(runtimescheme)
}

func GetFakeClientBuilder() *fake.ClientBuilder {
	return fake.NewClientBuilder().WithScheme(
		runtimescheme,
	).WithRuntimeObjects(
		&v1alpha1.Hardware{}, &v1alpha1.Template{}, &v1alpha1.Workflow{},
	)
}

type fakeDynamicClient struct {
	unstructured map[string]interface{}
	error        error
}

func (f *fakeDynamicClient) DynamicRead(_ context.Context, _ schema.GroupVersionResource, _, _ string) (map[string]interface{}, error) {
	return f.unstructured, f.error
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
          # Tootles IP
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
          # Tootles IP
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
		OriginalHardware *v1alpha1.Hardware
		WantHardware     *v1alpha1.Hardware
		WantError        error
		AllowPXE         bool
	}{
		"before workflow": {
			OriginalHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "machine1",
					Namespace:       "default",
					ResourceVersion: "1000",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							DHCP: &v1alpha1.DHCP{
								MAC: "3c:ec:ef:4c:4f:54",
							},
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(false),
							},
						},
					},
				},
			},
			WantHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "machine1",
					Namespace:       "default",
					ResourceVersion: "1001",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							DHCP: &v1alpha1.DHCP{
								MAC: "3c:ec:ef:4c:4f:54",
							},
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
			AllowPXE: true,
		},
		"after workflow": {
			OriginalHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "machine1",
					Namespace:       "default",
					ResourceVersion: "1000",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							DHCP: &v1alpha1.DHCP{
								MAC: "3c:ec:ef:4c:4f:54",
							},
							Netboot: &v1alpha1.Netboot{
								AllowPXE: valueToPointer(true),
							},
						},
					},
				},
			},
			WantHardware: &v1alpha1.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "machine1",
					Namespace:       "default",
					ResourceVersion: "1001",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							DHCP: &v1alpha1.DHCP{
								MAC: "3c:ec:ef:4c:4f:54",
							},
							Netboot: &v1alpha1.Netboot{
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
			wf := &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workflow1",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					HardwareRef: "machine1",
				},
			}
			err := setAllowPXE(context.Background(), fakeClient, wf, nil, tt.AllowPXE, withDuration(func() time.Duration { return 0 }))

			got := &v1alpha1.Hardware{}
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
		seedTemplate *v1alpha1.Template
		seedWorkflow *v1alpha1.Workflow
		seedHardware *v1alpha1.Hardware
		req          reconcile.Request
		want         reconcile.Result
		wantWflow    *v1alpha1.Workflow
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
			wantWflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "999",
				},
			},
			wantErr: nil,
		},
		{
			name: "NewWorkflow",
			seedTemplate: &v1alpha1.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.TemplateSpec{
					Data: &minimalTemplate,
				},
				Status: v1alpha1.TemplateStatus{},
			},
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{},
			},
			seedHardware: &v1alpha1.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &v1alpha1.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &v1alpha1.IP{
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
			wantWflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{
					AgentID:           "3c:ec:ef:4c:4f:54",
					State:             v1alpha1.WorkflowStatePending,
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					Conditions: []v1alpha1.WorkflowCondition{
						{Type: v1alpha1.TemplateRenderedSuccess, Status: metav1.ConditionTrue, Reason: "Complete", Message: "template rendered successfully"},
					},
					Tasks: []v1alpha1.Task{
						{
							Name: "os-installation",

							AgentID: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 600,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									State: v1alpha1.WorkflowStatePending,
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
			seedTemplate: &v1alpha1.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.TemplateSpec{
					Data: &[]string{`version: "0.1"
					name: debian
global_timeout: 1800
tasks:
	- name: "os-installation"
		worker: "{{.device_1}}"`}[0],
				},
				Status: v1alpha1.TemplateStatus{},
			},
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{},
			},
			seedHardware: &v1alpha1.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &v1alpha1.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &v1alpha1.IP{
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
			wantWflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{
					State:         v1alpha1.WorkflowStatePending,
					GlobalTimeout: 1800,
					Tasks: []v1alpha1.Task{
						{
							Name: "os-installation",

							AgentID: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 600,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									State: v1alpha1.WorkflowStatePending,
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
			seedTemplate: &v1alpha1.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy",
					Namespace: "default",
				},
				Spec:   v1alpha1.TemplateSpec{},
				Status: v1alpha1.TemplateStatus{},
			},
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian", // doesn't exist
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{},
			},
			seedHardware: &v1alpha1.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &v1alpha1.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &v1alpha1.IP{
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
			wantWflow: &v1alpha1.Workflow{
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
			seedTemplate: &v1alpha1.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.TemplateSpec{
					Data: &minimalTemplate,
				},
				Status: v1alpha1.TemplateStatus{},
			},
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{
					State:               v1alpha1.WorkflowStateRunning,
					GlobalTimeout:       50,
					GlobalExecutionStop: TestTime.MetaV1BeforeSec(60),
					Tasks: []v1alpha1.Task{
						{
							Name:    "os-installation",
							AgentID: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 10,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									State:          v1alpha1.WorkflowStateRunning,
									ExecutionStart: TestTime.MetaV1BeforeSec(120),
									ExecutionStop:  TestTime.MetaV1BeforeSec(60),
								},
							},
						},
					},
				},
			},
			seedHardware: &v1alpha1.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &v1alpha1.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &v1alpha1.IP{
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
			wantWflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{
					State:               v1alpha1.WorkflowStateTimeout,
					GlobalTimeout:       50,
					GlobalExecutionStop: TestTime.MetaV1BeforeSec(60),
					Tasks: []v1alpha1.Task{
						{
							Name:    "os-installation",
							AgentID: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 10,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									State:          v1alpha1.WorkflowStateRunning,
									ExecutionStart: TestTime.MetaV1BeforeSec(120),
									ExecutionStop:  TestTime.MetaV1BeforeSec(60),
									Message:        "",
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
			seedTemplate: &v1alpha1.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.TemplateSpec{
					Data: &minimalTemplate,
				},
				Status: v1alpha1.TemplateStatus{},
			},
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareRef: "i_dont_exist",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "debian",
					Namespace: "default",
				},
			},
			want: reconcile.Result{},
			wantWflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{
					State:         v1alpha1.WorkflowStatePending,
					GlobalTimeout: 1800,
					Tasks: []v1alpha1.Task{
						{
							Name: "os-installation",

							AgentID: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 600,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									State: v1alpha1.WorkflowStatePending,
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
			seedHardware: &v1alpha1.Hardware{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "machine1",
					Namespace: "default",
				},
				Spec: v1alpha1.HardwareSpec{
					References: map[string]v1alpha1.Reference{
						"hw": {
							Name:      "machine1",
							Namespace: "default",
							Group:     "tinkerbell.org",
							Version:   "v1alpha1",
							Resource:  "hardware",
						},
					},
					Disks: []v1alpha1.Disk{
						{Device: "/dev/nvme0n1"},
					},
					Interfaces: []v1alpha1.Interface{
						{
							Netboot: &v1alpha1.Netboot{
								AllowPXE:      &[]bool{true}[0],
								AllowWorkflow: &[]bool{true}[0],
							},
							DHCP: &v1alpha1.DHCP{
								Arch:     "x86_64",
								Hostname: "sm01",
								IP: &v1alpha1.IP{
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
					Metadata:   &v1alpha1.HardwareMetadata{State: "active"},
					VendorData: valueToPointer("vendor-data"),
				},
			},
			seedTemplate: &v1alpha1.Template{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Template",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.TemplateSpec{
					Data: &templateWithDiskTemplate,
				},
				Status: v1alpha1.TemplateStatus{},
			},
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "debian",
					Namespace: "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareRef: "machine1",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "debian",
					Namespace: "default",
				},
			},
			want: reconcile.Result{},
			wantWflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "debian",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "debian",
					HardwareRef: "machine1",
					HardwareMap: map[string]string{
						"device_1": "3c:ec:ef:4c:4f:54",
					},
				},
				Status: v1alpha1.WorkflowStatus{
					AgentID:           "3c:ec:ef:4c:4f:54",
					State:             v1alpha1.WorkflowStatePending,
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					Conditions: []v1alpha1.WorkflowCondition{
						{Type: v1alpha1.TemplateRenderedSuccess, Status: metav1.ConditionTrue, Reason: "Complete", Message: "template rendered successfully"},
					},
					Tasks: []v1alpha1.Task{
						{
							Name: "os-installation",

							AgentID: "3c:ec:ef:4c:4f:54",
							Volumes: []string{
								"/dev:/dev",
								"/dev/console:/dev/console",
								"/lib/firmware:/lib/firmware:ro",
							},
							Actions: []v1alpha1.Action{
								{
									Name:    "stream-debian-image",
									Image:   "quay.io/tinkerbell-actions/image2disk:v1.0.0",
									Timeout: 600,
									Environment: map[string]string{
										"COMPRESSED": "true",
										"DEST_DISK":  "/dev/nvme0n1",
										"IMG_URL":    "http://10.1.1.11:8080/debian-10-openstack-amd64.raw.gz",
									},
									State: v1alpha1.WorkflowStatePending,
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
									State: v1alpha1.WorkflowStatePending,
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
			client:        kc.Build(),
			nowFunc:       TestTime.Now,
			dynamicClient: &fakeDynamicClient{},
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
			wflow := &v1alpha1.Workflow{}
			err := controller.client.Get(
				context.Background(),
				client.ObjectKey{Name: tc.wantWflow.Name, Namespace: tc.wantWflow.Namespace},
				wflow)
			if err != nil {
				t.Errorf("Error finding desired workflow: %v", err)
				return
			}

			if diff := cmp.Diff(wflow, tc.wantWflow, cmpopts.IgnoreFields(v1alpha1.WorkflowCondition{}, "Time"), cmpopts.IgnoreFields(v1alpha1.Task{}, "ID"), cmpopts.IgnoreFields(v1alpha1.Action{}, "ID")); diff != "" {
				t.Logf("got: %+v", wflow)
				t.Logf("want: %+v", tc.wantWflow)
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

// After Now() by int64 seconds.
func (f *FrozenTime) AfterSec(s int64) time.Time {
	return f.Now().Add(time.Duration(s) * time.Second)
}

func (f *FrozenTime) MetaV1BeforeSec(s int64) *metav1.Time {
	t := metav1.NewTime(f.BeforeSec(s))
	return &t
}

func (f *FrozenTime) MetaV1AfterSec(s int64) *metav1.Time {
	t := metav1.NewTime(f.AfterSec(s))
	return &t
}
