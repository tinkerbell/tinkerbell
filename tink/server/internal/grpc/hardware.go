package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// hardware returns the Hardware object for the given agentID, or nil and no error if there is none.
func (h *Handler) hardware(ctx context.Context, agentID string) (*v1alpha1.Hardware, error) {
	// Check if Hardware object already exists.
	// Pass an empty name so backends use ByAgentID as the sole selector,
	// avoiding accidental matches by object name.
	existing, err := h.Backend.FilterHardware(ctx, data.HardwareFilter{ByAgentID: agentID})
	if err == nil {
		journal.Log(ctx, "Hardware object exists")
		return existing, nil
	}

	if foundMultipleHardware(err) {
		// Multiple Hardware objects found for the same ID, this is unexpected
		journal.Log(ctx, "Multiple hardware objects found for the same ID", "error", err)
		return nil, fmt.Errorf("multiple hardware objects found for ID %s: %w", agentID, err)
	}
	if hardwareNotFound(err) {
		return nil, nil
	}

	return nil, err
}

// hardwareLookupError converts a Hardware lookup error into a gRPC status error.
// It returns nil for a nil error, and a permanent error when multiple Hardware match.
func hardwareLookupError(ctx context.Context, log logr.Logger, agentID string, err error) error {
	if err == nil {
		return nil
	}
	journal.Log(ctx, "not enrolling, Hardware lookup failed", "error", err)
	if foundMultipleHardware(err) {
		log.Error(err, "not enrolling, Hardware lookup failed")
		return backoff.Permanent(status.Errorf(codes.FailedPrecondition, "multiple Hardware objects have agent ID %s", agentID))
	}
	return errors.Join(ErrBackendRead, status.Errorf(codes.Unavailable, "error looking up Hardware: %v", err))
}

func foundMultipleHardware(e error) bool {
	type foundMultiple interface {
		MultipleFound() bool
	}
	var fn foundMultiple
	return errors.As(e, &fn) && fn.MultipleFound()
}

func hardwareNotFound(e error) bool {
	type notFound interface {
		NotFound() bool
	}
	var fn notFound
	if errors.As(e, &fn) && fn.NotFound() {
		return true
	}
	return apierrors.IsNotFound(e)
}
