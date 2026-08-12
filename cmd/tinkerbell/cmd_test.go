package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tinkerbell/tinkerbell/pkg/build"
)

func TestExecuteVersion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout bytes.Buffer
	if err := executeWithOutput(ctx, cancel, []string{"--version"}, &stdout); err != nil {
		t.Fatalf("executeWithOutput() error = %v", err)
	}

	want := fmt.Sprintf("tinkerbell %s\n", build.GitRevision())
	if got := stdout.String(); got != want {
		t.Fatalf("executeWithOutput() output = %q, want %q", got, want)
	}
}

func TestVersionHelp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout bytes.Buffer
	err := executeWithOutput(ctx, cancel, []string{"--help"}, &stdout)
	if err == nil {
		t.Fatal("executeWithOutput() error = nil, want help output")
	}
	for _, want := range []string{"--version", "Print the version and exit"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("executeWithOutput() help does not contain %q:\n%s", want, err)
		}
	}
}

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
