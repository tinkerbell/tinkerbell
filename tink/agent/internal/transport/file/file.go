package file

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Log     logr.Logger
	Actions chan spec.Action
	FileLoc string
	cancel  chan bool
}

// Start reads the file and sends the actions to the Actions channel.
func (c *Config) Start(ctx context.Context) error {
	c.Log.Info("file transport starting")
	c.cancel = make(chan bool)
	contents, err := os.ReadFile(c.FileLoc)
	if err != nil {
		return err
	}
	actions := []spec.Action{}
	if err := yaml.Unmarshal(contents, &actions); err != nil {
		return err
	}
	for _, action := range actions {
		select {
		case <-ctx.Done():
			return nil
		case <-c.cancel:
			return nil
		case c.Actions <- action:
		}
	}

	return nil
}

func (c *Config) Read(ctx context.Context) (spec.Action, error) {
	select {
	case <-ctx.Done():
		return spec.Action{}, context.Canceled
	case v := <-c.Actions:
		return v, nil
	}
}

func (c *Config) Write(_ context.Context, event spec.Event) error {
	if event.State == spec.StateFailure || event.State == spec.StateTimeout {
		c.Actions = make(chan spec.Action)
	}
	return nil
}
