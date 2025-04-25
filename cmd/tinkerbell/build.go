package main

import (
	"runtime/debug"
)

// gitRevision retrieves the revision of the current build. If the build contains uncommitted
// changes the revision will be suffixed with "-dirty".
func gitRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	return info.Main.Version
}
