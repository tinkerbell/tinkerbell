// Package build contains build specific information.
package build

import (
	"runtime/debug"
	"strconv"
)

// gitRevision is injected at build time.
var gitRevision string

func init() {
	var (
		revision string
		dirty    bool
	)

	info, _ := debug.ReadBuildInfo()
	for _, i := range info.Settings {
		switch {
		case i.Key == "vcs.revision":
			revision = i.Value
		case i.Key == "vcs.modified":
			dirty, _ = strconv.ParseBool(i.Value)
		}
	}

	gitRevision = revision
	if dirty {
		gitRevision += "-dirty"
	}
}

// GetGitRevision retrieves the revision of the current build. If the build contains uncommitted
// changes the revision will be suffixed with "-dirty".
func GetGitRevision() string {
	return gitRevision
}
