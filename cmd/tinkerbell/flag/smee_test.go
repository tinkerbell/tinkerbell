package flag

import (
	"net/netip"
	"testing"

	"github.com/peterbourgon/ff/v4"
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

			cfg.Convert(nil, netip.Addr{}, netip.Addr{}, globalBindAddr, 8080)

			if got := cfg.Config.Syslog.BindAddr; got != tt.wantSyslog {
				t.Errorf("Syslog.BindAddr = %v, want %v", got, tt.wantSyslog)
			}
			if got := cfg.Config.TFTP.BindAddr; got != tt.wantTFTP {
				t.Errorf("TFTP.BindAddr = %v, want %v", got, tt.wantTFTP)
			}
		})
	}
}

func TestSmeeConvertAdvertisedEndpoints(t *testing.T) {
	publicIP := netip.MustParseAddr("10.0.2.15")
	publicIPv6 := netip.MustParseAddr("2001:db8::15")
	cfg := &SmeeConfig{
		Config: smee.NewConfig(smee.Config{}),
	}

	cfg.Convert(nil, publicIP, publicIPv6, netip.Addr{}, 7080)

	if got, want := cfg.Config.DHCP.IPXEHTTPScript.URL.Host, "10.0.2.15:7080"; got != want {
		t.Errorf("IPXEHTTPScript.URL.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCP.IPXEHTTPBinaryURL.Host, "10.0.2.15:7080"; got != want {
		t.Errorf("IPXEHTTPBinaryURL.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.IPXEHTTPScript.URL.Host, "[2001:db8::15]:7080"; got != want {
		t.Errorf("DHCPv6 IPXEHTTPScript.URL.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.IPXEHTTPBinaryURL.Host, "[2001:db8::15]:7080"; got != want {
		t.Errorf("DHCPv6 IPXEHTTPBinaryURL.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCP.SyslogIP, publicIP; got != want {
		t.Errorf("DHCP SyslogIP = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCP.TFTPIP, publicIP; got != want {
		t.Errorf("DHCP TFTPIP = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCP.IPForPacket, publicIP; got != want {
		t.Errorf("DHCP IPForPacket = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.SyslogIP, publicIPv6; got != want {
		t.Errorf("DHCPv6 SyslogIP = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.TFTPIP, publicIPv6; got != want {
		t.Errorf("DHCPv6 TFTPIP = %q, want %q", got, want)
	}
	if got, want := cfg.Config.TinkServer.AddrPort, "10.0.2.15:42113"; got != want {
		t.Errorf("TinkServer.AddrPort = %q, want %q", got, want)
	}
	if got, want := cfg.Config.TinkServer.AddrPortV6, "[2001:db8::15]:42113"; got != want {
		t.Errorf("TinkServer.AddrPortV6 = %q, want %q", got, want)
	}
}

func TestSmeeConvertSeparatesAdvertisedAndBindAddresses(t *testing.T) {
	publicIP := netip.MustParseAddr("10.0.2.15")
	publicIPv6 := netip.MustParseAddr("2001:db8::15")
	bindAddr := netip.IPv6Unspecified()
	cfg := &SmeeConfig{
		Config: smee.NewConfig(smee.Config{}),
	}

	cfg.Convert(nil, publicIP, publicIPv6, bindAddr, 7080)

	if got, want := cfg.Config.DHCP.IPXEHTTPScript.URL.Host, "10.0.2.15:7080"; got != want {
		t.Errorf("DHCP advertised script host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.IPXEHTTPScript.URL.Host, "[2001:db8::15]:7080"; got != want {
		t.Errorf("DHCPv6 advertised script host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.Syslog.BindAddr, bindAddr; got != want {
		t.Errorf("Syslog.BindAddr = %q, want %q", got, want)
	}
	if got, want := cfg.Config.TFTP.BindAddr, bindAddr; got != want {
		t.Errorf("TFTP.BindAddr = %q, want %q", got, want)
	}
}

func TestSmeeConvertPreservesExplicitServiceBindAddresses(t *testing.T) {
	globalBindAddr := netip.IPv6Unspecified()
	syslogBindAddr := netip.MustParseAddr("192.0.2.20")
	tftpBindAddr := netip.MustParseAddr("192.0.2.21")
	cfg := &SmeeConfig{
		Config: smee.NewConfig(smee.Config{}),
	}
	cfg.Config.Syslog.BindAddr = syslogBindAddr
	cfg.Config.TFTP.BindAddr = tftpBindAddr

	cfg.Convert(nil, netip.Addr{}, netip.Addr{}, globalBindAddr, 7080)

	if got := cfg.Config.Syslog.BindAddr; got != syslogBindAddr {
		t.Errorf("Syslog.BindAddr = %q, want %q", got, syslogBindAddr)
	}
	if got := cfg.Config.TFTP.BindAddr; got != tftpBindAddr {
		t.Errorf("TFTP.BindAddr = %q, want %q", got, tftpBindAddr)
	}
}

func TestSmeeConvertKeepsV6DefaultsWithoutPublicIPv6(t *testing.T) {
	publicIP := netip.MustParseAddr("10.0.2.15")
	cfg := &SmeeConfig{
		Config: smee.NewConfig(smee.Config{}),
	}

	cfg.Convert(nil, publicIP, netip.Addr{}, netip.Addr{}, 7080)

	if got, want := cfg.Config.DHCP.IPXEHTTPScript.URL.Host, "10.0.2.15:7080"; got != want {
		t.Errorf("IPXEHTTPScript.URL.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCP.IPXEHTTPBinaryURL.Host, "10.0.2.15:7080"; got != want {
		t.Errorf("IPXEHTTPBinaryURL.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.IPXEHTTPScript.URL.Host, ""; got != want {
		t.Errorf("DHCPv6 IPXEHTTPScript.URL.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.IPXEHTTPBinaryURL.Host, ""; got != want {
		t.Errorf("DHCPv6 IPXEHTTPBinaryURL.Host = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.SyslogIP, (netip.Addr{}); got != want {
		t.Errorf("DHCPv6 SyslogIP = %q, want %q", got, want)
	}
	if got, want := cfg.Config.TinkServer.AddrPort, "10.0.2.15:42113"; got != want {
		t.Errorf("TinkServer.AddrPort = %q, want %q", got, want)
	}
	if got, want := cfg.Config.TinkServer.AddrPortV6, ""; got != want {
		t.Errorf("TinkServer.AddrPortV6 = %q, want %q", got, want)
	}
}

func TestRegisterSmeeFlagsV6(t *testing.T) {
	cfg := &SmeeConfig{
		Config: smee.NewConfig(smee.Config{}),
	}
	fs := ff.NewFlagSet("test")
	RegisterSmeeFlags(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	err := cmd.Parse([]string{
		"--ipxe-http-script-osie-url-v6", "http://[2001:db8::20]/hook",
		"--ipxe-script-tink-server-addr-port-v6", "[2001:db8::30]:42113",
		"--dhcpv6-enable-netboot-options=false",
		"--dhcpv6-server-duid", "00:04:12:34:56:78:12:34:56:78:90:ab:cd:ef:12:34:56:78",
		"--dhcpv6-derived-direct-address-pool", "2001:db8:abcd::/64",
		"--dhcpv6-derived-relay-address-prefix", "56",
		"--dhcpv6-bind-interface", "macvlan0,eth0",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := cfg.Config.IPXE.HTTPScriptServer.OSIEURLv6.String(), "http://[2001:db8::20]/hook"; got != want {
		t.Errorf("OSIEURLv6 = %q, want %q", got, want)
	}
	if got, want := cfg.Config.TinkServer.AddrPortV6, "[2001:db8::30]:42113"; got != want {
		t.Errorf("TinkServer.AddrPortV6 = %q, want %q", got, want)
	}
	if cfg.Config.DHCPv6.EnableNetbootOptions {
		t.Fatal("expected DHCPv6 netboot options to be disabled")
	}
	if got, want := cfg.Config.DHCPv6.ServerDUID, "00:04:12:34:56:78:12:34:56:78:90:ab:cd:ef:12:34:56:78"; got != want {
		t.Errorf("ServerDUID = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.DerivedDirectAddressPool, netip.MustParsePrefix("2001:db8:abcd::/64"); got != want {
		t.Errorf("DerivedDirectAddressPool = %q, want %q", got, want)
	}
	if got, want := cfg.Config.DHCPv6.DerivedRelayAddressPrefix, 56; got != want {
		t.Errorf("DerivedRelayAddressPrefix = %d, want %d", got, want)
	}
	if got, want := cfg.Config.DHCPv6.BindInterface, "macvlan0,eth0"; got != want {
		t.Errorf("BindInterface = %q, want %q", got, want)
	}
}
