package flag

import (
	"net/netip"
	"testing"

	"github.com/tinkerbell/tinkerbell/tink/server"
)

func TestTinkServerConvertBindAddressPrecedence(t *testing.T) {
	globalBindAddr := netip.MustParseAddr("192.0.2.10")
	serviceBindAddr := netip.MustParseAddr("192.0.2.20")

	tests := []struct {
		name     string
		bindAddr netip.Addr
		want     string
	}{
		{
			name: "global address is the default",
			want: "192.0.2.10:42113",
		},
		{
			name:     "service-specific address takes precedence",
			bindAddr: serviceBindAddr,
			want:     "192.0.2.20:42113",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &TinkServerConfig{
				Config:   server.NewConfig(),
				BindAddr: tt.bindAddr,
				BindPort: 42113,
			}

			cfg.Convert(globalBindAddr, netip.Addr{})

			if got := cfg.Config.V4.AddrPort().String(); got != tt.want {
				t.Errorf("V4.AddrPort = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTinkServerConvertUsesGlobalBindAddrByDefault(t *testing.T) {
	cfg := &TinkServerConfig{
		Config:     server.NewConfig(),
		BindPort:   42113,
		BindPortV6: 42113,
	}

	cfg.Convert(netip.MustParseAddr("10.0.2.15"), netip.MustParseAddr("2001:db8::15"))

	if got, want := cfg.Config.V4.AddrPort().String(), "10.0.2.15:42113"; got != want {
		t.Errorf("V4.AddrPort = %q, want %q", got, want)
	}
	if got, want := cfg.Config.V6.AddrPort().String(), "[2001:db8::15]:42113"; got != want {
		t.Errorf("V6.AddrPort = %q, want %q", got, want)
	}
}

// A family with no global and no service address is left unset so nothing binds for it.
func TestTinkServerConvertLeavesUnsetFamilyUnbound(t *testing.T) {
	cfg := &TinkServerConfig{
		Config:     server.NewConfig(),
		BindPort:   42113,
		BindPortV6: 42113,
	}

	cfg.Convert(netip.MustParseAddr("10.0.2.15"), netip.Addr{})

	if cfg.Config.V6.AddrPort().Addr().IsValid() {
		t.Errorf("V6.AddrPort = %q, want unset", cfg.Config.V6.AddrPort())
	}
}

func TestTinkServerConvertPreservesExplicitBindAddr(t *testing.T) {
	cfg := &TinkServerConfig{
		Config:     server.NewConfig(),
		BindAddr:   netip.MustParseAddr("10.0.2.15"),
		BindAddrV6: netip.MustParseAddr("2001:db8::20"),
		BindPort:   42113,
		BindPortV6: 42113,
	}

	cfg.Convert(netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("2001:db8::15"))

	if got, want := cfg.Config.V4.AddrPort().String(), "10.0.2.15:42113"; got != want {
		t.Errorf("V4.AddrPort = %q, want %q", got, want)
	}
	if got, want := cfg.Config.V6.AddrPort().String(), "[2001:db8::20]:42113"; got != want {
		t.Errorf("V6.AddrPort = %q, want %q", got, want)
	}
}
