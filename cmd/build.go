package cmd

import (
	"runtime/debug"
	"strconv"
)

// gitRevision retrieves the revision of the current build. If the build contains uncommitted
// changes the revision will be suffixed with "-dirty".
func gitRevision() string {
	var (
		revision    string
		dirty       bool
		gitRevision string
	)

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, i := range info.Settings {
		switch {
		case i.Key == "vcs.revision":
			revision = i.Value
		case i.Key == "vcs.modified":
			dirty, _ = strconv.ParseBool(i.Value)
		}
	}

	if len(revision) > 7 {
		revision = revision[:7]
	}
	gitRevision = revision
	if dirty {
		gitRevision += "-dirty"
	}

	return gitRevision
}
