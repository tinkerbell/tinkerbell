package templates

import (
	"time"

	"github.com/tinkerbell/tinkerbell/pkg/build"
)

// AllNamespace is the constant value representing "all namespaces" in the UI.
// This is the single source of truth for the "all namespaces" identifier.
//
// Usage:
//   - Backend Go code: Import this package and use templates.AllNamespace
//   - Frontend JavaScript: The value is automatically injected as ALL_NAMESPACE constant
//   - Templates: Use { AllNamespace } in templ files
//
// This constant is referenced by:
//   - ui/internal/http/kube.go - List methods use this to determine namespace filtering
//   - ui/templates/layout.templ - Namespace selector UI
//   - ui/templates/scripts.templ - JavaScript namespace logic
const AllNamespace = "All"

// currentYear returns the current year as a string for use in copyright notices.
func currentYear() string {
	return time.Now().Format("2006")
}

// getVersion returns the Tinkerbell version string.
func getVersion() string {
	v := build.GitRevision()
	if v == "(devel)" {
		v = "v0.0.0-dev"
	}

	return v
}
