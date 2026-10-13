package bootstrap

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

// Dir is where the delivered files land in the initramfs, relative to its root.
// A dedicated top-level path, so it survives the OSIE (CaptainOS's) rootfs-to-tmpfs
// transition (its init does cp -ax) and is not hidden by a later mount.
const Dir = "osie_files"

// Files returns the per-host files to deliver.
//
// Content is entirely user defined: every file comes from the Hardware's
// spec.metadata.osieFiles, placed under Dir. Nothing is synthesized, so a Hardware
// with no files produces an empty archive.
func Files(hw *tinkerbell.Hardware) ([]File, error) {
	if hw == nil || hw.Spec.Metadata == nil {
		return nil, nil
	}

	files := make([]File, 0, len(hw.Spec.Metadata.OSIEFiles))
	for i, f := range hw.Spec.Metadata.OSIEFiles {
		// Paths are relative to Dir; a leading "/" is a common way to write one
		// and means the same thing.
		p := strings.TrimSpace(strings.TrimPrefix(f.Path, "/"))
		if p == "" {
			return nil, fmt.Errorf("spec.metadata.osieFiles[%d] has an empty path", i)
		}
		mode, err := parseMode(f.Mode)
		if err != nil {
			return nil, fmt.Errorf("spec.metadata.osieFiles[%d] (%s): %w", i, p, err)
		}
		files = append(files, File{Name: Dir + "/" + p, Mode: mode, Data: []byte(f.Contents)})
	}

	return files, nil
}

// parseMode reads an octal mode as written in the API, with or without a
// leading "0" or "0o". An empty value yields DefaultMode.
//
// Bits above 0777 are rejected: setuid or setgid on a file that arrives from
// cluster state is a privilege escalation waiting to happen, and nothing needs
// them today.
func parseMode(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultMode, nil
	}
	digits := strings.TrimPrefix(strings.TrimPrefix(s, "0o"), "0O")
	v, err := strconv.ParseUint(digits, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q, want octal such as 0644", s)
	}
	if v > 0o777 {
		return 0, fmt.Errorf("invalid mode %q, must not exceed 0777", s)
	}

	return uint32(v), nil
}

// ModTime is the timestamp stamped on the delivered files.
//
// The Hardware's creation time is used because it is stable for the life of the
// object; see [Archive] for why a live clock cannot be. It does not move when
// the files themselves are edited.
func ModTime(hw *tinkerbell.Hardware) time.Time {
	if hw == nil {
		return time.Time{}
	}

	return hw.CreationTimestamp.Time
}
