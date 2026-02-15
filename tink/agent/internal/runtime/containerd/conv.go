package containerd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

// parseVolumes converts action volumes to OCI runtime spec mounts.
// Volume format: {SRC-HOST-DIR}:{TGT-CONTAINER-DIR}[:OPTIONS]
// Options can include: ro (read-only), rw (read-write, default)
// Only bind mounts with absolute or relative path sources are supported;
// named volumes are silently ignored.
// Examples:
//   - /etc/data:/data:ro     - Read-only bind mount
//   - /tmp/work:/work        - Read-write bind mount (default)
func parseVolumes(log logr.Logger, volumes []spec.Volume) []specs.Mount {
	var mounts []specs.Mount
	for _, v := range volumes {
		mount := parseVolume(log, string(v))
		if mount != nil {
			// Resolve relative paths to absolute and auto-create missing source
			// directories on the host. This matches nerdctl/Docker -v behavior
			// which calls os.MkdirAll(src, 0o755) for bind mount sources.
			resolved, err := ensureBindMountSource(mount.Source)
			if err != nil {
				log.V(1).Info("failed to ensure bind mount source, skipping volume", "source", mount.Source, "error", err)
				continue
			}
			mount.Source = resolved
			mounts = append(mounts, *mount)
		}
	}
	return mounts
}

// parseVolume parses a single volume string into a specs.Mount.
func parseVolume(log logr.Logger, volume string) *specs.Mount {
	parts := strings.SplitN(volume, ":", 3)
	if len(parts) < 2 {
		log.V(1).Info("invalid volume format, must be at least source:destination, skipping", "volume", volume)
		return nil
	}

	source := parts[0]
	destination := parts[1]
	// Destination (container path) must be non-empty and absolute.
	if destination == "" || !filepath.IsAbs(destination) {
		log.V(1).Info("invalid volume destination, must be non-empty and absolute, skipping", "volume", volume)
		return nil
	}

	if !filepath.IsAbs(source) && !strings.HasPrefix(source, ".") {
		// Skip named volumes - not supported without a volume manager
		log.V(1).Info("skipping named volume, only bind mounts with absolute or relative paths are supported", "volume", volume)
		return nil
	}

	// Default options for bind mounts
	options := []string{"rbind", "rw"}

	// Parse options if provided
	if len(parts) >= 3 {
		opts := strings.Split(parts[2], ",")
		options = []string{"rbind"}
		// Track the last-specified ro/rw option (last-wins, matching Linux mount behavior).
		rwMode := ""
		for _, opt := range opts {
			trimmed := strings.TrimSpace(opt)
			switch trimmed {
			case "ro":
				rwMode = "ro"
			case "rw":
				rwMode = "rw"
			default:
				// Pass through other options
				if trimmed != "" {
					options = append(options, trimmed)
				}
			}
		}
		// Default to rw if neither ro nor rw was specified.
		if rwMode == "" {
			rwMode = "rw"
		}
		options = append(options, rwMode)
	}

	return &specs.Mount{
		Type:        "bind",
		Source:      source,
		Destination: destination,
		Options:     options,
	}
}

// ensureBindMountSource resolves relative paths to absolute and creates the
// source directory if it does not exist, matching nerdctl's handleBindMounts
// and createDirOnHost behavior for -v style volume mounts.
// See https://github.com/containerd/nerdctl/blob/61a62f37/pkg/mountutil/mountutil.go#L165
func ensureBindMountSource(source string) (string, error) {
	// Resolve relative paths to absolute.
	if !filepath.IsAbs(source) {
		abs, err := filepath.Abs(source)
		if err != nil {
			return "", fmt.Errorf("failed to resolve absolute path for %q: %w", source, err)
		}
		source = abs
	}

	// Check if source exists.
	_, err := os.Stat(source)
	if err == nil {
		return source, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		// Source does not exist; create it as a directory.
		if err := os.MkdirAll(source, 0o755); err != nil {
			return "", fmt.Errorf("failed to create bind mount source directory %q: %w", source, err)
		}
		return source, nil
	}

	// Other errors (e.g., permission denied, I/O error, invalid path, etc.) should be returned.
	return "", fmt.Errorf("failed to stat %q: %w", source, err)
}
