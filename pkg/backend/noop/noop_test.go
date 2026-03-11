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
	_, err := b.FilterHardware(ctx, data.HardwareFilter{})
	if err == nil {
		t.Error("expected error")
	}
	if !errors.Is(err, errAlways) {
		t.Error("expected errAlways")
	}
}
