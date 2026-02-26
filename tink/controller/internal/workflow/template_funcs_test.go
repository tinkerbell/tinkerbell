package workflow

import (
	"reflect"
	"strings"
	"testing"
	"text/template"

	"sigs.k8s.io/yaml"
)

func TestToYaml(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
		// validate checks the YAML output structurally rather than by exact string.
		validate func(t *testing.T, yamlStr string)
	}{
		{
			name:  "simple map",
			input: map[string]interface{}{"name": "bob", "age": 25},
			validate: func(t *testing.T, yamlStr string) {
				t.Helper()
				var got map[string]interface{}
				if err := yaml.Unmarshal([]byte(yamlStr), &got); err != nil {
					t.Fatalf("failed to unmarshal toYaml output: %v", err)
				}
				if got["name"] != "bob" {
					t.Errorf("name = %v, want bob", got["name"])
				}
			},
		},
		{
			name: "nested map",
			input: map[string]interface{}{
				"person": map[string]interface{}{
					"name": "alice",
					"address": map[string]interface{}{
						"city": "wonderland",
					},
				},
			},
			validate: func(t *testing.T, yamlStr string) {
				t.Helper()
				if !strings.Contains(yamlStr, "city: wonderland") {
					t.Errorf("output missing 'city: wonderland', got: %q", yamlStr)
				}
				if !strings.Contains(yamlStr, "name: alice") {
					t.Errorf("output missing 'name: alice', got: %q", yamlStr)
				}
			},
		},
		{
			name:  "slice",
			input: []string{"a", "b", "c"},
			validate: func(t *testing.T, yamlStr string) {
				t.Helper()
				var got []string
				if err := yaml.Unmarshal([]byte(yamlStr), &got); err != nil {
					t.Fatalf("failed to unmarshal toYaml output: %v", err)
				}
				if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
					t.Errorf("got %v, want [a b c]", got)
				}
			},
		},
		{
			name:  "nil input",
			input: nil,
			validate: func(t *testing.T, yamlStr string) {
				t.Helper()
				if strings.TrimSpace(yamlStr) != "null" {
					t.Errorf("got %q, want 'null'", yamlStr)
				}
			},
		},
		{
			name:  "empty map",
			input: map[string]interface{}{},
			validate: func(t *testing.T, yamlStr string) {
				t.Helper()
				if strings.TrimSpace(yamlStr) != "{}" {
					t.Errorf("got %q, want '{}'", yamlStr)
				}
			},
		},
		{
			name:  "string value",
			input: "hello",
			validate: func(t *testing.T, yamlStr string) {
				t.Helper()
				if strings.TrimSpace(yamlStr) != "hello" {
					t.Errorf("got %q, want 'hello'", yamlStr)
				}
			},
		},
		{
			name:    "unmarshalable type (channel)",
			input:   make(chan int),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toYaml(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("toYaml() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}

func TestFromYaml(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		validate func(t *testing.T, result interface{})
	}{
		{
			name:  "simple yaml map",
			input: "name: bob\nage: 25",
			validate: func(t *testing.T, result interface{}) {
				t.Helper()
				m, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map, got %T", result)
				}
				if m["name"] != "bob" {
					t.Errorf("name = %v, want bob", m["name"])
				}
			},
		},
		{
			name:  "yaml array",
			input: "- a\n- b\n- c",
			validate: func(t *testing.T, result interface{}) {
				t.Helper()
				s, ok := result.([]interface{})
				if !ok {
					t.Fatalf("expected slice, got %T", result)
				}
				if len(s) != 3 {
					t.Errorf("len = %d, want 3", len(s))
				}
			},
		},
		{
			name:  "nested yaml",
			input: "person:\n  name: alice",
			validate: func(t *testing.T, result interface{}) {
				t.Helper()
				m, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("expected map, got %T", result)
				}
				if _, ok := m["person"]; !ok {
					t.Error("missing key 'person'")
				}
			},
		},
		{
			name:    "invalid yaml",
			input:   ":\n  :\n  - :\n  invalid:: yaml:: content",
			wantErr: true,
		},
		{
			name:    "empty string returns error",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fromYaml(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("fromYaml() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}

func TestToYamlRoundTrip(t *testing.T) {
	original := map[string]interface{}{
		"name": "test",
		"items": []interface{}{
			"one",
			"two",
		},
	}

	yamlStr, err := toYaml(original)
	if err != nil {
		t.Fatalf("toYaml() error = %v", err)
	}

	result, err := fromYaml(yamlStr)
	if err != nil {
		t.Fatalf("fromYaml() error = %v", err)
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["name"] != "test" {
		t.Errorf("round trip: name = %v, want %v", m["name"], "test")
	}
}

func TestToYamlInTemplate(t *testing.T) {
	// Test that toYaml works correctly when called from a Go template.
	tmpl, err := template.New("test").Funcs(template.FuncMap{
		"toYaml": toYaml,
	}).Parse(`{{ .data | toYaml }}`)
	if err != nil {
		t.Fatalf("template parse error: %v", err)
	}

	data := map[string]interface{}{
		"data": map[string]interface{}{
			"key": "value",
			"nested": map[string]interface{}{
				"inner": "data",
			},
		},
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "key: value") {
		t.Errorf("template output missing 'key: value', got: %q", got)
	}
	if !strings.Contains(got, "inner: data") {
		t.Errorf("template output missing 'inner: data', got: %q", got)
	}
}

func TestToYamlInTemplateError(t *testing.T) {
	// Test that toYaml errors halt template execution.
	tmpl, err := template.New("test").Funcs(template.FuncMap{
		"toYaml": toYaml,
	}).Parse(`{{ .data | toYaml }}`)
	if err != nil {
		t.Fatalf("template parse error: %v", err)
	}

	data := map[string]interface{}{
		"data": make(chan int), // un-marshalable
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err == nil {
		t.Fatal("expected template execution to fail for un-marshalable type, but it succeeded")
	}
}

func TestFromYamlInTemplate(t *testing.T) {
	// Test that fromYaml works correctly when called from a Go template.
	tmpl, err := template.New("test").Funcs(template.FuncMap{
		"fromYaml": fromYaml,
	}).Parse(`{{ $m := fromYaml .yamlStr }}{{ $m.name }}`)
	if err != nil {
		t.Fatalf("template parse error: %v", err)
	}

	data := map[string]interface{}{
		"yamlStr": "name: alice\nage: 30",
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute error: %v", err)
	}

	got := buf.String()
	if got != "alice" {
		t.Errorf("fromYaml in template = %q, want %q", got, "alice")
	}
}

func TestNetmaskToPrefixLength(t *testing.T) {
	tests := []struct {
		name      string
		netmask   string
		want      string
		wantError bool
	}{
		{
			name:      "valid /24 netmask",
			netmask:   "255.255.255.0",
			want:      "24",
			wantError: false,
		},
		{
			name:      "valid /16 netmask",
			netmask:   "255.255.0.0",
			want:      "16",
			wantError: false,
		},
		{
			name:      "valid /8 netmask",
			netmask:   "255.0.0.0",
			want:      "8",
			wantError: false,
		},
		{
			name:      "valid /32 netmask",
			netmask:   "255.255.255.255",
			want:      "32",
			wantError: false,
		},
		{
			name:      "valid /0 netmask",
			netmask:   "0.0.0.0",
			want:      "0",
			wantError: false,
		},
		{
			name:      "valid /28 netmask",
			netmask:   "255.255.255.240",
			want:      "28",
			wantError: false,
		},
		{
			name:      "valid /30 netmask",
			netmask:   "255.255.255.252",
			want:      "30",
			wantError: false,
		},
		{
			name:      "invalid netmask format",
			netmask:   "invalid",
			want:      "",
			wantError: true,
		},
		{
			name:      "empty netmask",
			netmask:   "",
			want:      "",
			wantError: true,
		},
		{
			name:      "incomplete netmask",
			netmask:   "255.255.255",
			want:      "",
			wantError: true,
		},
		{
			name:      "out of range values",
			netmask:   "256.255.255.0",
			want:      "",
			wantError: true,
		},
		{
			name:      "IPv6 address",
			netmask:   "::1",
			want:      "",
			wantError: true,
		},
		{
			name:      "IPv6 full address",
			netmask:   "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			want:      "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := netmaskToPrefixLength(tt.netmask)
			if (err != nil) != tt.wantError {
				t.Errorf("netmaskToPrefixLength() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if got != tt.want {
				t.Errorf("netmaskToPrefixLength() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatPartition(t *testing.T) {
	tests := []struct {
		dev       string
		partition int
		expect    string
	}{
		{"/dev/disk/by-id/foobar", 1, "/dev/disk/by-id/foobar-part1"},
		{"/dev/disk/other", 2, "/dev/disk/other-part2"},
		{"/dev/nvme0n1", 1, "/dev/nvme0n1p1"},
		{"/dev/nvme0n1", 5, "/dev/nvme0n1p5"},
		{"/dev/sda", 1, "/dev/sda1"},
		{"/dev/sda", 2, "/dev/sda2"},
		{"/dev/loop0", 3, "/dev/loop0p3"},
		{"/dev/loop", 4, "/dev/loop4"},
	}
	for _, tt := range tests {
		got := formatPartition(tt.dev, tt.partition)
		if got != tt.expect {
			t.Errorf("formatPartition(%q, %d) = %q, want %q", tt.dev, tt.partition, got, tt.expect)
		}
	}
}
