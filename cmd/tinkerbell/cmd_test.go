package main

import (
	"testing"
)

func TestNormalizeURLPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "root slash", input: "/", want: "/"},
		{name: "already valid", input: "/ui", want: "/ui/"},
		{name: "already valid with slash", input: "/ui/", want: "/ui/"},
		{name: "missing leading slash", input: "ui", want: "/ui/"},
		{name: "whitespace around", input: "  /ui  ", want: "/ui/"},
		{name: "whitespace no slash", input: "  ui  ", want: "/ui/"},
		{name: "double slashes", input: "//ui//", want: "/ui/"},
		{name: "nested path", input: "/web/ui", want: "/web/ui/"},
		{name: "nested no leading", input: "web/ui", want: "/web/ui/"},
		{name: "dot segments", input: "/ui/../admin", want: "/admin/"},
		{name: "empty string", input: "", want: "/"},
		{name: "spaces only", input: "   ", want: "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeURLPrefix(tt.input)
			if got != tt.want {
				t.Errorf("normalizeURLPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
