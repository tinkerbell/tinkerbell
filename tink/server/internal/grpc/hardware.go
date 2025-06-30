package grpc

import (
	"context"
	"fmt"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
)

// hardware returns the Hardware object for the given agentID.
func (h *Handler) hardware(ctx context.Context, agentID string) (*v1alpha1.Hardware, error) {
	// Check if Hardware object already exists
	existing, err := h.AutoCapabilities.Discovery.ReadHardware(ctx, agentID, h.AutoCapabilities.Discovery.Namespace)
	if err == nil {
		journal.Log(ctx, "Hardware object exists")
		return existing, nil
	}

	if foundMultipleHardware(err) {
		// Multiple Hardware objects found for the same ID, this is unexpected
		journal.Log(ctx, "Multiple hardware objects found for the same ID", "error", err)
		return nil, fmt.Errorf("multiple hardware objects found for ID %s in namespace %s: %w", agentID, h.AutoCapabilities.Discovery.Namespace, err)
	}

	return nil, err
}

func foundMultipleHardware(e error) bool {
	type foundMultiple interface {
		MultipleFound() bool
	}
	fn, ok := e.(foundMultiple)
	return ok && fn.MultipleFound()
}
