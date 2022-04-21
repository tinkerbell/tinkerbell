// Package build contains build specific information.
package build

// gitRevision is injected at build time.
var gitRevision string

// GetGitRevision retrieves the revision injected into the build at build time.
// Deprecated. Removed when moving to 1.18 in favor of runtime/debug.ReadBuildInfo().
func GetGitRevision() string {
	return gitRevision
}
