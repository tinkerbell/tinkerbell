package agent

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

type mock struct{}

func (m *mock) Read(_ context.Context) (spec.Action, error) {
	return spec.Action{}, nil
}

func (m *mock) Execute(_ context.Context, _ spec.Action) error {
	return nil
}

func (m *mock) Write(_ context.Context, _ spec.Event) error {
	return nil
}

func TestRun(_ *testing.T) {
	c := &Config{TransportReader: &mock{}, RuntimeExecutor: &mock{}, TransportWriter: &mock{}}
	ctx, cancel := context.WithCancel(context.Background())
	go c.Run(ctx, logr.Discard())
	<-time.After(1 * time.Second)
	cancel()
}
