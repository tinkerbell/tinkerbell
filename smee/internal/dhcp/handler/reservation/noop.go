// Package noop is a backend handler that does nothing.
package reservation

import (
	"context"
	"errors"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

// Handler is a noop backend.
type noop struct{}

// GetByMac returns an error.
func (h noop) ReadHardware(ctx context.Context, id, namespace string, opts data.ReadListOptions) (*tinkerbell.Hardware, error) {
	return nil, errors.New("no backend specified, please specify a backend")
}
