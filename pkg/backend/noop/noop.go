package noop

import (
	"context"
	"errors"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

var errAlways = errors.New("noop backend always returns an error")

type Backend struct{}

func (n Backend) ReadHardware(ctx context.Context, id, namespace string, opts data.ReadListOptions) (*tinkerbell.Hardware, error) {
	return nil, errAlways
}
