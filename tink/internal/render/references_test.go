package render

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type fakeDynamicReader struct {
	objects map[string]map[string]interface{}
	err     error
}

func (f *fakeDynamicReader) DynamicRead(_ context.Context, _ schema.GroupVersionResource, name, _ string) (map[string]interface{}, error) {
	return f.objects[name], f.err
}

func TestResolveReferences(t *testing.T) {
	obj := map[string]interface{}{"spec": map[string]interface{}{"diskPath": "/dev/nvme0n1"}}
	allowExample := []string{`{"reference":{"name":["example"]}}`}
	hwWithRef := tinkerbell.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "machine1", Namespace: "default"},
		Spec: tinkerbell.HardwareSpec{
			References: map[string]tinkerbell.Reference{
				"ex": {Name: "example", Namespace: "default", Group: "tinkerbell.org", Version: "v1alpha1", Resource: "hardware"},
			},
		},
	}

	tests := map[string]struct {
		hardware tinkerbell.Hardware
		rules    ReferenceRules
		reader   *fakeDynamicReader
		want     map[string]interface{}
		wantErr  string
	}{
		"no references": {
			hardware: tinkerbell.Hardware{},
			rules:    ReferenceRules{Denylist: DefaultDenylist()},
			reader:   &fakeDynamicReader{},
			want:     map[string]interface{}{},
		},
		"denied by default": {
			hardware: hwWithRef,
			rules:    ReferenceRules{Denylist: DefaultDenylist()},
			reader:   &fakeDynamicReader{objects: map[string]map[string]interface{}{"example": obj}},
			want:     map[string]interface{}{},
			wantErr:  `reference "ex" denied`,
		},
		"allow-listed overrides the default deny-list": {
			hardware: hwWithRef,
			rules:    ReferenceRules{Allowlist: allowExample, Denylist: DefaultDenylist()},
			reader:   &fakeDynamicReader{objects: map[string]map[string]interface{}{"example": obj}},
			want:     map[string]interface{}{"ex": obj},
		},
		"read error is skipped and reported": {
			hardware: hwWithRef,
			rules:    ReferenceRules{Allowlist: allowExample, Denylist: DefaultDenylist()},
			reader:   &fakeDynamicReader{err: errors.New("boom")},
			want:     map[string]interface{}{},
			wantErr:  `reading reference "ex": boom`,
		},
		"value returned alongside an error is kept and the error dropped": {
			hardware: hwWithRef,
			rules:    ReferenceRules{Allowlist: allowExample, Denylist: DefaultDenylist()},
			reader:   &fakeDynamicReader{objects: map[string]map[string]interface{}{"example": obj}, err: errors.New("partial")},
			want:     map[string]interface{}{"ex": obj},
		},
		"invalid denylist rule is reported": {
			hardware: hwWithRef,
			rules:    ReferenceRules{Denylist: []string{"not json"}},
			reader:   &fakeDynamicReader{},
			want:     map[string]interface{}{},
			wantErr:  `evaluating denylist for reference "ex"`,
		},
		"invalid allowlist rule is reported": {
			hardware: hwWithRef,
			rules:    ReferenceRules{Allowlist: []string{"not json"}, Denylist: DefaultDenylist()},
			reader:   &fakeDynamicReader{},
			want:     map[string]interface{}{},
			wantErr:  `evaluating allowlist for reference "ex"`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := ResolveReferences(context.Background(), tc.reader, tc.rules, tc.hardware)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("ResolveReferences() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("ResolveReferences() error = %v, want it to contain %q", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ResolveReferences() diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveReferencesStableErrorOrder(t *testing.T) {
	hw := tinkerbell.Hardware{Spec: tinkerbell.HardwareSpec{References: map[string]tinkerbell.Reference{}}}
	for _, name := range []string{"e", "b", "d", "a", "c"} {
		hw.Spec.References[name] = tinkerbell.Reference{Name: name}
	}
	want := `reference "a" denied
reference "b" denied
reference "c" denied
reference "d" denied
reference "e" denied`

	for range 20 {
		_, err := ResolveReferences(context.Background(), &fakeDynamicReader{}, ReferenceRules{Denylist: DefaultDenylist()}, hw)
		if err == nil || err.Error() != want {
			t.Fatalf("ResolveReferences() error = %q, want %q", err, want)
		}
	}
}
