package workflow

import (
	"context"
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestMatch(t *testing.T) {
	tests := map[string]struct {
		rules         []string
		data          evaluationData
		expectedMatch bool
		expectedRules string
		expectedErr   bool
	}{
		"no match empty rules": {
			rules: []string{},
			data: evaluationData{
				Reference: tinkerbell.Reference{
					Namespace: "tink",
					Name:      "example",
					Group:     "tinkerbell.org",
					Version:   "v1alpha1",
					Resource:  "hardware",
				},
			},
			expectedMatch: false,
		},
		"no match empty data struct": {
			rules:         []string{`{"reference":{"name":[{"wildcard":"*"}]}}`},
			data:          evaluationData{Reference: tinkerbell.Reference{}},
			expectedMatch: false,
		},
		"no match": {
			rules: []string{`{"reference":{"resource":["workflows"]}},{"version":["example"]}`},
			data: evaluationData{
				Reference: tinkerbell.Reference{
					Namespace: "tink",
					Name:      "example",
					Group:     "tinkerbell.org",
					Version:   "v1alpha1",
					Resource:  "hardware",
				},
			},
			expectedMatch: false,
		},
		"match": {
			rules: []string{`{"reference":{"name":["example"]}}`},
			data: evaluationData{
				Reference: tinkerbell.Reference{
					Namespace: "tink",
					Name:      "example",
					Group:     "tinkerbell.org",
					Version:   "v1alpha1",
					Resource:  "hardware",
				},
			},
			expectedMatch: true,
			expectedRules: `pattern-{"reference":{"name":["example"]}}`,
		},
		"deny all": {
			rules: []string{`{"reference":{"name":[{"wildcard":"*"}]}}`},
			data: evaluationData{
				Reference: tinkerbell.Reference{
					Namespace: "tink",
					Name:      "example",
					Group:     "tinkerbell.org",
					Version:   "v1alpha1",
					Resource:  "hardware",
				},
			},
			expectedMatch: true,
			expectedRules: `pattern-{"reference":{"name":[{"wildcard":"*"}]}}`,
		},
		"bad rule": {
			rules: []string{"this is not the rule format"},
			data: evaluationData{
				Reference: tinkerbell.Reference{
					Namespace: "tink",
					Name:      "example",
					Group:     "tinkerbell.org",
					Version:   "v1alpha1",
					Resource:  "hardware",
				},
			},
			expectedMatch: false,
			expectedErr:   true,
		},
		"match reference and source": {
			rules: []string{`{"reference":{"resource":["hardware"],"namespace":["tink"]},"source":{"namespace":["tink-system"]}}`},
			data: evaluationData{
				Source: source{
					Namespace: "tink-system",
				},
				Reference: tinkerbell.Reference{
					Namespace: "tink",
					Name:      "example",
					Group:     "tinkerbell.org",
					Version:   "v1alpha1",
					Resource:  "hardware",
				},
			},
			expectedMatch: true,
			expectedRules: `pattern-{"reference":{"resource":["hardware"],"namespace":["tink"]},"source":{"namespace":["tink-system"]}}`,
		},
		"case insensitive no match": {
			rules: []string{`{"reference":{"resource":["hardware"],"namespace":["tink"]},"source":{"namespace":["tink-system"]}}`},
			data: evaluationData{
				Source: source{
					Namespace: "tink-system",
				},
				Reference: tinkerbell.Reference{
					Namespace: "tink",
					Name:      "example",
					Group:     "tinkerbell.org",
					Version:   "v1alpha1",
					Resource:  "Hardware",
				},
			},
			expectedMatch: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, rules, err := evaluate(context.TODO(), test.rules, test.data)
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
