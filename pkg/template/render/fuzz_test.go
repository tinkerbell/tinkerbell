package render_test

import (
	"strings"
	"testing"

	"github.com/tinkerbell/tinkerbell/pkg/template/render"
)

// FuzzValue checks that Value never panics, that every rendered
// value is a string, and that no field observes another field unrendered.
func FuzzValue(f *testing.F) {
	for _, s := range []string{
		"{{ .self.name }}",
		"plain",
		"{{ .self.field }}",
		"{{ .self.a }}{{ .self.field }}",
		`{{ printf "%s" .self.field }}`,
		`{{ "{{" }}`,
		"{{ range .self }}{{ . }}{{ end }}",
		`{{ $x := .self.m }}{{ with .self.a }}{{ $x.k }}{{ end }}`,
		`{{ index .self.m "l" 0 }}`,
		"{{",
		"",
	} {
		f.Add(s)
	}

	const canary = `{{ "canary" }}`
	f.Fuzz(func(t *testing.T, tmpl string) {
		doc := map[string]any{
			"name":  "x",
			"a":     []any{"{{ .self.name }}"},
			"m":     map[string]any{"k": canary, "l": []any{canary}},
			"field": tmpl,
		}
		got, err := render.Value(doc, nil)
		if err != nil {
			return
		}
		v, ok := got.(map[string]any)["field"].(string)
		if !ok || (!strings.Contains(tmpl, "{{") && v != tmpl) {
			t.Fatalf("field = %#v for template %q", got.(map[string]any)["field"], tmpl)
		}
		if strings.Contains(v, `"canary"`) && !strings.Contains(tmpl, "canary") {
			t.Fatalf("field observed an unrendered value: %q for template %q", v, tmpl)
		}
	})
}
