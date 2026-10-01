package render

import (
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
			q, buildErr := newRuleMatcher(test.rules)
			if buildErr != nil {
				if !test.expectedErr {
					t.Fatalf("newRuleMatcher() error = %v", buildErr)
				}
				return
			}
			if test.expectedErr {
				t.Fatal("newRuleMatcher() expected an error, got nil")
			}
			got, rules, err := evaluate(q, test.data)
			if err != nil {
				t.Fatalf("evaluate() error = %v", err)
			}
			if got != test.expectedMatch {
				t.Errorf("evaluate() found: got = %v, want %v", got, test.expectedMatch)
			}
			if rules != test.expectedRules {
				t.Errorf("evaluate() rules: got = %v, want %v", rules, test.expectedRules)
			}
		})
	}
}

func TestNewRuleMatcherReused(t *testing.T) {
	q, err := newRuleMatcher([]string{`{"reference":{"name":["example"]}}`})
	if err != nil {
		t.Fatalf("newRuleMatcher() error = %v", err)
	}

	// A matcher can be evaluated against more than one event.
	match, _, err := evaluate(q, evaluationData{Reference: tinkerbell.Reference{Name: "example"}})
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if !match {
		t.Fatal("evaluate() expected a match for the first event")
	}

	noMatch, _, err := evaluate(q, evaluationData{Reference: tinkerbell.Reference{Name: "other"}})
	if err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	if noMatch {
		t.Fatal("evaluate() expected no match for the second event")
	}
}
