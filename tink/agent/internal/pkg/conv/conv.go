package conv

import (
	"fmt"
	"regexp"
	"strings"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

// ParseName converts an action ID into a usable container name.
func ParseName(actionID, name string) string {
	validContainerName := regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
	// Prepend 'tinkerbell_' so we guarantee the additional constraints on the first character.
	return fmt.Sprintf(
		"tinkerbell_%s_%s",
		validContainerName.ReplaceAllString(name, "_"),
		validContainerName.ReplaceAllString(actionID, "_"),
	)
}

// ParseEnv converts an action's envs to a slice of strings with k=v format.
func ParseEnv(envs []spec.Env) []string {
	var de []string
	for _, env := range envs {
		de = append(de, fmt.Sprintf("%v=%v", env.Key, env.Value))
	}
	return de
}

// ParseVolumes converts action volumes to OCI runtime spec mounts.
// Volume format: {SRC-HOST-DIR}:{TGT-CONTAINER-DIR}[:OPTIONS]
// Options can include: ro (read-only), rw (read-write, default)
// Examples:
//   - /etc/data:/data:ro     - Read-only bind mount
//   - /tmp/work:/work        - Read-write bind mount (default)
func ParseVolumes(volumes []spec.Volume) []specs.Mount {
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
	parts := strings.Split(volume, ":")
	if len(parts) < 2 {
		return nil
	}

	source := parts[0]
	destination := parts[1]

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
					options = append(options, opt)
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
