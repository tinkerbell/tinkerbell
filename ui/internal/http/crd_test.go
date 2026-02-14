package webhttp

import (
	"testing"

	"github.com/tinkerbell/tinkerbell/ui/templates"
	apiv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestGetDashboardData(t *testing.T) {
	// GetDashboardData should return parsed CRD data from embedded files
	data := GetDashboardData()

	// Should have two groups: tinkerbell.org and bmc.tinkerbell.org
	if len(data.Groups) != 2 {
		t.Errorf("len(Groups) = %d, want 2", len(data.Groups))
	}

	// Verify group names
	groupNames := make(map[string]bool)
	for _, g := range data.Groups {
		groupNames[g.Name] = true
	}

	expectedGroups := []string{"tinkerbell.org", "bmc.tinkerbell.org"}
	for _, name := range expectedGroups {
		if !groupNames[name] {
			t.Errorf("expected group %q not found", name)
		}
	}
}

func TestGetDashboardData_TinkerbellCRDs(t *testing.T) {
	data := GetDashboardData()

	var tinkerbellGroup *templates.CRDGroup
	for i := range data.Groups {
		if data.Groups[i].Name == "tinkerbell.org" {
			tinkerbellGroup = &data.Groups[i]
			break
		}
	}

	if tinkerbellGroup == nil {
		t.Fatal("tinkerbell.org group not found")
	}

	// Check expected CRD kinds exist
	expectedKinds := []string{"Hardware", "Template", "Workflow", "WorkflowRuleSet"}
	kindFound := make(map[string]bool)

	for _, crd := range tinkerbellGroup.CRDs {
		kindFound[crd.Kind] = true
	}

	for _, kind := range expectedKinds {
		if !kindFound[kind] {
			t.Errorf("expected CRD kind %q not found in tinkerbell.org group", kind)
		}
	}
}

func TestGetDashboardData_BMCCRDs(t *testing.T) {
	data := GetDashboardData()

	var bmcGroup *templates.CRDGroup
	for i := range data.Groups {
		if data.Groups[i].Name == "bmc.tinkerbell.org" {
			bmcGroup = &data.Groups[i]
			break
		}
	}

	if bmcGroup == nil {
		t.Fatal("bmc.tinkerbell.org group not found")
	}

	// Check expected CRD kinds exist
	expectedKinds := []string{"Job", "Machine", "Task"}
	kindFound := make(map[string]bool)

	for _, crd := range bmcGroup.CRDs {
		kindFound[crd.Kind] = true
	}

	for _, kind := range expectedKinds {
		if !kindFound[kind] {
			t.Errorf("expected CRD kind %q not found in bmc.tinkerbell.org group", kind)
		}
	}
}

func TestKindToRoute(t *testing.T) {
	tests := []struct {
		kind      string
		wantRoute string
	}{
		{kind: "Hardware", wantRoute: "/hardware"},
		{kind: "Workflow", wantRoute: "/workflows"},
		{kind: "Template", wantRoute: "/templates"},
		{kind: "WorkflowRuleSet", wantRoute: "/workflows/rulesets"},
		{kind: "Job", wantRoute: "/bmc/jobs"},
		{kind: "Machine", wantRoute: "/bmc/machines"},
		{kind: "Task", wantRoute: "/bmc/tasks"},
		{kind: "Unknown", wantRoute: ""},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := kindToRoute[tt.kind]
			if got != tt.wantRoute {
				t.Errorf("kindToRoute[%q] = %q, want %q", tt.kind, got, tt.wantRoute)
			}
		})
	}
}

func TestKindDescriptions(t *testing.T) {
	tests := []struct {
		kind         string
		wantNonEmpty bool
	}{
		{kind: "Hardware", wantNonEmpty: true},
		{kind: "Workflow", wantNonEmpty: true},
		{kind: "Template", wantNonEmpty: true},
		{kind: "WorkflowRuleSet", wantNonEmpty: true},
		{kind: "Job", wantNonEmpty: true},
		{kind: "Machine", wantNonEmpty: true},
		{kind: "Task", wantNonEmpty: true},
		{kind: "Unknown", wantNonEmpty: false},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := kindDescriptions[tt.kind]
			if tt.wantNonEmpty && got == "" {
				t.Errorf("kindDescriptions[%q] is empty, want non-empty", tt.kind)
			}
			if !tt.wantNonEmpty && got != "" {
				t.Errorf("kindDescriptions[%q] = %q, want empty", tt.kind, got)
			}
		})
	}
}

