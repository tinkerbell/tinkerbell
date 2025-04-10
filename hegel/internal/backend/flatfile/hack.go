package flatfile

import (
	"context"
	"errors"

	"github.com/tinkerbell/hegel/internal/frontend/hack"
)

// GetHackInstance exists to satisfy the hack.Client interface. It is not implemented.
func (b *Backend) GetHackInstance(context.Context, string) (hack.Instance, error) {
	return hack.Instance{}, errors.New("unsupported")
}
