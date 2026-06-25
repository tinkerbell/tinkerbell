package main

import (
	"net"
	"net/netip"
	"testing"
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

func TestDefaultBindAddr(t *testing.T) {
	tests := map[string]struct {
		publicIP   netip.Addr
		publicIPv6 netip.Addr
		dualStack  bool
		want       netip.Addr
	}{
		"dual stack without opt-in binds public ipv4": {
			publicIP:   netip.MustParseAddr("192.0.2.10"),
			publicIPv6: netip.MustParseAddr("2001:db8::10"),
			want:       netip.MustParseAddr("192.0.2.10"),
		},
		"dual stack opt-in binds wildcard v6": {
			publicIP:   netip.MustParseAddr("192.0.2.10"),
			publicIPv6: netip.MustParseAddr("2001:db8::10"),
			dualStack:  true,
			want:       netip.IPv6Unspecified(),
		},
		"ipv6 only binds wildcard v6": {
			publicIPv6: netip.MustParseAddr("2001:db8::10"),
			want:       netip.IPv6Unspecified(),
		},
		"ipv4 only binds public ipv4": {
			publicIP: netip.MustParseAddr("192.0.2.10"),
			want:     netip.MustParseAddr("192.0.2.10"),
		},
		"no public address binds wildcard v4": {
			want: netip.MustParseAddr("0.0.0.0"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := defaultBindAddr(tt.publicIP, tt.publicIPv6, tt.dualStack); got != tt.want {
				t.Fatalf("defaultBindAddr(%v, %v, %v) = %v, want %v", tt.publicIP, tt.publicIPv6, tt.dualStack, got, tt.want)
			}
		})
	}
}
