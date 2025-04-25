package build

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
)

// pseudoVersion represents a parsed Go module pseudo-version.
type pseudoVersion struct {
	BaseVersion string // vX.Y.Z or vX.Y.Z-pre
	CommitHash  string // Short commit hash
	IsDirty     bool   // Whether version has +dirty suffix
}

// formType represents the three possible forms of pseudo-versions.
type formType int

const (
	noBaseForm formType = iota
	preReleaseForm
	releaseForm

	// Regular expression patterns for each pseudo-version form.
	noBasePattern     = `^v(\d+)\.0\.0-(\d{8})(\d{6})-([a-f0-9]{12})(?:\+dirty)?$`
	preReleasePattern = `^v(\d+)\.(\d+)\.(\d+)-pre\.0\.(\d{8})(\d{6})-([a-f0-9]{12})(?:\+dirty)?$`
	releasePattern    = `^v(\d+)\.(\d+)\.(\d+)-0\.(\d{8})(\d{6})-([a-f0-9]{12})(?:\+dirty)?$`
)

// GitRevision retrieves the revision of the current build. If the build contains uncommitted
// changes the revision will be suffixed with "-dirty".
// This is written based on https://go.dev/ref/mod#pseudo-versions.
func GitRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return Version(info.Main.Version)
}

func Version(raw string) string {
	v, err := parse(raw)
	if err == nil {
		return v.String()
	}

	return raw
}

// parse parses a pseudo-version string into its components.
func parse(versionString string) (*pseudoVersion, error) {
	// Try each pattern in order
	patterns := []struct {
		form formType
		re   string
	}{
		{noBaseForm, noBasePattern},
		{preReleaseForm, preReleasePattern},
		{releaseForm, releasePattern},
	}

	for _, pattern := range patterns {
		matches := regexp.MustCompile(pattern.re).FindStringSubmatch(versionString)
		if len(matches) == 0 {
			continue
		}
		pv := &pseudoVersion{
			IsDirty: strings.HasSuffix(versionString, "+dirty"),
		}

		// Parse base version components
		switch pattern.form { // Use pattern.form instead of form
		case noBaseForm:
			pv.BaseVersion = fmt.Sprintf("v%s.0.0", matches[1])
		case preReleaseForm:
			pv.BaseVersion = fmt.Sprintf("v%s.%s.%s-pre", matches[1], matches[2], matches[3])
		case releaseForm:
			pv.BaseVersion = fmt.Sprintf("v%s.%s.%s", matches[1], matches[2], matches[3])
		}

		// Parse commit hash
		pv.CommitHash = matches[len(matches)-1][:8] // Truncate to 8 characters, same length as a git short hash.

		return pv, nil
	}

	return nil, fmt.Errorf("invalid pseudo-version format: %q", versionString)
}

// String returns the formatted pseudo-version string.
func (pv *pseudoVersion) String() string {
	base := fmt.Sprintf("%s-%s", pv.BaseVersion, pv.CommitHash)
	if pv.IsDirty {
		return base + "+dirty"
	}
	return base
}
