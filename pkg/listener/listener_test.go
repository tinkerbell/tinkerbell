package listener

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

// hasIPv6 reports whether an IPv6 socket can be created at all.
func hasIPv6(t *testing.T) bool {
	t.Helper()
	c, err := net.ListenPacket(networkUDP6, "[::1]:0")
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// TestUDPDualStackSharesPort is the regression test for both families binding
// the same port. A bare "udp" listen makes the first bind claim the port for
// both families, which broke TFTP on 69 and syslog on 514.
func TestUDPDualStackSharesPort(t *testing.T) {
	if !hasIPv6(t) {
		t.Skip("IPv6 is unavailable")
	}

	// Take any free port from the IPv4 wildcard, then bind the IPv6 wildcard to it.
	v4, err := UDP(netip.AddrPortFrom(netip.IPv4Unspecified(), 0))
	if err != nil {
		t.Fatalf("IPv4 listen: %v", err)
	}
	defer v4.Close()

	port := v4.LocalAddr().(*net.UDPAddr).AddrPort().Port()
	v6, err := UDP(netip.AddrPortFrom(netip.IPv6Unspecified(), port))
	if err != nil {
		t.Fatalf("IPv6 listen on port %d already held by the IPv4 listener: %v", port, err)
	}
	defer v6.Close()
}

func TestUDPNetworkPerFamily(t *testing.T) {
	tests := map[string]struct {
		addr netip.Addr
		want string
	}{
		"IPv4 wildcard":     {netip.IPv4Unspecified(), networkUDP4},
		"IPv4 loopback":     {netip.MustParseAddr("127.0.0.1"), networkUDP4},
		"IPv4 mapped":       {netip.MustParseAddr("::ffff:127.0.0.1"), networkUDP4},
		"IPv6 wildcard":     {netip.IPv6Unspecified(), networkUDP6},
		"IPv6 loopback":     {netip.IPv6Loopback(), networkUDP6},
		"IPv6 unique local": {netip.MustParseAddr("fd00::1"), networkUDP6},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if tt.want == networkUDP6 && !hasIPv6(t) {
				t.Skip("IPv6 is unavailable")
			}
			if tt.addr.Is6() && !tt.addr.Is4In6() && !tt.addr.IsUnspecified() && !tt.addr.IsLoopback() {
				t.Skip("address is not assigned to this host")
			}
			c, err := UDP(netip.AddrPortFrom(tt.addr, 0))
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer c.Close()

			// A udp4 socket reports a 4-byte local address, a udp6 socket a 16-byte one.
			got := networkUDP6
			if c.LocalAddr().(*net.UDPAddr).IP.To4() != nil {
				got = networkUDP4
			}
			if got != tt.want {
				t.Errorf("listened on %s, want %s", got, tt.want)
			}
		})
	}
}

// TestTCPDualStackSharesPort covers the same property for TCP.
func TestTCPDualStackSharesPort(t *testing.T) {
	if !hasIPv6(t) {
		t.Skip("IPv6 is unavailable")
	}
	ctx := context.Background()

	v4, err := TCP(ctx, netip.IPv4Unspecified(), 0)
	if err != nil {
		t.Fatalf("IPv4 listen: %v", err)
	}
	defer v4.Close()

	port := v4.Addr().(*net.TCPAddr).AddrPort().Port()
	v6, err := TCP(ctx, netip.IPv6Unspecified(), int(port))
	if err != nil {
		t.Fatalf("IPv6 listen on port %d already held by the IPv4 listener: %v", port, err)
	}
	defer v6.Close()
}
