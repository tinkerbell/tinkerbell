package render_test

import (
	"fmt"

	"github.com/tinkerbell/tinkerbell/pkg/template/render"
)

func ExampleValue() {
	doc := map[string]any{
		"name": "machine1",
		"fqdn": "{{ .self.name }}.{{ .references.net.domain }}",
	}
	data := map[string]any{"references": map[string]any{"net": map[string]any{"domain": "example.org"}}}

	out, err := render.Value(doc, data)
	if err != nil {
		panic(err)
	}
	fmt.Println(out.(map[string]any)["fqdn"])
	// Output: machine1.example.org
}
