package noop

import (
	"context"
	"errors"
	"testing"

	"github.com/tinkerbell/tinkerbell/pkg/data"
)

func TestBackend(t *testing.T) {
	b := Backend{}
	ctx := context.Background()
	_, err := b.ReadHardware(ctx, "", "", data.ReadListOptions{})
	if err == nil {
		t.Error("expected error")
	}
	if !errors.Is(err, errAlways) {
		t.Error("expected errAlways")
	}
}
