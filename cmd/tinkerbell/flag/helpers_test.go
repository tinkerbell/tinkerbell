package flag

import (
	"testing"

	"github.com/peterbourgon/ff/v4"
	"github.com/tinkerbell/tinkerbell/rufio"
	"github.com/tinkerbell/tinkerbell/secondstar"
	"github.com/tinkerbell/tinkerbell/smee"
	"github.com/tinkerbell/tinkerbell/tink/controller"
	"github.com/tinkerbell/tinkerbell/tink/server"
	"github.com/tinkerbell/tinkerbell/tootles"
	"github.com/tinkerbell/tinkerbell/ui"
)

// newFullFlagSet registers every service's flags into one set, mirroring what
// cmd.go assembles, so tests can reason about the complete flag surface.
func newFullFlagSet(t *testing.T) *ff.FlagSet {
	t.Helper()

	fs := ff.NewFlagSet("test")
	set := &Set{FlagSet: fs}

	RegisterSmeeFlags(set, &SmeeConfig{Config: smee.NewConfig(smee.Config{})})
	RegisterTootlesFlags(set, &TootlesConfig{Config: tootles.NewConfig(tootles.Config{})})
	RegisterTinkServerFlags(set, &TinkServerConfig{Config: server.NewConfig()})
	RegisterTinkControllerFlags(set, &TinkControllerConfig{Config: controller.NewConfig()})
	RegisterRufioFlags(set, &RufioConfig{Config: rufio.NewConfig()})
	RegisterSecondStarFlags(set, &SecondStarConfig{Config: &secondstar.Config{}})
	RegisterUIFlags(set, &UIConfig{Config: ui.NewConfig()})
	RegisterGlobal(set, &GlobalConfig{})

	return fs
}

// registeredFlagNames returns the long name of every registered flag.
func registeredFlagNames(t *testing.T) map[string]bool {
	t.Helper()

	names := map[string]bool{}
	err := newFullFlagSet(t).WalkFlags(func(f ff.Flag) error {
		if name, ok := f.GetLongName(); ok {
			names[name] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk flags: %v", err)
	}

	return names
}
