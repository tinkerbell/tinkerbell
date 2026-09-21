package smee

import (
	"net/netip"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestListenAddrs(t *testing.T) {
	v4 := Bind{Addr: netip.MustParseAddr("192.0.2.1"), Port: 69}
	v6 := Bind{Addr: netip.MustParseAddr("2001:db8::1"), Port: 69}
	// A port without an address does not describe a listener.
	portOnly := Bind{Port: 69}

	tests := map[string]struct {
		binds []Bind
		want  []netip.AddrPort
	}{
		"dual stack": {
			binds: []Bind{v4, v6},
			want: []netip.AddrPort{
				netip.AddrPortFrom(v4.Addr, 69),
				netip.AddrPortFrom(v6.Addr, 69),
			},
		},
		"IPv4 only":    {binds: []Bind{v4, portOnly}, want: []netip.AddrPort{netip.AddrPortFrom(v4.Addr, 69)}},
		"IPv6 only":    {binds: []Bind{portOnly, v6}, want: []netip.AddrPort{netip.AddrPortFrom(v6.Addr, 69)}},
		"unconfigured": {binds: []Bind{portOnly, portOnly}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := listenAddrs(tt.binds...)
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateComparable(netip.AddrPort{})); diff != "" {
				t.Errorf("listenAddrs mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDHCPDNSDefaultsKeepsIPv4Only(t *testing.T) {
	c := NewConfig(Config{
		DHCP: DHCP{
			DefaultNameServers: []netip.Addr{
				netip.MustParseAddr("192.0.2.53"),
				netip.MustParseAddr("2001:db8::53"),
			},
			DefaultDomainSearchList: []string{"example.com"},
		},
	})

	got := c.dhcpDNSDefaults()
	if len(got.NameServers) != 1 || got.NameServers[0].String() != "192.0.2.53" {
		t.Errorf("NameServers = %v, want only 192.0.2.53", got.NameServers)
	}
	if diff := cmp.Diff([]string{"example.com"}, got.DomainSearch); diff != "" {
		t.Errorf("DomainSearch mismatch (-want +got):\n%s", diff)
	}
}
