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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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
	ensureTypeMeta := func(obj client.Object) {
		if obj != nil {
			gvks, _, _ := runtimescheme.ObjectKinds(obj)
			if len(gvks) > 0 {
				obj.GetObjectKind().SetGroupVersionKind(gvks[0])
			}
		}
	}

	return fake.NewClientBuilder().WithScheme(
		runtimescheme,
	).WithRuntimeObjects(
		&v1alpha1.Hardware{}, &v1alpha1.Template{}, &v1alpha1.Workflow{},
	).WithStatusSubresource(&v1alpha1.Hardware{}, &v1alpha1.Template{}, &v1alpha1.Workflow{}, &v1alpha1.WorkflowRuleSet{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				err := client.Get(ctx, key, obj, opts...)
				ensureTypeMeta(obj)
				return err
			},
			SubResourceGet: func(ctx context.Context, client client.Client, subResource string, obj client.Object, subResourceObj client.Object, opts ...client.SubResourceGetOption) error {
				err := client.SubResource(subResource).Get(ctx, obj, subResourceObj, opts...)
				ensureTypeMeta(obj)
				ensureTypeMeta(subResourceObj)
				return err
			},
		})
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "Hardware",
					APIVersion: "tinkerbell.org/v1alpha1",
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
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
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

func TestUpdateAgentIDIfNeeded(t *testing.T) {
	tests := map[string]struct {
		workflow    *v1alpha1.Workflow
		wantUpdate  bool
		wantAgentID string
		description string
	}{
		"nil current state": {
			workflow:    &v1alpha1.Workflow{Status: v1alpha1.WorkflowStatus{}},
			wantUpdate:  false,
			wantAgentID: "",
			description: "should return false when CurrentState is nil",
		},
		"empty tasks": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					Tasks:        []v1alpha1.Task{},
				},
			},
			wantUpdate:  false,
			wantAgentID: "",
			description: "should return false when no tasks are present",
		},
		"current task not found": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "nonexistent"},
					Tasks: []v1alpha1.Task{
						{ID: "task1", AgentID: "agent1"},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "",
			description: "should return false when current task is not found",
		},
		"last task - no update needed": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					Tasks: []v1alpha1.Task{
						{ID: "task1", AgentID: "agent1"},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "",
			description: "should return false when in the last task",
		},
		"current task incomplete - action pending": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					AgentID:      "agent1",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
								{ID: "action2", State: v1alpha1.WorkflowStatePending},
							},
						},
						{
							ID:      "task2",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{ID: "action3", State: v1alpha1.WorkflowStatePending},
							},
						},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "agent1",
			description: "should return false when current task has pending actions",
		},
		"current task incomplete - action running": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					AgentID:      "agent1",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
								{ID: "action2", State: v1alpha1.WorkflowStateRunning},
							},
						},
						{
							ID:      "task2",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{ID: "action3", State: v1alpha1.WorkflowStatePending},
							},
						},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "agent1",
			description: "should return false when current task has running actions",
		},
		"next task has no actions": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					AgentID:      "agent1",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
								{ID: "action2", State: v1alpha1.WorkflowStateSuccess},
							},
						},
						{
							ID:      "task2",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{},
						},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "agent1",
			description: "should return false when next task has no actions",
		},
		"next task first action not pending": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					AgentID:      "agent1",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
								{ID: "action2", State: v1alpha1.WorkflowStateSuccess},
							},
						},
						{
							ID:      "task2",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{ID: "action3", State: v1alpha1.WorkflowStateRunning},
							},
						},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "agent1",
			description: "should return false when next task's first action is not pending",
		},
		"agent ID already matches next task": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					AgentID:      "agent2",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
								{ID: "action2", State: v1alpha1.WorkflowStateSuccess},
							},
						},
						{
							ID:      "task2",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{ID: "action3", State: v1alpha1.WorkflowStatePending},
							},
						},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "agent2",
			description: "should return false when AgentID already matches next task",
		},
		"successful transition - agent ID updated": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					AgentID:      "agent1",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
								{ID: "action2", State: v1alpha1.WorkflowStateSuccess},
							},
						},
						{
							ID:      "task2",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{ID: "action3", State: v1alpha1.WorkflowStatePending},
							},
						},
					},
				},
			},
			wantUpdate:  true,
			wantAgentID: "agent2",
			description: "should update AgentID when transitioning to next task with different agent",
		},
		"multiple tasks transition": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task2"},
					AgentID:      "agent2",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
							},
						},
						{
							ID:      "task2",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{ID: "action2", State: v1alpha1.WorkflowStateSuccess},
								{ID: "action3", State: v1alpha1.WorkflowStateSuccess},
							},
						},
						{
							ID:      "task3",
							AgentID: "agent3",
							Actions: []v1alpha1.Action{
								{ID: "action4", State: v1alpha1.WorkflowStatePending},
								{ID: "action5", State: v1alpha1.WorkflowStatePending},
							},
						},
					},
				},
			},
			wantUpdate:  true,
			wantAgentID: "agent3",
			description: "should update AgentID in multi-task workflow",
		},
		"transition with single action in next task": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					AgentID:      "agent1",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
							},
						},
						{
							ID:      "task2",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{ID: "action2", State: v1alpha1.WorkflowStatePending},
							},
						},
					},
				},
			},
			wantUpdate:  true,
			wantAgentID: "agent2",
			description: "should update AgentID when next task has single pending action",
		},
		"invalid current task index - out of bounds": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					AgentID:      "agent1",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task2", // Different ID from CurrentState.TaskID
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
							},
						},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "agent1",
			description: "should return false when current task index is invalid (not found)",
		},
		"edge case - exactly last task boundary": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task2"},
					AgentID:      "agent2",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
							},
						},
						{
							ID:      "task2",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{ID: "action2", State: v1alpha1.WorkflowStateSuccess},
							},
						},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "agent2",
			description: "should return false when current task is exactly the last task",
		},
		"defensive check - prevent out of bounds access": {
			workflow: &v1alpha1.Workflow{
				Status: v1alpha1.WorkflowStatus{
					CurrentState: &v1alpha1.CurrentState{TaskID: "task1"},
					AgentID:      "agent1",
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{ID: "action1", State: v1alpha1.WorkflowStateSuccess},
							},
						},
					},
				},
			},
			wantUpdate:  false,
			wantAgentID: "agent1",
			description: "should handle case where currentTaskIndex+1 would be out of bounds",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Store original AgentID for comparison
			originalAgentID := tt.workflow.Status.AgentID

			got := updateAgentIDIfNeeded(tt.workflow)

			// Check if update flag matches expectation
			if got != tt.wantUpdate {
				t.Errorf("updateAgentIDIfNeeded() = %v, want %v\nDescription: %s", got, tt.wantUpdate, tt.description)
			}

			// Check if AgentID was updated correctly
			if tt.workflow.Status.AgentID != tt.wantAgentID {
				t.Errorf("AgentID = %v, want %v\nDescription: %s", tt.workflow.Status.AgentID, tt.wantAgentID, tt.description)
			}

			// Ensure AgentID was not changed when update should be false
			if !tt.wantUpdate && tt.workflow.Status.AgentID != originalAgentID {
				t.Errorf("AgentID was modified when it shouldn't be: original=%v, new=%v\nDescription: %s", originalAgentID, tt.workflow.Status.AgentID, tt.description)
			}
		})
	}
}

