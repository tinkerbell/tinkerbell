package kube

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/template/render"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRenderHardware(t *testing.T) {
	const tmpl = "{{ .references.net.domain }}"
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "machine1",
			Namespace:   "tink",
			Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": tmpl},
		},
		Spec: tinkerbell.HardwareSpec{
			AgentID:    tmpl,
			References: map[string]tinkerbell.Reference{"net": {Name: tmpl}},
			Interfaces: []tinkerbell.Interface{{DHCP: &tinkerbell.DHCP{
				MAC:      tmpl,
				IP:       &tinkerbell.IP{Address: tmpl, Gateway: "{{ .references.net.gateway }}"},
				Hostname: "{{ .hardware.metadata.name }}",
			}}},
			Metadata: &tinkerbell.HardwareMetadata{Instance: &tinkerbell.MetadataInstance{
				ID:       tmpl,
				Hostname: "{{ .hardware.metadata.name }}." + tmpl,
			}},
			UserData: ptr("fqdn: {{ .hardware.spec.metadata.instance.hostname }}"),
		},
		Status: tinkerbell.HardwareStatus{State: "active"},
	}
	orig := hw.DeepCopy()
	refs := map[string]any{"net": map[string]any{"domain": "example.org", "gateway": "10.0.0.1"}}

	got, err := renderHardware(hw, refs)
	if err != nil {
		t.Fatal(err)
	}

	want := orig.DeepCopy()
	want.Spec.Interfaces[0].DHCP.IP.Gateway = "10.0.0.1"
	want.Spec.Interfaces[0].DHCP.Hostname = "machine1"
	want.Spec.Metadata.Instance.Hostname = "machine1.example.org"
	want.Spec.UserData = ptr("fqdn: machine1.example.org")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("rendered (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(orig, hw); diff != "" {
		t.Errorf("input was modified (-want +got):\n%s", diff)
	}
}

func TestRenderHardwareObjectData(t *testing.T) {
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{"rack": "rack-1"},
			Annotations: map[string]string{"template": "{{ .references.missing }}"},
		},
		Spec: tinkerbell.HardwareSpec{
			UserData: ptr("{{ .hardware.metadata.labels.rack }} {{ .hardware.metadata.annotations.template }} {{ .hardware.status.state }} {{ .hardware.status.attributes.inBand.collectionMethod }} {{ .hardware.status.attributes.outOfBand.collectionMethod }}"),
		},
		Status: tinkerbell.HardwareStatus{
			State: "active",
			Attributes: &tinkerbell.HardwareAttributes{
				InBand:    &tinkerbell.Attributes{CollectionMethod: "agent"},
				OutOfBand: &tinkerbell.Attributes{CollectionMethod: "{{ .references.missing }}"},
			},
		},
	}
	orig := hw.DeepCopy()

	got, err := renderHardware(hw, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := orig.DeepCopy()
	want.Spec.UserData = ptr("rack-1 {{ .references.missing }} active agent {{ .references.missing }}")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("rendered (-want +got):\n%s", diff)
	}
	got.Labels["rack"] = "rack-2"
	got.Status.Attributes.InBand.CollectionMethod = "changed"
	if diff := cmp.Diff(orig, hw); diff != "" {
		t.Errorf("input was modified (-want +got):\n%s", diff)
	}
}

func TestRenderHardwareBinary(t *testing.T) {
	raw := "\x30\x82\x00\xff\xfe"
	hw := &tinkerbell.Hardware{Spec: tinkerbell.HardwareSpec{UserData: ptr(`{{ .references.s.data.der | b64dec }}`)}}
	refs := map[string]any{"s": map[string]any{"data": map[string]any{"der": base64.StdEncoding.EncodeToString([]byte(raw))}}}

	got, err := renderHardware(hw, refs)
	if err != nil {
		t.Fatal(err)
	}
	if *got.Spec.UserData != raw {
		t.Fatalf("userData = %q, want %q", *got.Spec.UserData, raw)
	}
}

