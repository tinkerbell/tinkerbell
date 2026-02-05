package templates_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tinkerbell/tinkerbell/ui/templates"
)

func TestHomepageRender(t *testing.T) {
	namespaces := []string{"default", "kube-system", "test"}
	cfg := templates.PageConfig{
		BaseURL:    "",
		Namespaces: namespaces,
	}
	component := templates.Homepage(cfg, templates.HardwarePageData{})

	var buf strings.Builder
	err := component.Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Failed to render homepage: %v", err)
	}

	html := buf.String()

	// Check for basic HTML structure
	if !strings.Contains(strings.ToLower(html), "<!doctype html>") {
		t.Error("Expected DOCTYPE declaration")
	}

	if !strings.Contains(html, "Tinkerbell") {
		t.Error("Expected page title")
	}

	// Check for dark mode functionality
	if !strings.Contains(html, "darkModeToggle") {
		t.Error("Expected dark mode toggle")
	}

	// Check for navigation items
	expectedNavItems := []string{"Hardware", "Templates", "Workflows", "BMC"}
	for _, item := range expectedNavItems {
		if !strings.Contains(html, item) {
			t.Errorf("Expected navigation item '%s'", item)
		}
	}

	// Check for Tailwind CSS (local compiled stylesheet)
	if !strings.Contains(html, "/css/output.css") {
		t.Error("Expected Tailwind CSS stylesheet")
	}

	// Debug: Print first 200 characters if test fails
	if t.Failed() {
		t.Logf("Rendered HTML (first 200 chars): %s", html[:min(200, len(html))])
	}
}

func TestLayoutRender(t *testing.T) {
	component := templates.Layout("Test Title", "")

	var buf strings.Builder
	err := component.Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Failed to render layout: %v", err)
	}

	html := buf.String()

	if !strings.Contains(html, "<title>Test Title</title>") {
		t.Error("Expected custom title in layout")
	}
}
