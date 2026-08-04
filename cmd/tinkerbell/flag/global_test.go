package flag

import (
	"net/netip"
	"testing"

	"github.com/peterbourgon/ff/v4"
)

func TestRegisterGlobalBindAddressEnv(t *testing.T) {
	t.Setenv("TINKERBELL_BIND_ADDRESS", "192.0.2.10")

	cfg := &GlobalConfig{}
	fs := ff.NewFlagSet("test")
	RegisterGlobal(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	if err := cmd.Parse(nil, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		t.Fatal(err)
	}

	if got, want := cfg.BindAddr, netip.MustParseAddr("192.0.2.10"); got != want {
		t.Errorf("BindAddr = %v, want %v", got, want)
	}
}
