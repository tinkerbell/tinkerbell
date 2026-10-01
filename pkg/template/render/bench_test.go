package render_test

import (
	"fmt"
	"testing"

	"github.com/tinkerbell/tinkerbell/pkg/template/render"
)

func BenchmarkValueNoTemplates(b *testing.B) {
	for b.Loop() {
		doc := map[string]any{}
		for i := range 200 {
			doc[fmt.Sprintf("key%d", i)] = fmt.Sprintf("value%d", i)
		}
		if _, err := render.Value(doc, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValueSimple(b *testing.B) {
	data := map[string]any{"hardware": map[string]any{"arch": "x86_64"}}
	for b.Loop() {
		doc := map[string]any{
			"name": "example",
			"spec": map[string]any{
				"arch":    "{{ .hardware.arch }}",
				"network": "arch={{ .self.spec.arch }} agent={{ .self.name }}",
			},
		}
		if _, err := render.Value(doc, data); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValueWide(b *testing.B) {
	for b.Loop() {
		doc := map[string]any{"name": "example"}
		for i := range 2000 {
			doc[fmt.Sprintf("k%d", i)] = "{{ .self.name }}"
		}
		if _, err := render.Value(doc, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValueDeepChain(b *testing.B) {
	const depth = 8
	for b.Loop() {
		doc := map[string]any{fmt.Sprintf("k%d", depth): "literal"}
		for i := range depth {
			doc[fmt.Sprintf("k%d", i)] = fmt.Sprintf("{{ .self.k%d }}", i+1)
		}
		if _, err := render.Value(doc, nil); err != nil {
			b.Fatal(err)
		}
	}
}
