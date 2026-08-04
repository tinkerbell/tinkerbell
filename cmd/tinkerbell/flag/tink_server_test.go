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

			cfg.Convert(globalBindAddr)

			if got := cfg.Config.BindAddrPort.String(); got != tt.want {
				t.Errorf("BindAddrPort = %q, want %q", got, tt.want)
			}
		})
	}
}
