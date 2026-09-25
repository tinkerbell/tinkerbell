package main

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
)

func TestValidateAddrFamilies(t *testing.T) {
	tests := map[string]struct {
		addrs   []familyAddr
		wantErr string
	}{
		"matching families": {
			addrs: []familyAddr{
				{"public-ip-v4", flag.V4, netip.MustParseAddr("192.0.2.10")},
				{"public-ip-v6", flag.V6, netip.MustParseAddr("2001:db8::10")},
			},
		},
		"unset addresses are ignored": {
			addrs: []familyAddr{{"public-ip-v4", flag.V4, netip.Addr{}}},
		},
		"IPv6 in a v4 flag": {
			addrs:   []familyAddr{{"dhcp-syslog-ip-v4", flag.V4, netip.MustParseAddr("2001:db8::10")}},
			wantErr: "--dhcp-syslog-ip-v4",
		},
		"IPv4 in a v6 flag": {
			addrs:   []familyAddr{{"dhcp-syslog-ip-v6", flag.V6, netip.MustParseAddr("192.0.2.10")}},
			wantErr: "--dhcp-syslog-ip-v6",
		},
		// An IPv4-mapped address is IPv6 by type but reaches an IPv4 peer.
		"IPv4-mapped in a v6 flag": {
			addrs:   []familyAddr{{"public-ip-v6", flag.V6, netip.MustParseAddr("::ffff:192.0.2.10")}},
			wantErr: "--public-ip-v6",
		},
		"every mismatch is reported": {
			addrs: []familyAddr{
				{"public-ip-v4", flag.V4, netip.MustParseAddr("2001:db8::10")},
				{"public-ip-v6", flag.V6, netip.MustParseAddr("192.0.2.10")},
			},
			wantErr: "--public-ip-v6",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateAddrFamilies(tt.addrs)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateAddrFamilies() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateAddrFamilies() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validateAddrFamilies() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestTopology(t *testing.T) {
	tests := map[string]struct {
		v4, v6 bool
		want   string
	}{
		"both":    {v4: true, v6: true, want: "dual-stack"},
		"v4 only": {v4: true, want: "ipv4-only"},
		"v6 only": {v6: true, want: "ipv6-only"},
		"neither": {want: topologyNone},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := topology(tt.v4, tt.v6); got != tt.want {
				t.Errorf("topology(%t, %t) = %q, want %q", tt.v4, tt.v6, got, tt.want)
			}
		})
	}
}
