package constant

import "testing"

func TestIsDisabled(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantBool    bool
		wantReason  string
	}{
		{
			name:        "present with reason",
			annotations: map[string]string{"tinkerbell.org/disabled": "under maintenance"},
			wantBool:    true,
			wantReason:  "under maintenance",
		},
		{
			name:        "present with empty value",
			annotations: map[string]string{"tinkerbell.org/disabled": ""},
			wantBool:    true,
			wantReason:  "",
		},
		{
			name:        "absent",
			annotations: map[string]string{"other-annotation": "value"},
			wantBool:    false,
			wantReason:  "",
		},
		{
			name:        "empty map",
			annotations: map[string]string{},
			wantBool:    false,
			wantReason:  "",
		},
		{
			name:        "nil map",
			annotations: nil,
			wantBool:    false,
			wantReason:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBool, gotReason := IsDisabled(tt.annotations)
			if gotBool != tt.wantBool || gotReason != tt.wantReason {
				t.Errorf("IsDisabled() = (%v, %q), want (%v, %q)", gotBool, gotReason, tt.wantBool, tt.wantReason)
			}
		})
	}
}
