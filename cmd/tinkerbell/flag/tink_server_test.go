package flag

import (
	"net/netip"
	"testing"

	"github.com/tinkerbell/tinkerbell/tink/server"
)

func TestTinkServerConvertIPv6BindAddr(t *testing.T) {
	cfg := &TinkServerConfig{
		Config:   server.NewConfig(),
		BindAddr: netip.MustParseAddr("10.0.2.15"),
		BindPort: 42113,
	}

	cfg.Convert(netip.MustParseAddr("2001:db8::15"))

	if got, want := cfg.Config.BindAddrPort.String(), "[2001:db8::15]:42113"; got != want {
		t.Errorf("BindAddrPort = %q, want %q", got, want)
	}
}
