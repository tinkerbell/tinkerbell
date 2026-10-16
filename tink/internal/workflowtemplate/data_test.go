package workflowtemplate

import (
	"encoding/base64"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestData(t *testing.T) {
	userData := "user"
	hw := v1alpha1.Hardware{Spec: v1alpha1.HardwareSpec{
		UserData: &userData,
		Disks:    []v1alpha1.Disk{{Device: "/dev/sda"}, {Device: "/dev/sdb"}},
	}}
	references := map[string]any{"ref": "value"}

	data, err := Data(hw, map[string]string{"device_1": "agent", "references": "overridden"}, references)
	if err != nil {
		t.Fatal(err)
	}

	if got := data["device_1"]; got != "agent" {
		t.Errorf("device_1 = %v, want agent", got)
	}
	if diff := cmp.Diff(references, data["references"]); diff != "" {
		t.Errorf("references must override the hardwareMap entry of the same name (-want +got):\n%s", diff)
	}
	hardware, ok := data["hardware"].(map[string]any)
	if !ok {
		t.Fatalf("hardware = %T, want map[string]any", data["hardware"])
	}
	if got := hardware["spec"].(map[string]any)["userData"]; got != "user" {
		t.Errorf("hardware.spec.userData = %v, want user", got)
	}
	legacy, ok := data["Hardware"].(templateHardwareData)
	if !ok {
		t.Fatalf("Hardware = %T, want templateHardwareData", data["Hardware"])
	}
	if diff := cmp.Diff(templateHardwareData{Disks: []string{"/dev/sda", "/dev/sdb"}, UserData: "user"}, legacy); diff != "" {
		t.Errorf("Hardware (-want +got):\n%s", diff)
	}
}

// Data must not convert Hardware through JSON, which would turn integers into float64 and lose
// precision above 2^53.
func TestDataKeepsIntegers(t *testing.T) {
	hw := v1alpha1.Hardware{Status: v1alpha1.HardwareStatus{Attributes: &v1alpha1.HardwareAttributes{
		InBand: &v1alpha1.Attributes{CPU: &v1alpha1.CPU{TotalCores: 8}, Memory: &v1alpha1.Memory{TotalBytes: 9007199254740993}},
	}}}
	data, err := Data(hw, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Render("integers", `{{ .hardware.status.attributes.inBand.memory.totalBytes }} {{ eq .hardware.status.attributes.inBand.cpu.totalCores 8 }}`, data)
	if err != nil {
		t.Fatal(err)
	}
	if want := "9007199254740993 true"; string(got) != want {
		t.Fatalf("rendered %q, want %q", got, want)
	}
}

// Bytes that aren't valid UTF-8 must reach the template unchanged.
func TestRenderBinaryUserData(t *testing.T) {
	raw := "\x30\x82\x00\xff\xfe"
	data, err := Data(v1alpha1.Hardware{Spec: v1alpha1.HardwareSpec{UserData: &raw}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Render("binary", `{{ .hardware.spec.userData | b64enc }}`, data)
	if want := base64.StdEncoding.EncodeToString([]byte(raw)); err != nil || string(got) != want {
		t.Fatalf("render = %q, %v; want %q", got, err, want)
	}
}

const statusTemplate = `version: "0.1"
name: example
global_timeout: 1800
tasks:
  - name: "os-installation"
    worker: "{{ .device_1 }}"
    actions:
      - name: "stream-image"
        image: quay.io/tinkerbell/actions/image2disk:v1.0.0
        timeout: 600
        environment:
          DEST_DISK: "{{ index .Hardware.Disks 0 }}"
`

func TestToWorkflowStatus(t *testing.T) {
	hw := v1alpha1.Hardware{Spec: v1alpha1.HardwareSpec{Disks: []v1alpha1.Disk{{Device: "/dev/nvme0n1"}}}}
	data, err := Data(hw, map[string]string{"device_1": "agent-1"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	status, err := ToWorkflowStatus("example", statusTemplate, data)
	if err != nil {
		t.Fatal(err)
	}

	if status.GlobalTimeout != 1800 {
		t.Errorf("GlobalTimeout = %d, want 1800", status.GlobalTimeout)
	}
	if len(status.Tasks) != 1 || status.Tasks[0].AgentID != "agent-1" {
		t.Fatalf("Tasks = %+v, want one Task for worker agent-1", status.Tasks)
	}
	if got := status.Tasks[0].Actions[0].Environment["DEST_DISK"]; got != "/dev/nvme0n1" {
		t.Errorf("DEST_DISK = %q, want /dev/nvme0n1", got)
	}
}

func TestToWorkflowStatusError(t *testing.T) {
	// device_1 is missing from the data, and a missing key is an error.
	data, err := Data(v1alpha1.Hardware{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if status, err := ToWorkflowStatus("example", statusTemplate, data); err == nil {
		t.Fatalf("expected an error, got status %+v", status)
	}
}
