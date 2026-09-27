package main

import (
	"net"
	"net/netip"
	"testing"

	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
	"github.com/tinkerbell/tinkerbell/secondstar"
	"github.com/tinkerbell/tinkerbell/smee"
	"github.com/tinkerbell/tinkerbell/tink/server"
)

func TestIsPublicInterfaceIPv6(t *testing.T) {
	tests := map[string]struct {
		ip   net.IP
		want bool
	}{
		"link local rejected":  {ip: net.ParseIP("fe80::1234"), want: false},
		"loopback rejected":    {ip: net.ParseIP("::1"), want: false},
		"multicast rejected":   {ip: net.ParseIP("ff02::1"), want: false},
		"unspecified rejected": {ip: net.ParseIP("::"), want: false},
		"ipv4 mapped rejected": {ip: net.ParseIP("::ffff:192.0.2.10"), want: false},
		"ula accepted":         {ip: net.ParseIP("fd8a:3f4b:7c91::10"), want: true},
		"global accepted":      {ip: net.ParseIP("2001:db8::10"), want: true},
		"ipv4 rejected":        {ip: net.ParseIP("192.0.2.10"), want: false},
		"nil rejected":         {ip: nil, want: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isPublicInterfaceIPv6(tt.ip); got != tt.want {
				t.Fatalf("isPublicInterfaceIPv6(%v) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestFirstPublicIPv6(t *testing.T) {
	mask4 := net.CIDRMask(24, 32)
	mask6 := net.CIDRMask(64, 128)
	tests := map[string]struct {
		addrs []net.Addr
		want  netip.Addr
		ok    bool
	}{
		"IPv4-only candidates return no address": {
			addrs: []net.Addr{
				&net.IPNet{IP: net.IPv4(192, 168, 5, 15), Mask: mask4},
			},
		},
		"IPv4 candidate before IPv6 is skipped": {
			addrs: []net.Addr{
				&net.IPNet{IP: net.IPv4(192, 168, 5, 15), Mask: mask4},
				&net.IPNet{IP: net.ParseIP("2001:db8::15"), Mask: mask6},
			},
			want: netip.MustParseAddr("2001:db8::15"),
			ok:   true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := firstPublicIPv6(tt.addrs)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("firstPublicIPv6() = (%v, %v), want (%v, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestDefaultBindAddrV4(t *testing.T) {
	tests := map[string]struct {
		detected netip.Addr
		want     netip.Addr
	}{
		"detected address is used":            {detected: netip.MustParseAddr("192.0.2.10"), want: netip.MustParseAddr("192.0.2.10")},
		"no detected address binds wildcard":  {want: netip.IPv4Unspecified()},
		"unspecified detected binds wildcard": {detected: netip.IPv4Unspecified(), want: netip.IPv4Unspecified()},
		"ipv6 detected binds wildcard":        {detected: netip.MustParseAddr("2001:db8::10"), want: netip.IPv4Unspecified()},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := defaultBindAddrV4(tt.detected); got != tt.want {
				t.Errorf("defaultBindAddrV4(%v) = %v, want %v", tt.detected, got, tt.want)
			}
		})
	}
}

// Every service records the families it serves explicitly, and configuration
// for an unserved family is dropped rather than rejected, so a single set of
// flags can drive any family.
func TestResolveListenerFamilies(t *testing.T) {
	newConfigs := func(families constant.ListenerFamilies) (*flag.GlobalConfig, *flag.SmeeConfig, *flag.TinkServerConfig, *flag.SecondStarConfig) {
		globals := &flag.GlobalConfig{
			ListenerFamilies: families,
			BindAddr:         netip.MustParseAddr("192.0.2.10"),
			BindAddrV6:       netip.MustParseAddr("2001:db8::10"),
			PublicIP:         netip.MustParseAddr("192.0.2.11"),
			PublicIPv6:       netip.MustParseAddr("2001:db8::11"),
		}
		s := &flag.SmeeConfig{Config: &smee.Config{}}
		s.Config.DHCP.Enabled = true
		s.Config.DHCPv6.Enabled = true
		s.Config.Syslog.V4.Addr = netip.MustParseAddr("192.0.2.12")
		s.Config.Syslog.V6.Addr = netip.MustParseAddr("2001:db8::12")
		s.Config.TFTP.V4.Addr = netip.MustParseAddr("192.0.2.13")
		s.Config.TFTP.V6.Addr = netip.MustParseAddr("2001:db8::13")
		ts := &flag.TinkServerConfig{Config: &server.Config{}}
		ssc := &flag.SecondStarConfig{Config: &secondstar.Config{}}

		return globals, s, ts, ssc
	}

	t.Run("ipv4 serves no ipv6 listener", func(t *testing.T) {
		globals, s, ts, ssc := newConfigs(constant.ListenerFamiliesIPv4)
		resolveListenerFamilies(globals, s, ts, ssc)

		for name, enabled := range map[string]bool{
			"DHCPv6.Enabled":    s.Config.DHCPv6.Enabled,
			"Syslog.V6.Enabled": s.Config.Syslog.V6.Enabled,
			"TFTP.V6.Enabled":   s.Config.TFTP.V6.Enabled,
			"tink V6.Enabled":   ts.Config.V6.Enabled,
			"ssh V6.Enabled":    ssc.Config.V6.Enabled,
		} {
			if enabled {
				t.Errorf("%s = true, want false", name)
			}
		}
		for name, got := range map[string]netip.Addr{
			"BindAddrV6":     globals.BindAddrV6,
			"PublicIPv6":     globals.PublicIPv6,
			"Syslog.V6.Addr": s.Config.Syslog.V6.Addr,
			"TFTP.V6.Addr":   s.Config.TFTP.V6.Addr,
		} {
			if got.IsValid() {
				t.Errorf("%s = %v, want cleared", name, got)
			}
		}
		if !globals.BindAddr.IsValid() || !s.Config.DHCP.Enabled || !ts.Config.V4.Enabled || !ssc.Config.V4.Enabled {
			t.Error("IPv4 configuration was dropped, want preserved")
		}
	})

	t.Run("ipv6 serves no ipv4 listener", func(t *testing.T) {
		globals, s, ts, ssc := newConfigs(constant.ListenerFamiliesIPv6)
		resolveListenerFamilies(globals, s, ts, ssc)

		for name, enabled := range map[string]bool{
			"DHCP.Enabled":      s.Config.DHCP.Enabled,
			"Syslog.V4.Enabled": s.Config.Syslog.V4.Enabled,
			"TFTP.V4.Enabled":   s.Config.TFTP.V4.Enabled,
			"tink V4.Enabled":   ts.Config.V4.Enabled,
			"ssh V4.Enabled":    ssc.Config.V4.Enabled,
		} {
			if enabled {
				t.Errorf("%s = true, want false", name)
			}
		}
		for name, got := range map[string]netip.Addr{
			"BindAddr":       globals.BindAddr,
			"PublicIP":       globals.PublicIP,
			"Syslog.V4.Addr": s.Config.Syslog.V4.Addr,
			"TFTP.V4.Addr":   s.Config.TFTP.V4.Addr,
		} {
			if got.IsValid() {
				t.Errorf("%s = %v, want cleared", name, got)
			}
		}
		if !globals.BindAddrV6.IsValid() || !s.Config.DHCPv6.Enabled || !ts.Config.V6.Enabled || !ssc.Config.V6.Enabled {
			t.Error("IPv6 configuration was dropped, want preserved")
		}
	})

	t.Run("dual serves both families", func(t *testing.T) {
		globals, s, ts, ssc := newConfigs(constant.ListenerFamiliesDual)
		resolveListenerFamilies(globals, s, ts, ssc)

		if !globals.BindAddr.IsValid() || !globals.BindAddrV6.IsValid() {
			t.Error("bind addresses were cleared, want both preserved")
		}
		if !s.Config.DHCP.Enabled || !s.Config.DHCPv6.Enabled {
			t.Error("DHCP was disabled, want both families enabled")
		}
		if !ts.Config.V6.Enabled || !ssc.Config.V6.Enabled {
			t.Error("service IPv6 listeners are not enabled, want enabled")
		}
	})

	// The global families are a ceiling: a service that turns itself off stays
	// off for a family the global setting serves.
	t.Run("a service can narrow but not widen", func(t *testing.T) {
		globals, s, ts, ssc := newConfigs(constant.ListenerFamiliesDual)
		s.Config.DHCPv6.Enabled = false
		resolveListenerFamilies(globals, s, ts, ssc)

		if s.Config.DHCPv6.Enabled {
			t.Error("DHCPv6.Enabled = true, want false")
		}
		if !s.Config.DHCP.Enabled {
			t.Error("DHCP.Enabled = false, want true")
		}
	})
}

func TestValidatePublicAddressFamilies(t *testing.T) {
	tests := map[string]struct {
		publicIP   netip.Addr
		publicIPv6 netip.Addr
		wantErr    bool
	}{
		"valid addresses": {
			publicIP:   netip.MustParseAddr("192.0.2.10"),
			publicIPv6: netip.MustParseAddr("2001:db8::10"),
		},
		"unset addresses": {},
		"IPv6 in IPv4 field": {
			publicIP: netip.MustParseAddr("2001:db8::10"),
			wantErr:  true,
		},
		"IPv4 in IPv6 field": {
			publicIPv6: netip.MustParseAddr("192.0.2.10"),
			wantErr:    true,
		},
		"IPv4-mapped address in IPv6 field": {
			publicIPv6: netip.MustParseAddr("::ffff:192.0.2.10"),
			wantErr:    true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := validatePublicAddressFamilies(tt.publicIP, tt.publicIPv6)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePublicAddressFamilies(%v, %v) error = %v, wantErr %v", tt.publicIP, tt.publicIPv6, err, tt.wantErr)
			}
		})
	}
}
