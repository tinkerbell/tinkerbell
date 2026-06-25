package flag

import (
	"net/netip"
	"testing"

	"github.com/peterbourgon/ff/v4"
)

func TestRegisterGlobalBindAddressEnv(t *testing.T) {
	t.Setenv("TINKERBELL_BIND_ADDRESS", "::")

	cfg := &GlobalConfig{}
	fs := ff.NewFlagSet("test")
	RegisterGlobal(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	if err := cmd.Parse(nil, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		t.Fatal(err)
	}

	if got, want := cfg.BindAddr, netip.MustParseAddr("::"); got != want {
		t.Errorf("BindAddr = %v, want %v", got, want)
	}
}

func TestRegisterGlobalPublicIPv6Env(t *testing.T) {
	t.Setenv("TINKERBELL_PUBLIC_IPV6", "2001:db8::15")

	cfg := &GlobalConfig{}
	fs := ff.NewFlagSet("test")
	RegisterGlobal(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	if err := cmd.Parse(nil, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		t.Fatal(err)
	}

	if got, want := cfg.PublicIPv6, netip.MustParseAddr("2001:db8::15"); got != want {
		t.Errorf("PublicIPv6 = %v, want %v", got, want)
	}
}

func TestRegisterGlobalDualStackEnv(t *testing.T) {
	t.Setenv("TINKERBELL_DUAL_STACK", "true")

	cfg := &GlobalConfig{}
	fs := ff.NewFlagSet("test")
	RegisterGlobal(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	if err := cmd.Parse(nil, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		t.Fatal(err)
	}

	if !cfg.DualStack {
		t.Error("DualStack = false, want true")
	}
}
