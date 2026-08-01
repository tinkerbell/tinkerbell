package flag

import (
	"net/netip"
	"testing"

	"github.com/tinkerbell/tinkerbell/smee"
)

func TestSmeeConvertBindAddressPrecedence(t *testing.T) {
	globalBindAddr := netip.MustParseAddr("192.0.2.10")
	syslogBindAddr := netip.MustParseAddr("192.0.2.20")
	tftpBindAddr := netip.MustParseAddr("192.0.2.21")

	tests := []struct {
		name       string
		syslogAddr netip.Addr
		tftpAddr   netip.Addr
		wantSyslog netip.Addr
		wantTFTP   netip.Addr
	}{
		{
			name:       "global address is the default",
			wantSyslog: globalBindAddr,
			wantTFTP:   globalBindAddr,
		},
		{
			name:       "service-specific addresses take precedence",
			syslogAddr: syslogBindAddr,
			tftpAddr:   tftpBindAddr,
			wantSyslog: syslogBindAddr,
			wantTFTP:   tftpBindAddr,
		},
		{
			name:       "service-specific syslog address takes precedence independently",
			syslogAddr: syslogBindAddr,
			wantSyslog: syslogBindAddr,
			wantTFTP:   globalBindAddr,
		},
		{
			name:       "service-specific TFTP address takes precedence independently",
			tftpAddr:   tftpBindAddr,
			wantSyslog: globalBindAddr,
			wantTFTP:   tftpBindAddr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &SmeeConfig{Config: smee.NewConfig(smee.Config{
				Syslog: smee.Syslog{BindAddr: tt.syslogAddr},
				TFTP:   smee.TFTP{BindAddr: tt.tftpAddr},
			})}

			cfg.Convert(nil, netip.Addr{}, globalBindAddr, 8080)

			if got := cfg.Config.Syslog.BindAddr; got != tt.wantSyslog {
				t.Errorf("Syslog.BindAddr = %v, want %v", got, tt.wantSyslog)
			}
			if got := cfg.Config.TFTP.BindAddr; got != tt.wantTFTP {
				t.Errorf("TFTP.BindAddr = %v, want %v", got, tt.wantTFTP)
			}
		})
	}
}

// TestSmeeConfig_Convert_TinkServerAddrPort verifies that a user-provided hostname or IP in
// --ipxe-script-tink-server-addr-port is preserved, and that publicIP is only used as a
// fallback for the host portion. Regression test for #531.
func TestSmeeConfig_Convert_TinkServerAddrPort(t *testing.T) {
	tests := []struct {
		name          string
		inputAddrPort string
		publicIP      netip.Addr
		want          string
	}{
		{
			name:          "hostname with port preserved",
			inputAddrPort: "reboot.example.com:443",
			publicIP:      netip.MustParseAddr("192.168.1.100"),
			want:          "reboot.example.com:443",
		},
		{
			name:          "hostname without port gets default port",
			inputAddrPort: "reboot.example.com",
			publicIP:      netip.MustParseAddr("192.168.1.100"),
			want:          "reboot.example.com:42113",
		},
		{
			name:          "IP with port preserved",
			inputAddrPort: "10.0.0.1:8080",
			publicIP:      netip.MustParseAddr("192.168.1.100"),
			want:          "10.0.0.1:8080",
		},
		{
			name:          "empty input falls back to publicIP with default port",
			inputAddrPort: "",
			publicIP:      netip.MustParseAddr("192.168.1.100"),
			want:          "192.168.1.100:42113",
		},
		{
			name:          "only port specified falls back to publicIP for host",
			inputAddrPort: ":443",
			publicIP:      netip.MustParseAddr("192.168.1.100"),
			want:          "192.168.1.100:443",
		},
		{
			name:          "hostname preserved even when publicIP is unspecified",
			inputAddrPort: "reboot.example.com",
			publicIP:      netip.Addr{},
			want:          "reboot.example.com:42113",
		},
		{
			name:          "IPv6 literal with port keeps brackets",
			inputAddrPort: "[2001:db8::1]:443",
			publicIP:      netip.MustParseAddr("192.168.1.100"),
			want:          "[2001:db8::1]:443",
		},
		{
			name:          "IPv6 literal without port gets default port and keeps brackets",
			inputAddrPort: "[2001:db8::1]",
			publicIP:      netip.MustParseAddr("192.168.1.100"),
			want:          "[2001:db8::1]:42113",
		},
		{
			name:          "empty input with unspecified publicIP yields empty addr",
			inputAddrPort: "",
			publicIP:      netip.Addr{},
			want:          "",
		},
		{
			name:          "only port with unspecified publicIP yields empty addr",
			inputAddrPort: ":443",
			publicIP:      netip.Addr{},
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &SmeeConfig{
				Config: smee.NewConfig(smee.Config{}),
			}
			sc.Config.TinkServer.AddrPort = tt.inputAddrPort

			var trustedProxies []netip.Prefix
			sc.Convert(&trustedProxies, tt.publicIP, netip.Addr{}, smee.DefaultTinkServerPort)

			if sc.Config.TinkServer.AddrPort != tt.want {
				t.Errorf("TinkServer.AddrPort = %q, want %q", sc.Config.TinkServer.AddrPort, tt.want)
			}
		})
	}
}