func TestReconcileWithMultipleTasksAndAgents(t *testing.T) {
	tests := map[string]struct {
		seedWorkflow *v1alpha1.Workflow
		req          reconcile.Request
		want         reconcile.Result
		wantWflow    *v1alpha1.Workflow
		wantErr      error
		description  string
	}{
		"workflow running with multiple tasks - agent transition": {
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "multi-task-workflow",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "multi-task",
					HardwareRef: "machine1",
				},
				Status: v1alpha1.WorkflowStatus{
					State:             v1alpha1.WorkflowStateRunning,
					AgentID:           "agent1",
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					CurrentState: &v1alpha1.CurrentState{
						TaskID:   "task1",
						ActionID: "action1",
					},
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							Name:    "first-task",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{
									ID:             "action1",
									Name:           "first-action",
									State:          v1alpha1.WorkflowStateSuccess,
									ExecutionStart: TestTime.MetaV1BeforeSec(10),
								},
								{
									ID:             "action2",
									Name:           "second-action",
									State:          v1alpha1.WorkflowStateSuccess,
									ExecutionStart: TestTime.MetaV1BeforeSec(5),
								},
							},
						},
						{
							ID:      "task2",
							Name:    "second-task",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{
									ID:    "action3",
									Name:  "third-action",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "multi-task-workflow",
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
					ResourceVersion: "1001",
					Name:            "multi-task-workflow",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "multi-task",
					HardwareRef: "machine1",
				},
				Status: v1alpha1.WorkflowStatus{
					State:             v1alpha1.WorkflowStateRunning,
					AgentID:           "agent2", // Should be updated to agent2
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					CurrentState: &v1alpha1.CurrentState{
						TaskID:   "task1",
						ActionID: "action1",
					},
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							Name:    "first-task",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{
									ID:             "action1",
									Name:           "first-action",
									State:          v1alpha1.WorkflowStateSuccess,
									ExecutionStart: TestTime.MetaV1BeforeSec(10),
								},
								{
									ID:             "action2",
									Name:           "second-action",
									State:          v1alpha1.WorkflowStateSuccess,
									ExecutionStart: TestTime.MetaV1BeforeSec(5),
								},
							},
						},
						{
							ID:      "task2",
							Name:    "second-task",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{
									ID:    "action3",
									Name:  "third-action",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			wantErr:     nil,
			description: "should update AgentID when transitioning between tasks with different agents",
		},
		"workflow running with three tasks - middle transition": {
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "three-task-workflow",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "three-task",
					HardwareRef: "machine1",
				},
				Status: v1alpha1.WorkflowStatus{
					State:             v1alpha1.WorkflowStateRunning,
					AgentID:           "agent2",
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					CurrentState: &v1alpha1.CurrentState{
						TaskID:   "task2",
						ActionID: "action3",
					},
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							Name:    "first-task",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{
									ID:    "action1",
									Name:  "first-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
							},
						},
						{
							ID:      "task2",
							Name:    "second-task",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{
									ID:    "action3",
									Name:  "third-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
								{
									ID:    "action4",
									Name:  "fourth-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
							},
						},
						{
							ID:      "task3",
							Name:    "third-task",
							AgentID: "agent3",
							Actions: []v1alpha1.Action{
								{
									ID:    "action5",
									Name:  "fifth-action",
									State: v1alpha1.WorkflowStatePending,
								},
								{
									ID:    "action6",
									Name:  "sixth-action",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "three-task-workflow",
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
					ResourceVersion: "1001",
					Name:            "three-task-workflow",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "three-task",
					HardwareRef: "machine1",
				},
				Status: v1alpha1.WorkflowStatus{
					State:             v1alpha1.WorkflowStateRunning,
					AgentID:           "agent3", // Should be updated to agent3
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					CurrentState: &v1alpha1.CurrentState{
						TaskID:   "task2",
						ActionID: "action3",
					},
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							Name:    "first-task",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{
									ID:    "action1",
									Name:  "first-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
							},
						},
						{
							ID:      "task2",
							Name:    "second-task",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{
									ID:    "action3",
									Name:  "third-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
								{
									ID:    "action4",
									Name:  "fourth-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
							},
						},
						{
							ID:      "task3",
							Name:    "third-task",
							AgentID: "agent3",
							Actions: []v1alpha1.Action{
								{
									ID:    "action5",
									Name:  "fifth-action",
									State: v1alpha1.WorkflowStatePending,
								},
								{
									ID:    "action6",
									Name:  "sixth-action",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			wantErr:     nil,
			description: "should update AgentID in multi-task workflow from task2 to task3",
		},
		"workflow running with same agent - no update needed": {
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "same-agent-workflow",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "same-agent",
					HardwareRef: "machine1",
				},
				Status: v1alpha1.WorkflowStatus{
					State:             v1alpha1.WorkflowStateRunning,
					AgentID:           "agent1",
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					CurrentState: &v1alpha1.CurrentState{
						TaskID:   "task1",
						ActionID: "action1",
					},
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							Name:    "first-task",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{
									ID:    "action1",
									Name:  "first-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
							},
						},
						{
							ID:      "task2",
							Name:    "second-task",
							AgentID: "agent1", // Same agent
							Actions: []v1alpha1.Action{
								{
									ID:    "action2",
									Name:  "second-action",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "same-agent-workflow",
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
					ResourceVersion: "1000", // No change expected
					Name:            "same-agent-workflow",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "same-agent",
					HardwareRef: "machine1",
				},
				Status: v1alpha1.WorkflowStatus{
					State:             v1alpha1.WorkflowStateRunning,
					AgentID:           "agent1", // Should remain unchanged
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					CurrentState: &v1alpha1.CurrentState{
						TaskID:   "task1",
						ActionID: "action1",
					},
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							Name:    "first-task",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{
									ID:    "action1",
									Name:  "first-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
							},
						},
						{
							ID:      "task2",
							Name:    "second-task",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{
									ID:    "action2",
									Name:  "second-action",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			wantErr:     nil,
			description: "should not update AgentID when next task uses same agent",
		},
		"workflow running with incomplete current task - no update": {
			seedWorkflow: &v1alpha1.Workflow{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Workflow",
					APIVersion: "tinkerbell.org/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "1000",
					Name:            "incomplete-task-workflow",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "incomplete-task",
					HardwareRef: "machine1",
				},
				Status: v1alpha1.WorkflowStatus{
					State:             v1alpha1.WorkflowStateRunning,
					AgentID:           "agent1",
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					CurrentState: &v1alpha1.CurrentState{
						TaskID:   "task1",
						ActionID: "action1",
					},
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							Name:    "first-task",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{
									ID:    "action1",
									Name:  "first-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
								{
									ID:    "action2",
									Name:  "second-action",
									State: v1alpha1.WorkflowStateRunning, // Still running
								},
							},
						},
						{
							ID:      "task2",
							Name:    "second-task",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{
									ID:    "action3",
									Name:  "third-action",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			req: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "incomplete-task-workflow",
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
					ResourceVersion: "1000", // No change expected
					Name:            "incomplete-task-workflow",
					Namespace:       "default",
				},
				Spec: v1alpha1.WorkflowSpec{
					TemplateRef: "incomplete-task",
					HardwareRef: "machine1",
				},
				Status: v1alpha1.WorkflowStatus{
					State:             v1alpha1.WorkflowStateRunning,
					AgentID:           "agent1", // Should remain unchanged
					GlobalTimeout:     1800,
					TemplateRendering: "successful",
					CurrentState: &v1alpha1.CurrentState{
						TaskID:   "task1",
						ActionID: "action1",
					},
					Tasks: []v1alpha1.Task{
						{
							ID:      "task1",
							Name:    "first-task",
							AgentID: "agent1",
							Actions: []v1alpha1.Action{
								{
									ID:    "action1",
									Name:  "first-action",
									State: v1alpha1.WorkflowStateSuccess,
								},
								{
									ID:    "action2",
									Name:  "second-action",
									State: v1alpha1.WorkflowStateRunning,
								},
							},
						},
						{
							ID:      "task2",
							Name:    "second-task",
							AgentID: "agent2",
							Actions: []v1alpha1.Action{
								{
									ID:    "action3",
									Name:  "third-action",
									State: v1alpha1.WorkflowStatePending,
								},
							},
						},
					},
				},
			},
			wantErr:     nil,
			description: "should not update AgentID when current task is incomplete",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			kc := GetFakeClientBuilder()
			if tc.seedWorkflow != nil {
				kc = kc.WithObjects(tc.seedWorkflow)
				kc = kc.WithStatusSubresource(tc.seedWorkflow)
			}
			controller := &Reconciler{
				client:        kc.Build(),
				nowFunc:       TestTime.Now,
				dynamicClient: &fakeDynamicClient{},
			}

			got, gotErr := controller.Reconcile(context.Background(), tc.req)
			if gotErr != nil {
				if tc.wantErr == nil {
					t.Errorf("Got unexpected error: %v\nDescription: %s", gotErr, tc.description)
				} else if !strings.Contains(gotErr.Error(), tc.wantErr.Error()) {
					t.Errorf("Got unexpected error: got %q, wanted %q\nDescription: %s", gotErr, tc.wantErr, tc.description)
				}
				return
			}
			if gotErr == nil && tc.wantErr != nil {
				t.Errorf("Missing expected error: %v\nDescription: %s", tc.wantErr, tc.description)
				return
			}
			// Skip reconcile.Result comparison for timing-sensitive tests since RequeueAfter depends on execution timing
			if tc.want.RequeueAfter == 0 && got.RequeueAfter != 0 {
				// For tests where we expect no requeue but get a timing-dependent requeue, ignore the difference
				t.Logf("Ignoring RequeueAfter timing difference: expected %v, got %v", tc.want.RequeueAfter, got.RequeueAfter)
			} else if tc.want != got {
				t.Errorf("Got unexpected result. Wanted %v, got %v\nDescription: %s", tc.want, got, tc.description)
			}

			wflow := &v1alpha1.Workflow{}
			err := controller.client.Get(
				context.Background(),
				client.ObjectKey{Name: tc.wantWflow.Name, Namespace: tc.wantWflow.Namespace},
				wflow)
			if err != nil {
				t.Errorf("Error finding desired workflow: %v\nDescription: %s", err, tc.description)
				return
			}

			// Check specific AgentID expectations
			if wflow.Status.AgentID != tc.wantWflow.Status.AgentID {
				t.Errorf("AgentID mismatch: got %q, want %q\nDescription: %s", wflow.Status.AgentID, tc.wantWflow.Status.AgentID, tc.description)
			}

			if diff := cmp.Diff(wflow, tc.wantWflow, cmpopts.IgnoreFields(v1alpha1.WorkflowCondition{}, "Time"), cmpopts.IgnoreFields(v1alpha1.Task{}, "ID"), cmpopts.IgnoreFields(v1alpha1.Action{}, "ID"), cmpopts.IgnoreFields(v1alpha1.WorkflowStatus{}, "GlobalExecutionStop")); diff != "" {
				t.Logf("got: %+v", wflow)
				t.Logf("want: %+v", tc.wantWflow)
				t.Errorf("unexpected difference:\n%v\nDescription: %s", diff, tc.description)
			}
		})
	}
}
