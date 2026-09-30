package kube

import (
	"encoding/base64"
	"errors"
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

func ptr[T any](v T) *T { return &v }
