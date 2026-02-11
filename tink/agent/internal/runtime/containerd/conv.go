package containerd

import (
	"path/filepath"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

// parseVolumes converts action volumes to OCI runtime spec mounts.
// Volume format: {SRC-HOST-DIR}:{TGT-CONTAINER-DIR}[:OPTIONS]
// Options can include: ro (read-only), rw (read-write, default)
// Examples:
//   - /etc/data:/data:ro     - Read-only bind mount
//   - /tmp/work:/work        - Read-write bind mount (default)
//   - named-volume:/data     - Read-write named volume
//   - named-volume:/data:ro  - Read-only named volume
func parseVolumes(volumes []spec.Volume) []specs.Mount {
	var mounts []specs.Mount
	for _, v := range volumes {
		mount := parseVolume(string(v))
		if mount != nil {
			mounts = append(mounts, *mount)
		}
	}
	return mounts
}

// parseVolume parses a single volume string into a specs.Mount.
func parseVolume(volume string) *specs.Mount {
	parts := strings.SplitN(volume, ":", 3)
	if len(parts) < 2 {
		return nil
	}

	source := parts[0]
	destination := parts[1]

	if !filepath.IsAbs(source) && !strings.HasPrefix(source, ".") {
		// Skip named volumes - not supported without a volume manager
		return nil
	}

	// Default options for bind mounts
	options := []string{"rbind", "rw"}

	// Parse options if provided
	if len(parts) >= 3 {
		opts := strings.Split(parts[2], ",")
		options = []string{"rbind"}
		for _, opt := range opts {
			switch strings.TrimSpace(opt) {
			case "ro":
				options = append(options, "ro")
			case "rw":
				options = append(options, "rw")
			default:
				// Pass through other options
				if opt != "" {
					options = append(options, strings.TrimSpace(opt))
				}
			}
		}
		// Ensure we have rw or ro
		hasRWOption := false
		for _, opt := range options {
			if opt == "rw" || opt == "ro" {
				hasRWOption = true
				break
			}
		}
		if !hasRWOption {
			options = append(options, "rw")
		}
	}

	return &specs.Mount{
		Type:        "bind",
		Source:      source,
		Destination: destination,
		Options:     options,
	}
}
