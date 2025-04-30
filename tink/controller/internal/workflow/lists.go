package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"quamina.net/go/quamina"
)

// match checks if the data matches any rules defined.
// It returns a boolean indicating if at least one rule was matched, the rule that matched for the decision, and an error if any occurred.
func match(_ context.Context, rules []string, data tinkerbell.Reference) (bool, string, error) {
	q, _ := quamina.New() // errors are ignored because they can only happen when passing in options.
	for _, r := range rules {
		if err := q.AddPattern(fmt.Sprintf("pattern-%v", r), r); err != nil {
			return false, "", fmt.Errorf("error adding matching pattern: %v err: %w", r, err)
		}
	}

	jsonEvent, err := json.Marshal(&data)
	if err != nil {
		return false, "", fmt.Errorf("error while marshalling data: %w", err)
	}
	matches, err := q.MatchesForEvent(jsonEvent)
	if err != nil {
		return false, "", fmt.Errorf("error while matching pattern: %w", err)
	}
	if len(matches) == 0 {
		return false, "", nil
	}

	var rs []string
	for idx, match := range matches {
		if m, ok := match.(string); ok {
			rs = append(rs, m)
		} else {
			rs = append(rs, fmt.Sprintf("pattern-%d", idx))
		}
	}

	return true, strings.Join(rs, ";"), nil
}
