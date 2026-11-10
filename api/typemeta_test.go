package api_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	bmcv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkerbellv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	tinkerbellv1alpha2 "github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell"
	bmcv1alpha2 "github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell/bmc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/randfill"
	"sigs.k8s.io/structured-merge-diff/v6/typed"
)

// apiSchemes returns the AddToScheme function of every API package, keyed by its directory in
// this module.
func apiSchemes() map[string]func(*runtime.Scheme) error {
	return map[string]func(*runtime.Scheme) error{
		"v1alpha1/bmc":            bmcv1alpha1.AddToScheme,
		"v1alpha1/tinkerbell":     tinkerbellv1alpha1.AddToScheme,
		"v1alpha2/tinkerbell":     tinkerbellv1alpha2.AddToScheme,
		"v1alpha2/tinkerbell/bmc": bmcv1alpha2.AddToScheme,
	}
}

// TestStructuredMergeDiffMatchesUnstructuredConverter checks that structured-merge-diff sees every
// API type the same way apimachinery's unstructured converter does. Field management uses
// structured-merge-diff for types without an OpenAPI schema, such as in controller-runtime's fake
// client. Unlike encoding/json and apimachinery, structured-merge-diff inlines an embedded struct only
// when its json tag says ",inline". An embedded TypeMeta tagged `json:""` ends up under a "TypeMeta"
// key, and managed-fields updates fail with "Object 'Kind' is missing".
func TestStructuredMergeDiffMatchesUnstructuredConverter(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, add := range apiSchemes() {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}

	var checked int
	for gvk, goType := range scheme.AllKnownTypes() {
		// AddToScheme also registers apimachinery's own types, such as ListOptions, in each group.
		if !strings.HasPrefix(goType.PkgPath(), "github.com/tinkerbell/tinkerbell/api/") {
			continue
		}
		checked++
		t.Run(gvk.String(), func(t *testing.T) {
			// Fixed seeds keep failures reproducible. Filled objects reach the fields that omitempty
			// drops from a zero value.
			for seed := range int64(20) {
				obj, err := scheme.New(gvk)
				if err != nil {
					t.Fatal(err)
				}
				randfill.NewWithSeed(seed).NilChance(0.2).NumElements(1, 2).Funcs(
					// FieldsV1 holds raw JSON, which random bytes aren't.
					func(f *metav1.FieldsV1, _ randfill.Continue) { f.Raw = []byte(`{"f:metadata":{}}`) },
				).Fill(obj)
				obj.GetObjectKind().SetGroupVersionKind(gvk)

				want, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
				if err != nil {
					t.Fatalf("seed %d: ToUnstructured: %v", seed, err)
				}
				tv, err := typed.DeducedParseableType.FromStructured(obj)
				if err != nil {
					t.Fatalf("seed %d: FromStructured: %v", seed, err)
				}
				// The converters pick different Go types for the same number, such as uint64 and int64.
				if diff := cmp.Diff(normalize(t, want), normalize(t, tv.AsValue().Unstructured())); diff != "" {
					t.Fatalf("seed %d: structured-merge-diff disagrees with apimachinery; does an embedded struct need `json:\",inline\"`? (-apimachinery +structured-merge-diff):\n%s", seed, diff)
				}
			}
		})
	}
	if checked == 0 {
		t.Fatal("no API types checked")
	}
}

// normalize round-trips v through JSON, so that numbers compare by value rather than by Go type.
func normalize(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestAPISchemesListsEveryAPIPackage fails when a package in this module declares AddToScheme but
// apiSchemes leaves it out, so its types would go unchecked.
func TestAPISchemesListsEveryAPIPackage(t *testing.T) {
	declaresAddToScheme := regexp.MustCompile(`\bAddToScheme\s*=|func AddToScheme\(`)
	listed := apiSchemes()
	found := map[string]bool{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if declaresAddToScheme.Match(src) {
			found[filepath.ToSlash(filepath.Dir(path))] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for dir := range found {
		if listed[dir] == nil {
			t.Errorf("%s declares AddToScheme but apiSchemes doesn't list it", dir)
		}
	}
	for dir := range listed {
		if !found[dir] {
			t.Errorf("apiSchemes lists %s, which declares no AddToScheme", dir)
		}
	}
}