func TestGetRequiredSet(t *testing.T) {
	tests := []struct {
		name     string
		required []string
		check    string
		want     bool
	}{
		{
			name:     "empty slice",
			required: []string{},
			check:    "foo",
			want:     false,
		},
		{
			name:     "field is required",
			required: []string{"foo", "bar"},
			check:    "foo",
			want:     true,
		},
		{
			name:     "field is not required",
			required: []string{"foo", "bar"},
			check:    "baz",
			want:     false,
		},
		{
			name:     "nil slice",
			required: nil,
			check:    "foo",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := getRequiredSet(tt.required)
			if got := set[tt.check]; got != tt.want {
				t.Errorf("getRequiredSet(%v)[%q] = %v, want %v", tt.required, tt.check, got, tt.want)
			}
		})
	}
}

func TestExtractField(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		prop      apiv1.JSONSchemaProps
		required  bool
		kind      string
		wantType  string
		wantReq   bool
	}{
		{
			name:      "string field",
			fieldName: "name",
			prop:      apiv1.JSONSchemaProps{Type: "string", Description: "The name"},
			required:  true,
			kind:      "Hardware",
			wantType:  "string",
			wantReq:   true,
		},
		{
			name:      "integer field",
			fieldName: "count",
			prop:      apiv1.JSONSchemaProps{Type: "integer"},
			required:  false,
			kind:      "Hardware",
			wantType:  "integer",
			wantReq:   false,
		},
		{
			name:      "boolean field",
			fieldName: "enabled",
			prop:      apiv1.JSONSchemaProps{Type: "boolean"},
			required:  false,
			kind:      "Hardware",
			wantType:  "boolean",
			wantReq:   false,
		},
		{
			name:      "array of strings",
			fieldName: "tags",
			prop: apiv1.JSONSchemaProps{
				Type: "array",
				Items: &apiv1.JSONSchemaPropsOrArray{
					Schema: &apiv1.JSONSchemaProps{Type: "string"},
				},
			},
			required: false,
			kind:     "Hardware",
			wantType: "array[string]",
			wantReq:  false,
		},
		{
			name:      "field with pattern",
			fieldName: "mac",
			prop: apiv1.JSONSchemaProps{
				Type:    "string",
				Pattern: "^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$",
			},
			required: true,
			kind:     "Hardware",
			wantType: "string",
			wantReq:  true,
		},
		{
			name:      "field with format",
			fieldName: "ip",
			prop: apiv1.JSONSchemaProps{
				Type:   "string",
				Format: "ipv4",
			},
			required: false,
			kind:     "Hardware",
			wantType: "string",
			wantReq:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := extractField(tt.fieldName, tt.prop, tt.required, tt.kind)

			if field.Name != tt.fieldName {
				t.Errorf("Name = %q, want %q", field.Name, tt.fieldName)
			}
			if field.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", field.Type, tt.wantType)
			}
			if field.Required != tt.wantReq {
				t.Errorf("Required = %v, want %v", field.Required, tt.wantReq)
			}
		})
	}
}

func TestExtractField_NestedObject(t *testing.T) {
	prop := apiv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiv1.JSONSchemaProps{
			"host": {Type: "string"},
			"port": {Type: "integer"},
		},
	}

	field := extractField("connection", prop, true, "Machine")

	if field.Type != "object" {
		t.Errorf("Type = %q, want %q", field.Type, "object")
	}

	if len(field.Children) != 2 {
		t.Errorf("len(Children) = %d, want 2", len(field.Children))
	}
}

