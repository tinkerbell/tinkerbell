package workflowtemplate

import (
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// templateDataReferences is the key used to access the Hardware references in the template data.
	// This is lowercase as it is new and follows the all lowercase convention used when referencing
	// fields in the reference object.
	templateDataReferences = "references"
	// templateDataHardware is the key used to access the Hardware data in the template data.
	templateDataHardware = "hardware"
	// templateDataHardwareLegacy is the key used to access the Hardware data in the template data.
	// This is Title cased as it was the original convention used in the template data and is
	// used for backwards compatibility.
	//
	// Deprecated: use templateDataHardware instead. This key will be removed in a future release.
	templateDataHardwareLegacy = "Hardware"
)

// templateHardwareData defines the data exposed for a Hardware instance to a Template.
type templateHardwareData struct {
	Disks      []string
	Interfaces []v1alpha1.Interface
	UserData   string
	Metadata   v1alpha1.HardwareMetadata
	VendorData string
}

// Data returns the data a Workflow's Template is rendered with: the Workflow's hardwareMap entries,
// the Hardware under the "hardware" key (and, for backwards compatibility, under "Hardware"), and
// the resolved Hardware references under "references". The last three override hardwareMap
// entries with the same key. It returns an error if hardware can't be converted.
func Data(hardware v1alpha1.Hardware, hardwareMap map[string]string, references map[string]any) (map[string]any, error) {
	data := make(map[string]any)
	for key, val := range hardwareMap {
		data[key] = val
	}
	// Not a JSON round trip: that turns integers into float64 and replaces non-UTF-8 bytes.
	hardwareData, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&hardware)
	if err != nil {
		return nil, err
	}
	data[templateDataHardware] = hardwareData
	data[templateDataHardwareLegacy] = toTemplateHardwareData(hardware)
	data[templateDataReferences] = references

	return data, nil
}

// toTemplateHardwareData converts a Hardware instance of templateHardwareData for use in template
// rendering.
func toTemplateHardwareData(hardware v1alpha1.Hardware) templateHardwareData {
	var contract templateHardwareData
	for _, disk := range hardware.Spec.Disks {
		contract.Disks = append(contract.Disks, disk.Device)
	}
	if len(hardware.Spec.Interfaces) > 0 {
		contract.Interfaces = hardware.Spec.Interfaces
	}
	if hardware.Spec.UserData != nil {
		contract.UserData = *hardware.Spec.UserData
	}
	if hardware.Spec.Metadata != nil {
		contract.Metadata = *hardware.Spec.Metadata
	}
	if hardware.Spec.VendorData != nil {
		contract.VendorData = *hardware.Spec.VendorData
	}
	return contract
}
