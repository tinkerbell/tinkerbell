package conv

import (
	"testing"

	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

func TestParseName(t *testing.T) {
	tests := map[string]struct {
		actionID string
		actionNm string
		want     string
	}{
		"simple alphanumeric": {
			actionID: "abc123",
			actionNm: "myaction",
			want:     "tinkerbell_myaction_abc123",
		},
		"with spaces replaced by underscores": {
			actionID: "id 1",
			actionNm: "my action",
			want:     "tinkerbell_my_action_id_1",
		},
		"with special characters": {
			actionID: "id@#$%",
			actionNm: "name!&*()",
			want:     "tinkerbell_name______id____",
		},
		"with dots and dashes preserved": {
			actionID: "id-1.2",
			actionNm: "my.action-name",
			want:     "tinkerbell_my.action-name_id-1.2",
		},
		"with underscores preserved": {
			actionID: "action_id",
			actionNm: "action_name",
			want:     "tinkerbell_action_name_action_id",
		},
		"empty strings": {
			actionID: "",
			actionNm: "",
			want:     "tinkerbell__",
		},
		"uuid style ID": {
			actionID: "550e8400-e29b-41d4-a716-446655440000",
			actionNm: "provision",
			want:     "tinkerbell_provision_550e8400-e29b-41d4-a716-446655440000",
		},
		"slashes replaced": {
			actionID: "a/b/c",
			actionNm: "x/y/z",
			want:     "tinkerbell_x_y_z_a_b_c",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := ParseName(tt.actionID, tt.actionNm)
			if got != tt.want {
				t.Errorf("ParseName(%q, %q) = %q, want %q", tt.actionID, tt.actionNm, got, tt.want)
			}
		})
	}
}

func TestParseEnv(t *testing.T) {
	tests := map[string]struct {
		envs []spec.Env
		want []string
	}{
		"nil envs": {
			envs: nil,
			want: nil,
		},
		"empty envs": {
			envs: []spec.Env{},
			want: nil,
		},
		"single env": {
			envs: []spec.Env{
				{Key: "FOO", Value: "bar"},
			},
			want: []string{"FOO=bar"},
		},
		"multiple envs": {
			envs: []spec.Env{
				{Key: "FOO", Value: "bar"},
				{Key: "BAZ", Value: "qux"},
			},
			want: []string{"FOO=bar", "BAZ=qux"},
		},
		"empty key": {
			envs: []spec.Env{
				{Key: "", Value: "bar"},
			},
			want: []string{"=bar"},
		},
		"empty value": {
			envs: []spec.Env{
				{Key: "FOO", Value: ""},
			},
			want: []string{"FOO="},
		},
		"value with equals sign": {
			envs: []spec.Env{
				{Key: "FOO", Value: "a=b=c"},
			},
			want: []string{"FOO=a=b=c"},
		},
		"value with spaces": {
			envs: []spec.Env{
				{Key: "MSG", Value: "hello world"},
			},
			want: []string{"MSG=hello world"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := ParseEnv(tt.envs)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseEnv() returned %d items, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseEnv()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