func TestExtractField_EnumValues(t *testing.T) {
	prop := apiv1.JSONSchemaProps{
		Type: "string",
		Enum: []apiv1.JSON{
			{Raw: []byte(`"on"`)},
			{Raw: []byte(`"off"`)},
		},
	}

	field := extractField("state", prop, false, "Machine")

	if len(field.Enum) != 2 {
		t.Errorf("len(Enum) = %d, want 2", len(field.Enum))
	}
}

func TestExtractField_DefaultValue(t *testing.T) {
	prop := apiv1.JSONSchemaProps{
		Type:    "integer",
		Default: &apiv1.JSON{Raw: []byte(`30`)},
	}

	field := extractField("timeout", prop, false, "Workflow")

	if field.Default != "30" {
		t.Errorf("Default = %q, want %q", field.Default, "30")
	}
}

func TestExtractField_TemplateDataField(t *testing.T) {
	// Special handling for Template.spec.data field
	prop := apiv1.JSONSchemaProps{
		Type:        "string",
		Description: "Original description",
	}

	field := extractField("data", prop, false, "Template")

	// Should have workflow schema children
	if len(field.Children) == 0 {
		t.Error("Template data field should have workflow schema children")
	}

	// Check for expected top-level fields
	fieldNames := make(map[string]bool)
	for _, child := range field.Children {
		fieldNames[child.Name] = true
	}

	expectedFields := []string{"name", "tasks"}
	for _, name := range expectedFields {
		if !fieldNames[name] {
			t.Errorf("expected workflow schema field %q not found", name)
		}
	}
}

func TestExtractFields(t *testing.T) {
	prop := apiv1.JSONSchemaProps{
		Properties: map[string]apiv1.JSONSchemaProps{
			"name":      {Type: "string"},
			"namespace": {Type: "string"},
			"enabled":   {Type: "boolean"},
		},
		Required: []string{"name"},
	}

	fields := extractFields(prop, getRequiredSet(prop.Required), "Test")

	if len(fields) != 3 {
		t.Errorf("len(fields) = %d, want 3", len(fields))
	}

	// Check name field is required
	for _, f := range fields {
		if f.Name == "name" && !f.Required {
			t.Error("name field should be required")
		}
		if f.Name == "namespace" && f.Required {
			t.Error("namespace field should not be required")
		}
	}
}

func TestExtractFields_EmptyProperties(t *testing.T) {
	prop := apiv1.JSONSchemaProps{
		Properties: nil,
	}

	fields := extractFields(prop, nil, "Test")

	if len(fields) != 0 {
		t.Errorf("len(fields) = %d, want 0", len(fields))
	}
}

func TestExtractFields_Sorted(t *testing.T) {
	prop := apiv1.JSONSchemaProps{
		Properties: map[string]apiv1.JSONSchemaProps{
			"zebra":  {Type: "string"},
			"alpha":  {Type: "string"},
			"middle": {Type: "string"},
		},
	}

	fields := extractFields(prop, nil, "Test")

	// Fields should be sorted alphabetically
	if len(fields) != 3 {
		t.Fatalf("len(fields) = %d, want 3", len(fields))
	}
	if fields[0].Name != "alpha" {
		t.Errorf("fields[0].Name = %q, want %q", fields[0].Name, "alpha")
	}
	if fields[1].Name != "middle" {
		t.Errorf("fields[1].Name = %q, want %q", fields[1].Name, "middle")
	}
	if fields[2].Name != "zebra" {
		t.Errorf("fields[2].Name = %q, want %q", fields[2].Name, "zebra")
	}
}

