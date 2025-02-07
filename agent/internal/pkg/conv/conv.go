package conv

import (
	"fmt"
	"regexp"

	"github.com/tinkerbell/tinkerbell/agent/internal/pkg/rand"
	"github.com/tinkerbell/tinkerbell/agent/internal/spec"
)

// ParseName converts an action ID into a usable container name.
func ParseName(actionID, name string) string {
	validContainerName := regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
	// Prepend 'tinkerbell_' so we guarantee the additional constraints on the first character.
	return fmt.Sprintf(
		"tinkerbell_%s_%s_%s",
		validContainerName.ReplaceAllString(name, "_"),
		validContainerName.ReplaceAllString(actionID, "_"),
		rand.String(6),
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
