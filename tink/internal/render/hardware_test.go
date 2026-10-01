package render

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestToTemplateHardwareData(t *testing.T) {
	hw := v1alpha1.Hardware{
		Spec: v1alpha1.HardwareSpec{
			Disks: []v1alpha1.Disk{
				{Device: "/dev/nvme0n1"},
				{Device: "/dev/nvme1n1"},
			},
			Interfaces: []v1alpha1.Interface{
				{DHCP: &v1alpha1.DHCP{MAC: "3c:ec:ef:4c:4f:54"}},
			},
			UserData:   valueToPointer("user-data"),
			VendorData: valueToPointer("vendor-data"),
			Metadata:   &v1alpha1.HardwareMetadata{State: "active"},
		},
	}

	got := toTemplateHardwareData(hw)
	want := templateHardwareData{
		Disks:      []string{"/dev/nvme0n1", "/dev/nvme1n1"},
		Interfaces: hw.Spec.Interfaces,
		UserData:   "user-data",
		VendorData: "vendor-data",
		Metadata:   v1alpha1.HardwareMetadata{State: "active"},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("toTemplateHardwareData() diff (-got +want):\n%s", diff)
	}
}

func TestToTemplateHardwareDataNilPointers(t *testing.T) {
	// UserData/VendorData/Metadata are all optional - absence must not panic and must
	// zero-value rather than propagate a nil pointer.
	got := toTemplateHardwareData(v1alpha1.Hardware{})
	if got.UserData != "" || got.VendorData != "" || got.Metadata != (v1alpha1.HardwareMetadata{}) {
		t.Errorf("expected zero-valued contract for an empty Hardware, got %+v", got)
	}
}

func valueToPointer[V any](v V) *V {
	return &v
}
