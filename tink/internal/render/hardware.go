package render

import (
	"encoding/json"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

// structToMap converts a struct to a map[string]interface{}.
func structToMap(item interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Marshal the struct to JSON.
	jsonBytes, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON to a map[string]interface{}.
	if err = json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// templateHardwareData defines the data exposed for a Hardware instance to a Template.
type templateHardwareData struct {
	Disks      []string
	Interfaces []v1alpha1.Interface
	UserData   string
	Metadata   v1alpha1.HardwareMetadata
	VendorData string
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
		contract.UserData = pointerToValue(hardware.Spec.UserData)
	}
	if hardware.Spec.Metadata != nil {
		contract.Metadata = *hardware.Spec.Metadata
	}
	if hardware.Spec.VendorData != nil {
		contract.VendorData = pointerToValue(hardware.Spec.VendorData)
	}
	return contract
}

// pointerToValue returns the value pointed to by ptr, or the zero value of V if ptr is nil.
func pointerToValue[V any](ptr *V) V {
	if ptr == nil {
		var zero V
		return zero
	}
	return *ptr
}
