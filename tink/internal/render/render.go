// Package render renders Tinkerbell Go templates: a Workflow Template against a Hardware
// object (producing the Workflow's Tasks, AgentID and GlobalTimeout), and plain template
// strings via Render.
package render

import (
	"fmt"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
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

// Input is everything ToWorkflowStatus needs to render a Template against a specific
// Hardware instance.
type Input struct {
	// WorkflowName identifies this render in error messages (which, for historical
	// reasons, call it the template ID).
	WorkflowName string
	// TemplateData is the Template's Spec.Data - the Go-template source.
	TemplateData string
	// Hardware is the Hardware object the Workflow targets. Its metadata, Spec and Status
	// are all exposed to the Template under templateDataHardware.
	Hardware v1alpha1.Hardware
	// HardwareMap is Workflow.Spec.HardwareMap, merged into the template data root.
	HardwareMap map[string]string
	// References is pre-resolved reference data (see ResolveReferences), keyed by the same
	// names as Hardware.Spec.References. nil renders as an empty map.
	References map[string]interface{}
}

// NewInput builds an Input from a Workflow, its Template, and pre-resolved references.
func NewInput(wf *v1alpha1.Workflow, tpl *v1alpha1.Template, hardware v1alpha1.Hardware, references map[string]interface{}) Input {
	return Input{
		WorkflowName: wf.Name,
		TemplateData: pointerToValue(tpl.Spec.Data),
		Hardware:     hardware,
		HardwareMap:  wf.Spec.HardwareMap,
		References:   references,
	}
}

// ToWorkflowStatus renders in.TemplateData against in.Hardware and returns the resulting
// WorkflowStatus (Tasks/AgentID/GlobalTimeout populated), or an error if rendering failed.
func ToWorkflowStatus(in Input) (*v1alpha1.WorkflowStatus, error) {
	tdata := make(map[string]interface{})
	for key, val := range in.HardwareMap {
		tdata[key] = val
	}

	// structToMap is used so that fields are accessible in Templates by their json struct tag names instead of
	// their Go struct field names and their case.
	// for example, {{ hardware.spec.metadata.instance.id }} instead of {{ hardware.Spec.Metadata.Instance.ID }}.
	//
	// A failure is surfaced as a render error rather than falling back to an empty map,
	// which would leave the Template silently seeing no hardware data.
	hwMap, err := structToMap(in.Hardware)
	if err != nil {
		return nil, fmt.Errorf("converting hardware to template data: %w", err)
	}
	tdata[templateDataHardware] = hwMap
	tdata[templateDataHardwareLegacy] = toTemplateHardwareData(in.Hardware)

	references := in.References
	if references == nil {
		references = map[string]interface{}{}
	}
	tdata[templateDataReferences] = references

	tinkWf, err := renderTemplateHardware(in.WorkflowName, in.TemplateData, tdata)
	if err != nil {
		return nil, err
	}

	return YAMLToStatus(tinkWf), nil
}
