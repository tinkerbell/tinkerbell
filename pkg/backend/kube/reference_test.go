package kube

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestResolveReferences(t *testing.T) {
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "machine1", Namespace: "tink-system"},
		Spec: tinkerbell.HardwareSpec{References: map[string]tinkerbell.Reference{
			"cm":     {Name: "cm1", Namespace: "tink-system", Version: "v1", Resource: "configmaps"},
			"secret": {Name: "s1", Namespace: "tink-system", Version: "v1", Resource: "secrets"},
		}},
	}

	tests := map[string]struct {
		allow, deny []string
		readErr     error
		want        []string
		wantErr     bool
	}{
		"deny all by default": {
			wantErr: true,
		},
		"allow list overrides the default deny": {
			allow: []string{`{"source":{"namespace":["tink-system"]}}`},
			want:  []string{"cm", "secret"},
		},
		"deny list without allow list": {
			deny:    []string{`{"reference":{"resource":["secrets"]}}`},
			want:    []string{"cm"},
			wantErr: true,
		},
		"consumer-specific rules do not authorize Hardware references": {
			allow:   []string{`{"consumer":["tink-controller"],"reference":{"resource":["configmaps"]}}`},
			wantErr: true,
		},
		"invalid rule": {
			allow:   []string{"not a rule"},
			wantErr: true,
		},
		"read error": {
			allow:   []string{`{"source":{"namespace":["tink-system"]}}`},
			readErr: errors.New("boom"),
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			b := &Backend{
				dynamicClient:                   &fakeDynamicClient{gvr: schema.GroupVersionResource{Version: "v1"}, error: tt.readErr},
				Namespace:                       "tink-system",
				HardwareReferenceAllowListRules: tt.allow,
				HardwareReferenceDenyListRules:  tt.deny,
			}
			got, err := b.ResolveReferences(context.Background(), hw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			var names []string
			for k := range got {
				names = append(names, k)
			}
			slices.Sort(names)
			if !slices.Equal(names, tt.want) {
				t.Errorf("resolved %v, want %v", names, tt.want)
			}
		})
	}
}

func TestResolveReferencesRejectsOutOfScopeNamespace(t *testing.T) {
	tests := map[string]string{
		"different namespace": "shared",
		"cluster-scoped":      "",
	}
	for name, referenceNamespace := range tests {
		t.Run(name, func(t *testing.T) {
			dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
			b := &Backend{
				Namespace:                       "tink-system",
				dynamicClient:                   dynamicClient,
				HardwareReferenceAllowListRules: []string{`{"source":{"namespace":["tink-system"]}}`},
			}
			hw := &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{Name: "machine1", Namespace: "tink-system"},
				Spec: tinkerbell.HardwareSpec{References: map[string]tinkerbell.Reference{
					"cm": {Name: "cm1", Namespace: referenceNamespace, Version: "v1", Resource: "configmaps"},
				}},
			}

			got, err := b.ResolveReferences(context.Background(), hw)
			if err == nil {
				t.Fatal("expected out-of-scope reference error")
			}
			if len(got) != 0 {
				t.Fatalf("resolved out-of-scope references: %v", got)
			}
			if actions := dynamicClient.Actions(); len(actions) != 0 {
				t.Fatalf("made API calls for out-of-scope reference: %v", actions)
			}
		})
	}
}

func TestResolveReferencesRejectsIncompleteReferences(t *testing.T) {
	tests := map[string]tinkerbell.Reference{
		"name":     {Namespace: "tink-system", Version: "v1", Resource: "configmaps"},
		"version":  {Namespace: "tink-system", Name: "cm1", Resource: "configmaps"},
		"resource": {Namespace: "tink-system", Name: "cm1", Version: "v1"},
	}
	for missingField, reference := range tests {
		t.Run(missingField, func(t *testing.T) {
			b := &Backend{
				dynamicClient:                   &fakeDynamicClient{},
				HardwareReferenceAllowListRules: []string{`{"source":{"namespace":["tink-system"]}}`},
			}
			hw := &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{Name: "machine1", Namespace: "tink-system"},
				Spec:       tinkerbell.HardwareSpec{References: map[string]tinkerbell.Reference{"cm": reference}},
			}

			got, err := b.ResolveReferences(context.Background(), hw)
			if err == nil {
				t.Fatal("expected incomplete reference error")
			}
			if len(got) != 0 {
				t.Errorf("resolved incomplete reference: %v", got)
			}
		})
	}
}

func TestResolveReferencesClusterScoped(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": "node1"},
	}}
	b := &Backend{
		dynamicClient:                   dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), node),
		HardwareReferenceAllowListRules: []string{`{"reference":{"resource":["nodes"]}}`},
	}
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "machine1", Namespace: "tink-system"},
		Spec: tinkerbell.HardwareSpec{References: map[string]tinkerbell.Reference{
			"node": {Name: "node1", Version: "v1", Resource: "nodes"},
		}},
	}

	got, err := b.ResolveReferences(context.Background(), hw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got["node"]; !ok {
		t.Errorf("cluster-scoped reference not resolved: %v", got)
	}
}

func TestResolveReferencesAppliesPolicyToNormalizedResource(t *testing.T) {
	b := &Backend{
		dynamicClient: &fakeDynamicClient{},
		HardwareReferenceDenyListRules: []string{
			`{"reference":{"group":["example.org"],"resource":["secrets"]}}`,
		},
	}
	hw := &tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "machine1", Namespace: "tink-system"},
		Spec: tinkerbell.HardwareSpec{References: map[string]tinkerbell.Reference{
			"secret": {Name: "s1", Group: "EXAMPLE.ORG", Namespace: "tink-system", Version: "v1", Resource: "SECRETS"},
		}},
	}

	got, err := b.ResolveReferences(context.Background(), hw)
	if err == nil {
		t.Fatal("expected normalized reference to match deny rule")
	}
	if len(got) != 0 {
		t.Errorf("resolved denied reference: %v", got)
	}
}

// TestDocumentedRulesMatch guards the rule examples in docs/technical/REFERENCES.md.
func TestDocumentedRulesMatch(t *testing.T) {
	ed := evaluationData{
		Source:    source{Name: "example1", Namespace: "tink-system"},
		Reference: tinkerbell.Reference{Namespace: "example", Name: "exampleLVM", Group: "example.org", Version: "v1alpha1", Resource: "lvms"},
	}
	for _, rule := range []string{
		`{"source":{"namespace":["tink-system"]}}`,
		`{"reference":{"resource":["lvms"]}}`,
		`{"source":{"namespace":["tink-system"]},"reference":{"resource":["lvms"]}}`,
		`{"source":{"name":["example1"],"namespace":["tink-system"]},"reference":{"name":["exampleLVM"],"namespace":["example"],"resource":["lvms"]}}`,
	} {
		matched, _, err := evaluate(context.Background(), []string{rule}, ed)
		if err != nil || !matched {
			t.Errorf("rule %s: matched = %v, err = %v", rule, matched, err)
		}
	}
}

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
