package kube

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/template/render"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

func TestRenderHardwareNonJSONReferences(t *testing.T) {
	hw := &tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{UserData: ptr("{{ .references.net.cores }} {{ .references.net.labels.rack }}")},
	}
	refs := map[string]any{"net": map[string]any{"cores": uint64(8), "labels": map[string]string{"rack": "r1"}}}

	got, err := renderHardware(hw, refs)
	if err != nil {
		t.Fatal(err)
	}
	if want := "8 r1"; got.Spec.UserData == nil || *got.Spec.UserData != want {
		t.Errorf("userData = %v, want %q", got.Spec.UserData, want)
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

func TestRenderHardwareWithUnsignedAttributes(t *testing.T) {
	hw := &tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{UserData: ptr("cores={{ .hardware.status.attributes.inBand.cpu.totalCores }}")},
		Status: tinkerbell.HardwareStatus{Attributes: &tinkerbell.HardwareAttributes{
			InBand: &tinkerbell.Attributes{CPU: &tinkerbell.CPU{TotalCores: 8}},
		}},
	}

	got, err := renderHardware(hw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.UserData == nil || *got.Spec.UserData != "cores=8" {
		t.Fatalf("UserData = %v, want cores=8", got.Spec.UserData)
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
				References: map[string]tinkerbell.Reference{"net.name": {Name: "{{ invalid }}"}},
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

func TestSkipRender(t *testing.T) {
	for _, test := range []struct {
		path string
		want bool
	}{
		{path: "apiVersion", want: true},
		{path: "kind", want: true},
		{path: "metadata.name", want: true},
		{path: `metadata.annotations["a.b"]`, want: true},
		{path: "status.attributes.inBand.collectionMethod", want: true},
		{path: "futureRoot.value", want: true},
		{path: "spec.references", want: true},
		{path: "spec.references.net.name", want: true},
		{path: `spec.references["net.name"].name`, want: true},
		{path: "spec.referencesOther.name"},
		{path: "spec.agentID", want: true},
		{path: "spec.agentIDOther"},
		{path: "spec.metadata.instance.id", want: true},
		{path: "spec.metadata.instance.hostname"},
		{path: "spec.interfaces[0].dhcp.mac", want: true},
		{path: "spec.interfaces[12].dhcp.ip.address", want: true},
		{path: "spec.interfaces[0].dhcp.hostname"},
		{path: "spec.interfaces[0].dhcp.ip.gateway"},
		{path: "spec.interfaces[0].dhcp.macOther"},
		{path: "spec.interfacesOther[0].dhcp.mac"},
		{path: "spec.interfaces[].dhcp.mac"},
		{path: "spec.interfaces[*].dhcp.mac"},
		{path: "spec.interfaces[-1].dhcp.mac"},
		{path: "spec.userData"},
	} {
		t.Run(test.path, func(t *testing.T) {
			if got := skipRender(test.path); got != test.want {
				t.Fatalf("skipRender(%q) = %v, want %v", test.path, got, test.want)
			}
		})
	}
}

func TestMatchesSkippedPath(t *testing.T) {
	for _, test := range []struct {
		path string
		skip string
		want bool
	}{
		{path: "metadata", skip: "metadata", want: true},
		{path: "metadata.labels.rack", skip: "metadata", want: true},
		{path: `metadata["a.b"]`, skip: "metadata", want: true},
		{path: "metadataOther.name", skip: "metadata"},
		{path: "groups[1].interfaces[2].dhcp.mac", skip: "groups[].interfaces[].dhcp.mac", want: true},
		{path: "groups[1].interfaces[2].dhcp.hostname", skip: "groups[].interfaces[].dhcp.mac"},
		{path: "groups[1].interfaces[2].dhcp.mac", skip: "groups[*].interfaces[*].dhcp.mac"},
	} {
		t.Run(test.path+"/"+test.skip, func(t *testing.T) {
			if got := matchesSkippedPath(test.path, test.skip); got != test.want {
				t.Fatalf("matchesSkippedPath(%q, %q) = %v, want %v", test.path, test.skip, got, test.want)
			}
		})
	}
}

func TestRenderHardwareProtectedMutation(t *testing.T) {
	for _, test := range []struct {
		name     string
		template string
		path     string
	}{
		{name: "metadata", template: `{{ $_ := set .hardware.metadata "name" "other" }}`, path: "metadata"},
		{name: "incompatible type", template: `{{ $_ := set .hardware.metadata "name" 42 }}`, path: "metadata"},
		{name: "unset", template: `{{ $_ := unset .hardware.metadata "name" }}`, path: "metadata"},
		{name: "merge", template: `{{ $_ := merge .hardware.metadata (dict "new" "value") }}`, path: "metadata"},
		{name: "merge overwrite", template: `{{ $_ := mergeOverwrite .hardware.metadata (dict "name" "other") }}`, path: "metadata"},
		{name: "lookup key", template: `{{ $_ := set .hardware.spec.metadata.instance "id" "other" }}`, path: "spec.metadata.instance.id"},
		{name: "MAC", template: `{{ $_ := set (index .hardware.spec.interfaces 0).dhcp "mac" "other" }}`, path: "spec.interfaces[0].dhcp.mac"},
		{name: "IP", template: `{{ $_ := set (index .hardware.spec.interfaces 0).dhcp.ip "address" "other" }}`, path: "spec.interfaces[0].dhcp.ip.address"},
		{name: "references", template: `{{ $_ := unset .hardware.spec.references "net" }}`, path: "spec.references"},
		{name: "status", template: `{{ $_ := set .hardware.status "state" "other" }}`, path: "status"},
	} {
		t.Run(test.name, func(t *testing.T) {
			hw := &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{Name: "machine1", Namespace: "tink"},
				Spec: tinkerbell.HardwareSpec{
					UserData:   ptr(test.template),
					Metadata:   &tinkerbell.HardwareMetadata{Instance: &tinkerbell.MetadataInstance{ID: "machine1"}},
					References: map[string]tinkerbell.Reference{"net": {Name: "network"}},
					Interfaces: []tinkerbell.Interface{{DHCP: &tinkerbell.DHCP{
						MAC: "00:11:22:33:44:55", IP: &tinkerbell.IP{Address: "10.0.0.1"},
					}}},
				},
				Status: tinkerbell.HardwareStatus{State: "active"},
			}
			original := hw.DeepCopy()
			got, err := renderHardware(hw, nil)
			if got != nil || err == nil || !strings.Contains(err.Error(), fmt.Sprintf("protected field %q changed", test.path)) {
				t.Fatalf("render = %v, %v; want protected mutation error", got, err)
			}
			if diff := cmp.Diff(original, hw); diff != "" {
				t.Errorf("input was modified (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRenderHardwareNumericHelperMutation(t *testing.T) {
	for _, test := range []struct {
		name     string
		template string
		path     string
	}{
		{name: "allowed lease time", template: `{{ $_ := set (index .hardware.spec.interfaces 0).dhcp "lease_time" 42 }}`},
		{name: "protected MAC", template: `{{ $_ := set (index .hardware.spec.interfaces 0).dhcp "mac" 42 }}`, path: "spec.interfaces[0].dhcp.mac"},
		{name: "protected IP", template: `{{ $_ := set (index .hardware.spec.interfaces 0).dhcp.ip "address" 42 }}`, path: "spec.interfaces[0].dhcp.ip.address"},
	} {
		t.Run(test.name, func(t *testing.T) {
			hw := &tinkerbell.Hardware{Spec: tinkerbell.HardwareSpec{
				UserData: ptr(test.template),
				Interfaces: []tinkerbell.Interface{{DHCP: &tinkerbell.DHCP{
					MAC: "00:11:22:33:44:55", IP: &tinkerbell.IP{Address: "10.0.0.1"},
				}}},
			}}
			original := hw.DeepCopy()
			got, err := renderHardware(hw, nil)
			if test.path == "" {
				if err != nil {
					t.Fatal(err)
				}
				if got.Spec.Interfaces[0].DHCP.LeaseTime != 42 {
					t.Fatalf("lease time = %d, want 42", got.Spec.Interfaces[0].DHCP.LeaseTime)
				}
			} else if got != nil || err == nil || !strings.Contains(err.Error(), fmt.Sprintf("protected field %q changed", test.path)) {
				t.Fatalf("render = %v, %v; want protected mutation error", got, err)
			}
			if diff := cmp.Diff(original, hw); diff != "" {
				t.Errorf("input was modified (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateProtectedHardware(t *testing.T) {
	const hardwareKind = "Hardware"
	hw := &tinkerbell.Hardware{
		TypeMeta:   metav1.TypeMeta{APIVersion: "tinkerbell.org/v1alpha1", Kind: hardwareKind},
		ObjectMeta: metav1.ObjectMeta{Name: "machine1"},
		Spec: tinkerbell.HardwareSpec{
			AgentID:    "agent1",
			Metadata:   &tinkerbell.HardwareMetadata{Instance: &tinkerbell.MetadataInstance{ID: "instance1"}},
			References: map[string]tinkerbell.Reference{"net": {Name: "network"}},
			Interfaces: []tinkerbell.Interface{{DHCP: &tinkerbell.DHCP{
				MAC: "00:11:22:33:44:55", IP: &tinkerbell.IP{Address: "10.0.0.1"},
			}}},
		},
		Status: tinkerbell.HardwareStatus{State: "active"},
	}
	doc, err := runtime.DefaultUnstructuredConverter.ToUnstructured(hw)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		path   []string
		value  any
		remove bool
		absent bool
		want   string
	}{
		{name: "apiVersion", path: []string{"apiVersion"}, value: "other", want: "apiVersion"},
		{name: "kind", path: []string{"kind"}, value: "other", want: "kind"},
		{name: "metadata removed", path: []string{"metadata"}, remove: true, want: "metadata"},
		{name: "status null", path: []string{"status"}, want: "status"},
		{name: "status added", path: []string{"status"}, absent: true, value: map[string]any{"state": "new"}, want: "status"},
		{name: "agentID changed", path: []string{"spec", "agentID"}, value: "other", want: "spec.agentID"},
		{name: "agentID removed", path: []string{"spec", "agentID"}, remove: true, want: "spec.agentID"},
		{name: "agentID added", path: []string{"spec", "agentID"}, absent: true, value: "other", want: "spec.agentID"},
		{name: "agentID absent to null", path: []string{"spec", "agentID"}, absent: true, want: "spec.agentID"},
		{name: "references removed", path: []string{"spec", "references"}, remove: true, want: "spec.references"},
		{name: "instance parent removed", path: []string{"spec", "metadata", "instance"}, remove: true, want: "spec.metadata.instance.id"},
		{name: "instance parent replaced", path: []string{"spec", "metadata", "instance"}, value: "other", want: "spec.metadata.instance.id"},
		{name: "spec parent replaced", path: []string{"spec"}, value: "other", want: "spec.references"},
		{name: "interfaces removed", path: []string{"spec", "interfaces"}, remove: true, want: "spec.interfaces[0].dhcp.mac"},
		{name: "interfaces replaced", path: []string{"spec", "interfaces"}, value: "other", want: "spec.interfaces"},
		{name: "interfaces shortened", path: []string{"spec", "interfaces"}, value: []any{}, want: "spec.interfaces[0].dhcp.mac"},
		{name: "interface replaced", path: []string{"spec", "interfaces"}, value: []any{"other"}, want: "spec.interfaces[0]"},
		{name: "DHCP parent replaced", path: []string{"spec", "interfaces"}, value: []any{map[string]any{"dhcp": "other"}}, want: "spec.interfaces[0].dhcp.mac"},
		{name: "unprotected field", path: []string{"spec", "userData"}, value: "rendered"},
		{name: "unprotected sibling", path: []string{"spec", "metadata", "instance", "hostname"}, value: "rendered"},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := runtime.DeepCopyJSON(doc)
			if test.absent {
				unstructured.RemoveNestedField(before, test.path...)
			}
			after := runtime.DeepCopyJSON(before)
			if test.remove {
				unstructured.RemoveNestedField(after, test.path...)
			} else if err := unstructured.SetNestedField(after, test.value, test.path...); err != nil {
				t.Fatal(err)
			}
			err := validateProtectedHardware(before, after)
			if test.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || err.Error() != fmt.Sprintf("protected field %q changed", test.want) {
				t.Fatalf("err = %v, want protected field %q changed", err, test.want)
			}
		})
	}
	for _, test := range []struct {
		name            string
		mutate          func([]any) []any
		want            string
		absentIP        bool
		secondInterface bool
	}{
		{name: "IP parent replaced", mutate: func(interfaces []any) []any {
			interfaces[0].(map[string]any)["dhcp"].(map[string]any)["ip"] = "other"
			return interfaces
		}, want: "spec.interfaces[0].dhcp.ip.address"},
		{name: "new interface with lookup key", mutate: func(interfaces []any) []any {
			return append(interfaces, map[string]any{"dhcp": map[string]any{"mac": "other"}})
		}, want: "spec.interfaces[1].dhcp.mac"},
		{name: "lookup key added", mutate: func(interfaces []any) []any {
			interfaces[0].(map[string]any)["dhcp"].(map[string]any)["ip"].(map[string]any)["address"] = "new"
			return interfaces
		}, want: "spec.interfaces[0].dhcp.ip.address", absentIP: true},
		{name: "interfaces reordered", mutate: func(interfaces []any) []any {
			return []any{interfaces[1], interfaces[0]}
		}, want: "spec.interfaces[0].dhcp.mac", secondInterface: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := runtime.DeepCopyJSON(doc)
			if test.absentIP {
				interfaces := before["spec"].(map[string]any)["interfaces"].([]any)
				delete(interfaces[0].(map[string]any)["dhcp"].(map[string]any)["ip"].(map[string]any), "address")
			}
			if test.secondInterface {
				spec := before["spec"].(map[string]any)
				spec["interfaces"] = append(spec["interfaces"].([]any), map[string]any{"dhcp": map[string]any{"mac": "other"}})
			}
			after := runtime.DeepCopyJSON(before)
			spec := after["spec"].(map[string]any)
			spec["interfaces"] = test.mutate(spec["interfaces"].([]any))
			if err := validateProtectedHardware(before, after); err == nil || err.Error() != fmt.Sprintf("protected field %q changed", test.want) {
				t.Fatalf("err = %v, want protected field %q changed", err, test.want)
			}
		})
	}
}

func TestChangedProtectedPathArrays(t *testing.T) {
	before := map[string]any{"groups": []any{map[string]any{
		"interfaces": []any{map[string]any{"mac": "original", "hostname": "literal"}},
	}}}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{name: "unchanged", mutate: func(map[string]any) {}},
		{name: "nested element", mutate: func(after map[string]any) {
			group := after["groups"].([]any)[0].(map[string]any)
			group["interfaces"].([]any)[0].(map[string]any)["mac"] = "changed"
		}, want: "groups[0].interfaces[0].mac"},
		{name: "nested array replaced", mutate: func(after map[string]any) {
			after["groups"].([]any)[0].(map[string]any)["interfaces"] = "changed"
		}, want: "groups[0].interfaces"},
		{name: "nested array removed", mutate: func(after map[string]any) {
			delete(after["groups"].([]any)[0].(map[string]any), "interfaces")
		}, want: "groups[0].interfaces[0].mac"},
		{name: "unprotected sibling", mutate: func(after map[string]any) {
			group := after["groups"].([]any)[0].(map[string]any)
			group["interfaces"].([]any)[0].(map[string]any)["hostname"] = "changed"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			after := runtime.DeepCopyJSON(before)
			test.mutate(after)
			if got := changedProtectedPath(before, after, "groups[].interfaces[].mac"); got != test.want {
				t.Fatalf("changed path = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRenderHardwareSkipAnnotationDoesNotIterate(t *testing.T) {
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{templateSkipAnnotation: `["spec.interfaces[].dhcp.hostname"]`}},
		Spec: tinkerbell.HardwareSpec{Interfaces: []tinkerbell.Interface{
			{DHCP: &tinkerbell.DHCP{Hostname: "{{ ds.meta_data.hostname }}"}},
		}},
	}
	if _, err := renderHardware(hw, nil); err == nil {
		t.Fatal("expected Jinja parse error; annotation paths must not iterate arrays")
	}
}

func TestRenderHardwareProtectionFinalState(t *testing.T) {
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "machine1"},
		Spec:       tinkerbell.HardwareSpec{UserData: ptr(`{{ $_ := set .hardware.metadata "name" "other" }}{{ $_ := set .hardware.metadata "name" "machine1" }}{{ .hardware.metadata.name }}`)},
	}
	got, err := renderHardware(hw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "machine1" || *got.Spec.UserData != "machine1" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestRenderHardwareSkipIsNotProtection(t *testing.T) {
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{templateSkipAnnotation: `["spec.metadata.instance.hostname"]`}},
		Spec: tinkerbell.HardwareSpec{
			Metadata: &tinkerbell.HardwareMetadata{Instance: &tinkerbell.MetadataInstance{ID: "instance1", Hostname: "{{ ds.meta_data.hostname }}"}},
			UserData: ptr(`{{ $_ := set .hardware.spec.metadata.instance "hostname" "changed" }}{{ .hardware.spec.metadata.instance.hostname }}`),
		},
	}
	got, err := renderHardware(hw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Metadata.Instance.Hostname != "changed" || *got.Spec.UserData != "changed" || hw.Spec.Metadata.Instance.Hostname != "{{ ds.meta_data.hostname }}" {
		t.Fatalf("unexpected result: %+v", got.Spec)
	}
}

func TestRenderHardwareReferenceIsolation(t *testing.T) {
	for _, test := range []struct {
		name      string
		suffix    string
		wantError bool
	}{
		{name: "success"},
		{name: "protected mutation failure", suffix: `{{ $_ := set .hardware.metadata "name" "other" }}`, wantError: true},
		{name: "execution failure", suffix: `{{ .references.missing }}`, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			references := map[string]any{"net": map[string]any{"data": map[string]any{"domain": "original"}}}
			original := runtime.DeepCopyJSON(references)
			hw := &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{Name: "machine1"},
				Spec:       tinkerbell.HardwareSpec{UserData: ptr(`{{ $_ := set .references.net.data "domain" "changed" }}{{ .references.net.data.domain }}` + test.suffix)},
			}
			got, err := renderHardware(hw, references)
			if (err != nil) != test.wantError {
				t.Fatalf("err = %v, want error %v", err, test.wantError)
			}
			if !test.wantError && *got.Spec.UserData != "changed" {
				t.Fatalf("userData = %q, want changed", *got.Spec.UserData)
			}
			if test.wantError && got != nil {
				t.Fatal("failed render returned hardware")
			}
			if diff := cmp.Diff(original, references); diff != "" {
				t.Errorf("references were modified (-want +got):\n%s", diff)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
