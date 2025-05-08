package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"quamina.net/go/quamina"
)

// evaluateData is the data structure used for evaluating rules.
// In Quamina, this is called the "event".
type evaluationData struct {
	// Source is the Object that contains the references.
	Source source `json:"source,omitempty"`
	// Reference is a reference to another Object from the source.
	Reference tinkerbell.Reference `json:"reference,omitempty"`
}

// source is the Object that contains the references.
type source struct {
	// Name is the name of the source object.
	Name string `json:"name,omitempty"`
	// Namespace is the namespace of the source object.
	Namespace string `json:"namespace,omitempty"`
}

// evaluate checks if the data matches any rules defined.
// It returns a boolean indicating if at least one rule was matched, the rule that matched for the decision, and an error if any occurred.
func evaluate(_ context.Context, rules []string, data evaluationData) (bool, string, error) {
	q, err := quamina.New()
	if err != nil {
		return false, "", fmt.Errorf("error creating rule evaluation engine: %w", err)
	}
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