func TestGetWorkflowSchemaFields(t *testing.T) {
	fields := getWorkflowSchemaFields()

	if len(fields) == 0 {
		t.Fatal("getWorkflowSchemaFields() returned empty slice")
	}

	// Check for required top-level fields
	fieldMap := make(map[string]templates.SchemaField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	// name should be required
	if f, ok := fieldMap["name"]; !ok {
		t.Error("name field not found")
	} else if !f.Required {
		t.Error("name field should be required")
	}

	// tasks should be required and have children
	if f, ok := fieldMap["tasks"]; !ok {
		t.Error("tasks field not found")
	} else {
		if !f.Required {
			t.Error("tasks field should be required")
		}
		if len(f.Children) == 0 {
			t.Error("tasks field should have children (task schema)")
		}
	}

	// global_timeout should be optional
	if f, ok := fieldMap["global_timeout"]; !ok {
		t.Error("global_timeout field not found")
	} else if f.Required {
		t.Error("global_timeout field should be optional")
	}
}

func TestGetWorkflowSchemaFields_TasksHaveActions(t *testing.T) {
	fields := getWorkflowSchemaFields()

	var tasksField *templates.SchemaField
	for i := range fields {
		if fields[i].Name == "tasks" {
			tasksField = &fields[i]
			break
		}
	}

	if tasksField == nil {
		t.Fatal("tasks field not found")
	}

	// Look for actions field in task children
	var actionsField *templates.SchemaField
	for i := range tasksField.Children {
		if tasksField.Children[i].Name == "actions" {
			actionsField = &tasksField.Children[i]
			break
		}
	}

	if actionsField == nil {
		t.Fatal("actions field not found in task schema")
	}

	if !actionsField.Required {
		t.Error("actions field should be required")
	}

	if len(actionsField.Children) == 0 {
		t.Error("actions field should have children (action schema)")
	}

	// Check action has expected fields
	actionFieldNames := make(map[string]bool)
	for _, f := range actionsField.Children {
		actionFieldNames[f.Name] = true
	}

	expectedActionFields := []string{"name", "image", "timeout", "command", "environment"}
	for _, name := range expectedActionFields {
		if !actionFieldNames[name] {
			t.Errorf("expected action field %q not found", name)
		}
	}
}

func TestExtractField_MapStringString(t *testing.T) {
	prop := apiv1.JSONSchemaProps{
		Type: "object",
		AdditionalProperties: &apiv1.JSONSchemaPropsOrBool{
			Schema: &apiv1.JSONSchemaProps{Type: "string"},
		},
	}

	field := extractField("labels", prop, false, "Hardware")

	if field.Type != "object (map[string]string)" {
		t.Errorf("Type = %q, want %q", field.Type, "object (map[string]string)")
	}
}

func TestExtractField_ArrayOfObjects(t *testing.T) {
	prop := apiv1.JSONSchemaProps{
		Type: "array",
		Items: &apiv1.JSONSchemaPropsOrArray{
			Schema: &apiv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiv1.JSONSchemaProps{
					"name":  {Type: "string"},
					"value": {Type: "string"},
				},
			},
		},
	}

	field := extractField("items", prop, false, "Test")

	if field.Type != "array" {
		t.Errorf("Type = %q, want %q", field.Type, "array")
	}

	if len(field.Children) != 2 {
		t.Errorf("len(Children) = %d, want 2", len(field.Children))
	}
}

func TestParseCRDs_CachesBehavior(t *testing.T) {
	// GetDashboardData is cached via sync.Once
	// Calling it multiple times should return the same data
	data1 := GetDashboardData()
	data2 := GetDashboardData()

	if len(data1.Groups) != len(data2.Groups) {
		t.Error("cached data should be identical")
	}

	for i := range data1.Groups {
		if data1.Groups[i].Name != data2.Groups[i].Name {
			t.Errorf("Groups[%d].Name mismatch: %q vs %q", i, data1.Groups[i].Name, data2.Groups[i].Name)
		}
	}
}

func TestCRDInfo_HasRoutes(t *testing.T) {
	data := GetDashboardData()

	for _, group := range data.Groups {
		for _, crd := range group.CRDs {
			if crd.Route == "" {
				t.Errorf("CRD %s.%s has empty route", crd.Kind, group.Name)
			}
		}
	}
}

func TestCRDInfo_HasVersions(t *testing.T) {
	data := GetDashboardData()

	for _, group := range data.Groups {
		for _, crd := range group.CRDs {
			if crd.Version == "" {
				t.Errorf("CRD %s.%s has empty version", crd.Kind, group.Name)
			}
		}
	}
}
