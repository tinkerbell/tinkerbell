package webhttp

import (
	"sort"
	"strings"
	"sync"

	"github.com/tinkerbell/tinkerbell/crd"
	"github.com/tinkerbell/tinkerbell/ui/templates"
	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

var (
	cachedDashboardData map[string]*templates.DashboardData
	crdParseOnce        sync.Once
)

// kindToRoute maps CRD kinds to their web UI routes.
var kindToRoute = map[string]string{
	"Hardware":        "/hardware",
	"Workflow":        "/workflows",
	"Template":        "/templates",
	"WorkflowRuleSet": "/workflows/rulesets",
	"Job":             "/bmc/jobs",
	"Machine":         "/bmc/machines",
	"Task":            "/bmc/tasks",
}

// kindDescriptions provides meaningful descriptions for each CRD kind, keyed by "version/kind".
var kindDescriptions = map[string]string{
	// v1alpha1
	"v1alpha1/Hardware":        "Machines in your infrastructure, with details about network interfaces, disks, and BMC connections.",
	"v1alpha1/Workflow":        "A provisioning workflow that executes a sequence of Actions on Hardware using a referenced Template.",
	"v1alpha1/Template":        "Reusable workflow definitions with templated Actions that can be applied to multiple Hardware resources.",
	"v1alpha1/WorkflowRuleSet": "Rules for automatic Workflow creation when Hardware matches specific criteria during discovery.",
	"v1alpha1/Machine":         "A BMC (Baseboard Management Controller) connection for out-of-band Hardware management.",
	"v1alpha1/Job":             "A BMC operation request containing one or more Tasks to execute on a target Machine.",
	"v1alpha1/Task":            "An individual BMC operation within a Job, such as power control or boot device configuration.",

	// v1alpha2
	"v1alpha2/Hardware": "Machines in your infrastructure, with details about network interfaces, disks, BMC connections, and auto-discovery capabilities.",
	"v1alpha2/Task":     "A reusable definition of Actions that can be referenced by Workflows for execution on Hardware.",
	"v1alpha2/BMC":      "BMCs (Baseboard Management Controller) for out-of-band hardware management operations.",
	"v1alpha2/Workflow": "An ordered sequence of Tasks with boot options, hardware references, and templating for provisioning Hardware.",
	"v1alpha2/Policy":   "Controls Workflow auto-creation rules based on agent attributes and reference access policies for Hardware and Tasks.",
	"v1alpha2/Job":      "A BMC operation request containing one or more operations to execute on a target BMC.",
}

// GetDashboardDataForVersion returns the parsed CRD data for a specific API version.
// Data is cached after first parse. Falls back to v1alpha1 for unknown versions.
func GetDashboardDataForVersion(version string) templates.DashboardData {
	crdParseOnce.Do(func() {
		cachedDashboardData = make(map[string]*templates.DashboardData, len(crd.CRDsByVersion))
		for v, crds := range crd.CRDsByVersion {
			cachedDashboardData[v] = parseCRDsFromSource(crds)
		}
	})
	data, ok := cachedDashboardData[version]
	if !ok {
		data = cachedDashboardData["v1alpha1"]
	}
	return *data
}

// GetDashboardData returns the parsed v1alpha1 CRD data for the dashboard.
func GetDashboardData() templates.DashboardData {
	return GetDashboardDataForVersion("v1alpha1")
}

// parseCRDsFromSource parses all CRD YAMLs from a source map and returns dashboard data.
func parseCRDsFromSource(crds map[string][]byte) *templates.DashboardData {
	tinkerbellCRDs := []templates.CRDInfo{}
	bmcCRDs := []templates.CRDInfo{}

	for _, rawYAML := range crds {
		crdInfo := parseSingleCRD(rawYAML)
		if crdInfo == nil {
			continue
		}

		switch crdInfo.Group {
		case crd.GroupBMC:
			bmcCRDs = append(bmcCRDs, *crdInfo)
		default:
			tinkerbellCRDs = append(tinkerbellCRDs, *crdInfo)
		}
	}

	// Sort CRDs by kind name for consistent ordering
	sort.Slice(tinkerbellCRDs, func(i, j int) bool {
		return tinkerbellCRDs[i].Kind < tinkerbellCRDs[j].Kind
	})
	sort.Slice(bmcCRDs, func(i, j int) bool {
		return bmcCRDs[i].Kind < bmcCRDs[j].Kind
	})

	groups := []templates.CRDGroup{
		{
			Name: crd.GroupTinkerbell,
			CRDs: tinkerbellCRDs,
		},
	}
	if len(bmcCRDs) > 0 {
		groups = append(groups, templates.CRDGroup{
			Name: crd.GroupBMC,
			CRDs: bmcCRDs,
		})
	}

	return &templates.DashboardData{
		Groups: groups,
	}
}

// parseSingleCRD parses a single CRD YAML and returns CRDInfo.
func parseSingleCRD(rawYAML []byte) *templates.CRDInfo {
	var crdDef apiv1.CustomResourceDefinition
	if err := yaml.Unmarshal(rawYAML, &crdDef); err != nil {
		return nil
	}

	// Get the first version (v1alpha1)
	if len(crdDef.Spec.Versions) == 0 {
		return nil
	}
	version := crdDef.Spec.Versions[0]

	schema := version.Schema
	if schema == nil || schema.OpenAPIV3Schema == nil {
		return nil
	}

	rootSchema := schema.OpenAPIV3Schema

	// Use custom description if available, otherwise fall back to CRD description
	kind := crdDef.Spec.Names.Kind
	description := kindDescriptions[version.Name+"/"+kind]
	if description == "" && rootSchema.Description != "" {
		description = rootSchema.Description
	}

	// Extract spec fields
	specFields := []templates.SchemaField{}
	if specProp, ok := rootSchema.Properties["spec"]; ok {
		specFields = extractFields(specProp, getRequiredSet(specProp.Required), kind)
	}

	// Extract status fields
	statusFields := []templates.SchemaField{}
	if statusProp, ok := rootSchema.Properties["status"]; ok {
		statusFields = extractFields(statusProp, getRequiredSet(statusProp.Required), kind)
	}

	route := kindToRoute[kind]

	return &templates.CRDInfo{
		Kind:         kind,
		Plural:       crdDef.Spec.Names.Plural,
		Group:        crdDef.Spec.Group,
		Version:      version.Name,
		Description:  description,
		Route:        route,
		SpecFields:   specFields,
		StatusFields: statusFields,
	}
}

// extractFields extracts schema fields from a JSONSchemaProps.
func extractFields(prop apiv1.JSONSchemaProps, requiredSet map[string]bool, kind string) []templates.SchemaField {
	fields := []templates.SchemaField{}

	if prop.Properties == nil {
		return fields
	}

	// Get sorted property names for consistent ordering
	propNames := make([]string, 0, len(prop.Properties))
	for name := range prop.Properties {
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)

	for _, name := range propNames {
		propDef := prop.Properties[name]
		field := extractField(name, propDef, requiredSet[name], kind)
		fields = append(fields, field)
	}

	return fields
}

// extractField extracts a single schema field.
func extractField(name string, prop apiv1.JSONSchemaProps, required bool, kind string) templates.SchemaField {
	field := templates.SchemaField{
		Name:        name,
		Type:        prop.Type,
		Description: prop.Description,
		Required:    required,
		Pattern:     prop.Pattern,
		Format:      prop.Format,
	}
	// Check if deprecated by looking for "Deprecated:" in description
	// or checking description text patterns
	if strings.HasPrefix(prop.Description, "Deprecated:") ||
		strings.Contains(prop.Description, "deprecated and will be removed") {
		field.Deprecated = true
	}

	// Special handling for Template.spec.data - expand workflow structure
	if kind == "Template" && name == "data" && prop.Type == "string" {
		field.Description = "Expand to see the typed structure that this string blob must follow."
		field.Children = getWorkflowSchemaFields()
		return field
	}

	// Handle enum values
	if len(prop.Enum) > 0 {
		field.Enum = make([]string, 0, len(prop.Enum))
		for _, e := range prop.Enum {
			field.Enum = append(field.Enum, string(e.Raw))
		}
	}

	// Handle default value
	if prop.Default != nil {
		field.Default = string(prop.Default.Raw)
	}

	// Handle nested objects
	if prop.Type == "object" && prop.Properties != nil {
		field.Children = extractFields(prop, getRequiredSet(prop.Required), kind)
	}

	// Handle objects with additionalProperties (maps with typed values)
	if prop.Type == "object" && prop.AdditionalProperties != nil && prop.AdditionalProperties.Schema != nil {
		valueSchema := prop.AdditionalProperties.Schema
		if valueSchema.Properties != nil {
			// This is a map[string]object - show the value object's structure
			field.Type = "object (map[string]object)"
			field.Children = extractFields(*valueSchema, getRequiredSet(valueSchema.Required), kind)
		} else if valueSchema.Type != "" {
			// This is a map[string]primitive
			field.Type = "object (map[string]" + valueSchema.Type + ")"
		}
	}

	// Handle arrays with object items
	if prop.Type == "array" && prop.Items != nil {
		if prop.Items.Schema != nil {
			itemSchema := prop.Items.Schema
			if itemSchema.Type == "object" && itemSchema.Properties != nil {
				field.Children = extractFields(*itemSchema, getRequiredSet(itemSchema.Required), kind)
			} else {
				// Simple array type - show the item type
				field.Type = "array[" + itemSchema.Type + "]"
				if itemSchema.Description != "" && field.Description == "" {
					field.Description = itemSchema.Description
				}
			}
		}
	}

	return field
}

// getRequiredSet converts a required slice to a set for O(1) lookup.
func getRequiredSet(required []string) map[string]bool {
	set := make(map[string]bool, len(required))
	for _, r := range required {
		set[r] = true
	}
	return set
}

// getWorkflowSchemaFields returns the expected schema structure for Template.spec.data workflow YAML.
// This needs to match tink/controller/internal/workflow/types.go
func getWorkflowSchemaFields() []templates.SchemaField {
	return []templates.SchemaField{
		{
			Name:        "name",
			Type:        "string",
			Description: "Workflow name",
			Required:    true,
		},
		{
			Name:        "id",
			Type:        "string",
			Description: "Unique workflow identifier",
			Required:    false,
		},
		{
			Name:        "global_timeout",
			Type:        "integer",
			Description: "Global timeout in seconds for the entire workflow",
			Required:    false,
		},
		{
			Name:        "tasks",
			Type:        "array[object]",
			Description: "List of tasks to execute in sequence",
			Required:    true,
			Children: []templates.SchemaField{
				{
					Name:        "name",
					Type:        "string",
					Description: "Task name",
					Required:    true,
				},
				{
					Name:        "worker",
					Type:        "string",
					Description: "Worker address (supports template variables like {{.device_1}})",
					Required:    true,
				},
				{
					Name:        "volumes",
					Type:        "array[string]",
					Description: "Volume mounts for all actions in this task",
					Required:    false,
				},
				{
					Name:        "environment",
					Type:        "object (map[string]string)",
					Description: "Environment variables for all actions in this task (key-value pairs)",
					Required:    false,
				},
				{
					Name:        "actions",
					Type:        "array[object]",
					Description: "List of actions to execute within this task",
					Required:    true,
					Children: []templates.SchemaField{
						{
							Name:        "name",
							Type:        "string",
							Description: "Action name",
							Required:    true,
						},
						{
							Name:        "image",
							Type:        "string",
							Description: "Container image to execute",
							Required:    true,
						},
						{
							Name:        "timeout",
							Type:        "integer",
							Description: "Action timeout in seconds",
							Required:    false,
						},
						{
							Name:        "command",
							Type:        "array[string]",
							Description: "Command to execute in the container",
							Required:    false,
						},
						{
							Name:        "on-timeout",
							Type:        "array[string]",
							Description: "Commands to run if the action times out",
							Required:    false,
						},
						{
							Name:        "on-failure",
							Type:        "array[string]",
							Description: "Commands to run if the action fails",
							Required:    false,
						},
						{
							Name:        "volumes",
							Type:        "array[string]",
							Description: "Volume mounts for this action",
							Required:    false,
						},
						{
							Name:        "environment",
							Type:        "object (map[string]string)",
							Description: "Environment variables for this action (key-value pairs)",
							Required:    false,
						},
						{
							Name:        "pid",
							Type:        "string",
							Description: "PID namespace mode (e.g., 'host')",
							Required:    false,
						},
					},
				},
			},
		},
	}
}
