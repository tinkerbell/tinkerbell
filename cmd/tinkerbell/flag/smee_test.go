package flag

import (
	"net/netip"
	"testing"

	"github.com/tinkerbell/tinkerbell/smee"
)

func TestSmeeConfig_Convert_TinkServerAddrPort(t *testing.T) {
	tests := []struct {
		name           string
		inputAddrPort  string
		publicIP       netip.Addr
		expectedResult string
	}{
		{
			name:           "hostname with port preserved",
			inputAddrPort:  "reboot.example.com:443",
			publicIP:       netip.MustParseAddr("192.168.1.100"),
			expectedResult: "reboot.example.com:443",
		},
		{
			name:           "hostname without port gets default port",
			inputAddrPort:  "reboot.example.com",
			publicIP:       netip.MustParseAddr("192.168.1.100"),
			expectedResult: "reboot.example.com:42113",
		},
		{
			name:           "IP address with port preserved",
			inputAddrPort:  "10.0.0.1:8080",
			publicIP:       netip.MustParseAddr("192.168.1.100"),
			expectedResult: "10.0.0.1:8080",
		},
		{
			name:           "empty input uses publicIP fallback",
			inputAddrPort:  "",
			publicIP:       netip.MustParseAddr("192.168.1.100"),
			expectedResult: "192.168.1.100:42113",
		},
		{
			name:           "only port specified uses publicIP for host",
			inputAddrPort:  ":443",
			publicIP:       netip.MustParseAddr("192.168.1.100"),
			expectedResult: "192.168.1.100:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &SmeeConfig{
				Config: smee.NewConfig(smee.Config{}, netip.Addr{}),
			}
			sc.Config.TinkServer.AddrPort = tt.inputAddrPort

			// Call Convert with test values
			var trustedProxies []netip.Prefix
			sc.Convert(&trustedProxies, tt.publicIP, netip.Addr{})

			if sc.Config.TinkServer.AddrPort != tt.expectedResult {
				t.Errorf("TinkServer.AddrPort = %q, want %q", sc.Config.TinkServer.AddrPort, tt.expectedResult)
			}
		})
	}
}
