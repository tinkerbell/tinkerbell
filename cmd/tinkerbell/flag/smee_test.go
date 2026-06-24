package flag

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
			sc.Convert(&trustedProxies, tt.publicIP, netip.Addr{}, netip.Addr{}, smee.DefaultTinkServerPort)

			if sc.Config.TinkServer.AddrPort != tt.want {
				t.Errorf("TinkServer.AddrPort = %q, want %q", sc.Config.TinkServer.AddrPort, tt.want)
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

func TestRegisterSmeeDHCPv6DNSDefaultFlags(t *testing.T) {
	cfg := &SmeeConfig{Config: smee.NewConfig(smee.Config{})}
	fs := ff.NewFlagSet("test")
	RegisterSmeeFlags(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	err := cmd.Parse([]string{
		"--dhcpv6-default-name-servers", "2001:db8::53, 2001:db8::54",
		"--dhcpv6-default-domain-search-list", "example.com, lab.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	wantNameServers := []netip.Addr{
		netip.MustParseAddr("2001:db8::53"),
		netip.MustParseAddr("2001:db8::54"),
	}
	if diff := cmp.Diff(wantNameServers, cfg.Config.DHCPv6.DefaultNameServers, cmpopts.EquateComparable(netip.Addr{})); diff != "" {
		t.Fatalf("unexpected default nameservers (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"example.com", "lab.example.com"}, cfg.Config.DHCPv6.DefaultDomainSearchList); diff != "" {
		t.Fatalf("unexpected default domain search list (-want +got):\n%s", diff)
	}
}

func TestRegisterSmeeDHCPv6DNSDefaultFlagsLastOccurrenceWins(t *testing.T) {
	cfg := &SmeeConfig{Config: smee.NewConfig(smee.Config{})}
	fs := ff.NewFlagSet("test")
	RegisterSmeeFlags(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	err := cmd.Parse([]string{
		"--dhcpv6-default-name-servers", "2001:db8::52",
		"--dhcpv6-default-domain-search-list", "old.example.com",
		"--dhcpv6-default-name-servers", "2001:db8::53",
		"--dhcpv6-default-domain-search-list", "new.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	wantNameServers := []netip.Addr{netip.MustParseAddr("2001:db8::53")}
	if diff := cmp.Diff(wantNameServers, cfg.Config.DHCPv6.DefaultNameServers, cmpopts.EquateComparable(netip.Addr{})); diff != "" {
		t.Fatalf("unexpected default nameservers (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"new.example.com"}, cfg.Config.DHCPv6.DefaultDomainSearchList); diff != "" {
		t.Fatalf("unexpected default domain search list (-want +got):\n%s", diff)
	}
}

func TestRegisterSmeeDHCPv6DNSDefaultDomainSearchRejectsInvalidNames(t *testing.T) {
	longLabel := strings.Repeat("a", 63)
	tests := map[string]string{
		"empty label":       "example.com,bad..example.com",
		"invalid character": "example.com,bad_domain.example.com",
		"label too long":    "example.com," + strings.Repeat("a", 64) + ".example.com",
		"name too long":     "example.com," + strings.Join([]string{longLabel, longLabel, longLabel, longLabel}, "."),
		"trailing dot":      "example.com,bad.example.com.",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := &SmeeConfig{Config: smee.NewConfig(smee.Config{})}
			fs := ff.NewFlagSet("test")
			RegisterSmeeFlags(&Set{FlagSet: fs}, cfg)
			cmd := &ff.Command{Name: "test", Flags: fs}

			if err := cmd.Parse([]string{"--dhcpv6-default-domain-search-list", value}); err == nil {
				t.Fatal("expected invalid default domain search list to fail")
			}
			if len(cfg.Config.DHCPv6.DefaultDomainSearchList) != 0 {
				t.Fatalf("validation failure mutated default domain search list: %v", cfg.Config.DHCPv6.DefaultDomainSearchList)
			}
		})
	}
}

func TestRegisterSmeeDHCPv6DNSDefaultEnv(t *testing.T) {
	t.Setenv("TINKERBELL_DHCPV6_DEFAULT_NAME_SERVERS", "2001:db8::53,2001:db8::54")
	t.Setenv("TINKERBELL_DHCPV6_DEFAULT_DOMAIN_SEARCH_LIST", "example.com,lab.example.com")

	cfg := &SmeeConfig{Config: smee.NewConfig(smee.Config{})}
	fs := ff.NewFlagSet("test")
	RegisterSmeeFlags(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}
	if err := cmd.Parse(nil, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		t.Fatal(err)
	}

	wantNameServers := []netip.Addr{
		netip.MustParseAddr("2001:db8::53"),
		netip.MustParseAddr("2001:db8::54"),
	}
	if diff := cmp.Diff(wantNameServers, cfg.Config.DHCPv6.DefaultNameServers, cmpopts.EquateComparable(netip.Addr{})); diff != "" {
		t.Fatalf("unexpected default nameservers (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"example.com", "lab.example.com"}, cfg.Config.DHCPv6.DefaultDomainSearchList); diff != "" {
		t.Fatalf("unexpected default domain search list (-want +got):\n%s", diff)
	}
}

func TestRegisterSmeeDHCPv6DNSDefaultNameServersRejectsInvalidAddress(t *testing.T) {
	tests := map[string]string{
		"IPv4":        "192.0.2.53",
		"IPv4-mapped": "::ffff:192.0.2.53",
		"invalid":     "invalid",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := &SmeeConfig{Config: smee.NewConfig(smee.Config{})}
			fs := ff.NewFlagSet("test")
			RegisterSmeeFlags(&Set{FlagSet: fs}, cfg)
			cmd := &ff.Command{Name: "test", Flags: fs}

			if err := cmd.Parse([]string{"--dhcpv6-default-name-servers", "2001:db8::53," + value}); err == nil {
				t.Fatal("expected invalid default nameserver to fail")
			}
		})
	}
}

func TestRegisterSmeeDHCPv6DNSDefaultNameServersRejectsInvalidEnvAddress(t *testing.T) {
	t.Setenv("TINKERBELL_DHCPV6_DEFAULT_NAME_SERVERS", "2001:db8::53,192.0.2.53")

	cfg := &SmeeConfig{Config: smee.NewConfig(smee.Config{})}
	fs := ff.NewFlagSet("test")
	RegisterSmeeFlags(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	if err := cmd.Parse(nil, ff.WithEnvVarPrefix("TINKERBELL")); err == nil {
		t.Fatal("expected invalid default nameserver environment value to fail")
	}
}

func TestRegisterSmeeEmptyDHCPv6DNSDefaults(t *testing.T) {
	cfg := &SmeeConfig{Config: smee.NewConfig(smee.Config{})}
	fs := ff.NewFlagSet("test")
	RegisterSmeeFlags(&Set{FlagSet: fs}, cfg)
	cmd := &ff.Command{Name: "test", Flags: fs}

	if err := cmd.Parse([]string{
		"--dhcpv6-default-name-servers", "",
		"--dhcpv6-default-domain-search-list", "",
	}); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Config.DHCPv6.DefaultNameServers) != 0 {
		t.Fatalf("unexpected default nameservers: %v", cfg.Config.DHCPv6.DefaultNameServers)
	}
	if len(cfg.Config.DHCPv6.DefaultDomainSearchList) != 0 {
		t.Fatalf("unexpected default domain search list: %v", cfg.Config.DHCPv6.DefaultDomainSearchList)
	}
}
