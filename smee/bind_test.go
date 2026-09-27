package smee

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// A misconfigured service must be caught before any listener runs. Reporting it
// afterwards skips g.Wait, leaving the services that did start holding their
// sockets behind the error.
func TestStartValidatesEveryServiceBeforeServing(t *testing.T) {
	syslogAddr := netip.MustParseAddr("127.0.0.1")
	probe, err := net.ListenUDP("udp4", net.UDPAddrFromAddrPort(netip.AddrPortFrom(syslogAddr, 0)))
	if err != nil {
		t.Fatal(err)
	}
	port := probe.LocalAddr().(*net.UDPAddr).AddrPort().Port()
	probe.Close()

	c := NewConfig(Config{})
	c.Backend = dhcpv6TestBackend{}
	c.DHCP.Enabled = false
	c.DHCPv6.Enabled = false
	c.Syslog = Syslog{Enabled: true, V4: Bind{Addr: syslogAddr, Port: port, Enabled: true}}
	// Enabled with no address, so planning this service fails.
	c.TFTP = TFTP{Enabled: true, V4: Bind{Port: 69, Enabled: true}}

	if err := c.Start(context.Background(), logr.Discard()); err == nil {
		t.Fatal("Start() = nil, want an error")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		conn, err := net.ListenUDP("udp4", net.UDPAddrFromAddrPort(netip.AddrPortFrom(syslogAddr, port)))
		if err != nil {
			t.Fatalf("syslog is still holding its socket after Start reported a configuration error: %v", err)
		}
		conn.Close()
	}
}

func TestEnabledBinds(t *testing.T) {
	v4 := Bind{Addr: netip.MustParseAddr("192.0.2.1"), Port: 69, Enabled: true}
	v6 := Bind{Addr: netip.MustParseAddr("2001:db8::1"), Port: 69, Enabled: true}
	disabled := Bind{Addr: netip.MustParseAddr("192.0.2.1"), Port: 69}
	// An enabled family with no address is a misconfiguration, not a listener
	// to skip.
	portOnly := Bind{Port: 69, Enabled: true}

	tests := map[string]struct {
		v4      Bind
		v6      Bind
		want    []netip.AddrPort
		wantErr bool
	}{
		"dual stack": {
			v4: v4,
			v6: v6,
			want: []netip.AddrPort{
				netip.AddrPortFrom(v4.Addr, 69),
				netip.AddrPortFrom(v6.Addr, 69),
			},
		},
		"IPv4 only":                  {v4: v4, v6: Bind{}, want: []netip.AddrPort{netip.AddrPortFrom(v4.Addr, 69)}},
		"IPv6 only":                  {v4: Bind{}, v6: v6, want: []netip.AddrPort{netip.AddrPortFrom(v6.Addr, 69)}},
		"an address is not enough":   {v4: disabled, v6: Bind{}},
		"neither family enabled":     {},
		"enabled without an address": {v4: portOnly, wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := enabledBinds("test", tt.v4, tt.v6)
			if (err != nil) != tt.wantErr {
				t.Fatalf("enabledBinds() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateComparable(netip.AddrPort{})); diff != "" {
				t.Errorf("enabledBinds mismatch (-want +got):\n%s", diff)
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
