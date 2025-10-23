package noop

import (
	"context"
	"errors"
	"testing"
)

func TestBackend(t *testing.T) {
	b := Backend{}
	ctx := context.Background()
	_, err := b.GetByMac(ctx, nil)
	if err == nil {
		t.Error("expected error")
	}
	if !errors.Is(err, errAlways) {
		t.Error("expected errAlways")
	}
	_, err = b.GetByIP(ctx, nil)
	if err == nil {
		t.Error("expected error")
	}
	if !errors.Is(err, errAlways) {
		t.Error("expected errAlways")
	}
}