func TestRenderHardwareNothingToRender(t *testing.T) {
	hw := &tinkerbell.Hardware{Spec: tinkerbell.HardwareSpec{
		UserData: ptr("#cloud-config"),
		// References used only by Workflow Templates do not make Hardware templated.
		References: map[string]tinkerbell.Reference{"net": {Name: "net1"}},
	}}

	got, err := renderHardware(hw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != hw {
		t.Fatal("expected the stored object to be returned unchanged")
	}
}

func TestRenderHardwareError(t *testing.T) {
	hw := &tinkerbell.Hardware{Spec: tinkerbell.HardwareSpec{UserData: ptr("{{ .references.missing.x }}")}}

	_, err := renderHardware(hw, map[string]any{})
	var fe *render.FieldError
	if !errors.As(err, &fe) || fe.Path != "spec.userData" {
		t.Fatalf("err = %v, want *render.FieldError at spec.userData", err)
	}
}

func TestRenderHardwareSkipAnnotation(t *testing.T) {
	const jinja = "## template: jinja\n#cloud-config\nhostname: {{ ds.meta_data.hostname }}\n"
	for _, test := range []struct {
		name       string
		annotation string
		vendorData *string
		wantVendor *string
		unchanged  bool
	}{
		{name: "Jinja only", annotation: `["spec.userData"]`, unchanged: true},
		{name: "duplicate and absent optional paths", annotation: `["spec.userData", "spec.userData", "spec.vendorData"]`, unchanged: true},
		{name: "mixed fields", annotation: `["spec.userData"]`, vendorData: ptr("{{ .hardware.metadata.name }}"), wantVendor: ptr("machine1")},
		{name: "skipped content is readable", annotation: `["spec.userData"]`, vendorData: ptr("{{ .hardware.spec.userData }}"), wantVendor: ptr(jinja)},
	} {
		t.Run(test.name, func(t *testing.T) {
			hw := &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{Name: "machine1", Annotations: map[string]string{templateSkipAnnotation: test.annotation}},
				Spec:       tinkerbell.HardwareSpec{UserData: ptr(jinja), VendorData: test.vendorData},
			}
			orig := hw.DeepCopy()
			got, err := renderHardware(hw, nil)
			if err != nil {
				t.Fatal(err)
			}
			if test.unchanged && got != hw {
				t.Fatal("expected the stored object to be returned unchanged")
			}
			want := orig.DeepCopy()
			want.Spec.VendorData = test.wantVendor
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("rendered (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(orig, hw); diff != "" {
				t.Errorf("input was modified (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRenderHardwareSkipAnnotationInvalid(t *testing.T) {
	for _, annotation := range []string{
		"", "null", `{}`, `"spec.userData"`, `[1]`, `[null]`, `[""]`,
		`["metadata.name"]`, `["status.state"]`, `["spec"]`, `["spec."]`,
		`["spec.userData"] trailing`,
	} {
		t.Run(annotation, func(t *testing.T) {
			hw := &tinkerbell.Hardware{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{templateSkipAnnotation: annotation}}}
			_, err := renderHardware(hw, nil)
			if err == nil || !strings.Contains(err.Error(), templateSkipAnnotation) {
				t.Fatalf("err = %v, want annotation validation error even without templates", err)
			}
		})
	}
}

func TestRenderHardwareSkipAnnotationChanged(t *testing.T) {
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{templateSkipAnnotation: `["spec.userData"]`}},
		Spec:       tinkerbell.HardwareSpec{UserData: ptr("{{ ds.meta_data.hostname }}")},
	}
	if got, err := renderHardware(hw, nil); err != nil || got != hw {
		t.Fatalf("initial render = %v, %v; want unchanged hardware", got, err)
	}
	for _, annotation := range []string{`[]`, `["spec.vendorData"]`} {
		hw.Annotations[templateSkipAnnotation] = annotation
		if _, err := renderHardware(hw, nil); err == nil {
			t.Fatalf("annotation %s: expected Jinja parse error", annotation)
		}
	}
	delete(hw.Annotations, templateSkipAnnotation)
	if _, err := renderHardware(hw, nil); err == nil {
		t.Fatal("expected Jinja parse error after removing annotation")
	}
}

func TestRenderHardwareSkipAnnotationExactPath(t *testing.T) {
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{templateSkipAnnotation: `["spec.interfaces[0].dhcp.hostname"]`}},
		Spec: tinkerbell.HardwareSpec{Interfaces: []tinkerbell.Interface{
			{DHCP: &tinkerbell.DHCP{Hostname: "{{ ds.meta_data.hostname }}"}},
			{DHCP: &tinkerbell.DHCP{Hostname: `{{ "machine2" }}`}},
		}},
	}
	got, err := renderHardware(hw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Interfaces[0].DHCP.Hostname != hw.Spec.Interfaces[0].DHCP.Hostname || got.Spec.Interfaces[1].DHCP.Hostname != "machine2" {
		t.Fatalf("unexpected hostnames: %+v", got.Spec.Interfaces)
	}
}

func TestRenderHardwareOnlySkippedFields(t *testing.T) {
	for _, annotation := range []string{`[]`, `["spec.vendorData"]`} {
		hw := &tinkerbell.Hardware{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "{{ invalid }}",
				Annotations: map[string]string{templateSkipAnnotation: annotation},
			},
			Spec: tinkerbell.HardwareSpec{
				AgentID:    "{{ invalid }}",
				References: map[string]tinkerbell.Reference{"net": {Name: "{{ invalid }}"}},
				Metadata:   &tinkerbell.HardwareMetadata{Instance: &tinkerbell.MetadataInstance{ID: "{{ invalid }}"}},
				Interfaces: []tinkerbell.Interface{{DHCP: &tinkerbell.DHCP{
					MAC: "{{ invalid }}", IP: &tinkerbell.IP{Address: "{{ invalid }}"},
				}}},
			},
			Status: tinkerbell.HardwareStatus{State: "{{ invalid }}"},
		}
		if got, err := renderHardware(hw, nil); err != nil || got != hw {
			t.Fatalf("annotation %s: render = %v, %v; want unchanged hardware", annotation, got, err)
		}
	}
}

func ptr[T any](v T) *T { return &v }
