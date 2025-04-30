package workflow

import (
	"context"
	"testing"

	"github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
)

func TestMatch(t *testing.T) {
	tests := map[string]struct {
		rules         []string
		data          tinkerbell.Reference
		expectedMatch bool
		expectedRules string
		expectedErr   bool
	}{
		"no match empty rules": {
			rules: []string{},
			data: tinkerbell.Reference{
				Namespace: "tink",
				Name:      "example",
				Group:     "tinkerbell.org",
				Version:   "v1alpha1",
				Resource:  "hardware",
			},
			expectedMatch: false,
		},
		"no match empty data struct": {
			rules:         []string{`{"name": [{"wildcard": "*"}]}`},
			data:          tinkerbell.Reference{},
			expectedMatch: false,
		},
		"no match": {
			rules: []string{`{"resource": ["workflows"]}`, `{"version": ["example"]}`},
			data: tinkerbell.Reference{
				Namespace: "tink",
				Name:      "example",
				Group:     "tinkerbell.org",
				Version:   "v1alpha1",
				Resource:  "hardware",
			},
			expectedMatch: false,
		},
		"match": {
			rules: []string{`{"name": ["example"]}`},
			data: tinkerbell.Reference{
				Namespace: "tink",
				Name:      "example",
				Group:     "tinkerbell.org",
				Version:   "v1alpha1",
				Resource:  "hardware",
			},
			expectedMatch: true,
			expectedRules: `pattern-{"name": ["example"]}`,
		},
		"deny all": {
			rules: []string{`{"name": [{"wildcard": "*"}]}`},
			data: tinkerbell.Reference{
				Namespace: "tink",
				Name:      "example",
				Group:     "tinkerbell.org",
				Version:   "v1alpha1",
				Resource:  "hardware",
			},
			expectedMatch: true,
			expectedRules: `pattern-{"name": [{"wildcard": "*"}]}`,
		},
		"bad rule": {
			rules: []string{"this is not the rule format"},
			data: tinkerbell.Reference{
				Namespace: "tink",
				Name:      "example",
				Group:     "tinkerbell.org",
				Version:   "v1alpha1",
				Resource:  "hardware",
			},
			expectedMatch: false,
			expectedErr:   true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, rules, err := match(context.TODO(), test.rules, test.data)
			if err != nil && !test.expectedErr {
				t.Fatalf("match() error = %v", err)
			}
			if got != test.expectedMatch {
				t.Errorf("match() found: got = %v, want %v", got, test.expectedMatch)
			}
			if rules != test.expectedRules {
				t.Errorf("match() rules: got = %v, want %v", rules, test.expectedRules)
			}
		})
	}
}
