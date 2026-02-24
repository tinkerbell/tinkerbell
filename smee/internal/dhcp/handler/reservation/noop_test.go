package reservation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

func TestNoop(t *testing.T) {
	want := errors.New("no backend specified, please specify a backend")
	_, got := noop{}.ReadHardware(context.TODO(), "", "", data.ReadListOptions{})
	if diff := cmp.Diff(want.Error(), got.Error()); diff != "" {
		t.Fatal(diff)
	}
}
