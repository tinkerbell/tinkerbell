package delimitedlist

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSpaceList(t *testing.T) {
	tests := map[string]struct {
		input    string
		want     []string
		source   string // "Set", "FromEnv", or "FromFile"
		wantStr  string
		initWith []string // initial values if any
	}{
		"single value": {
			input:   "value1",
			want:    []string{"value1"},
			source:  "Set",
			wantStr: "value1",
		},
		"multiple values": {
			input:   "value1 value2 value3",
			want:    []string{"value1", "value2", "value3"},
			source:  "Set",
			wantStr: "value1 value2 value3",
		},
		"multiple values with extra spaces": {
			input:   "  value1   value2    value3  ",
			want:    []string{"value1", "value2", "value3"},
			source:  "Set",
			wantStr: "value1 value2 value3",
		},
		"from environment": {
			input:   "env1 env2",
			want:    []string{"env1", "env2"},
			source:  "FromEnv",
			wantStr: "env1 env2",
		},
		"from file": {
			input:   "file1 file2",
			want:    []string{"file1", "file2"},
			source:  "FromFile",
			wantStr: "file1 file2",
		},
		"append to existing values": {
			input:    "value3 value4",
			initWith: []string{"value1", "value2"},
			want:     []string{"value1", "value2", "value3", "value4"},
			source:   "Set",
			wantStr:  "value1 value2 value3 value4",
		},
		"empty input": {
			input:   "",
			want:    nil,
			source:  "Set",
			wantStr: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var target []string
			if tt.initWith != nil {
				target = append(target, tt.initWith...)
			}

			v := New(&target, ' ')

			var err error
			switch tt.source {
			case "FromEnv":
				err = v.FromEnv(tt.input)
			case "FromFile":
				err = v.FromFile(tt.input)
			default:
				err = v.Set(tt.input)
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(target, tt.want); diff != "" {
				t.Errorf("values mismatch (-got +want):\n%s", diff)
			}

			if diff := cmp.Diff(v.String(), tt.wantStr); diff != "" {
				t.Errorf("String() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}

func TestString_NilTarget(t *testing.T) {
	tests := map[string]struct {
		target   *[]string
		expected string
	}{
		"nil target": {
			target:   nil,
			expected: "",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			v := &Value{target: tt.target}
			if diff := cmp.Diff(v.String(), tt.expected); diff != "" {
				t.Errorf("String() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}
