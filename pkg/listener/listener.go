// Package listener creates listeners that serve a single IP address family.
package listener

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

const (
	networkUDP4 = "udp4"
	networkUDP6 = "udp6"
)

// Bind is the listen address of a single address family. Enabled selects
// whether the family is served, and an enabled Bind must carry an address:
// serving is a decision made once rather than inferred from an address that may
// simply be unset.
//
// Addr and Port are separate because each is set by its own flag, which a
// [netip.AddrPort] cannot expose.
type Bind struct {
	Addr    netip.Addr
	Port    uint16
	Enabled bool
}

// AddrPort returns the address and port to listen on.
func (b Bind) AddrPort() netip.AddrPort {
	return netip.AddrPortFrom(b.Addr, b.Port)
}

// TCP listens on addr and port, serving only addr's address family.
//
// The network is selected per family deliberately. Go turns a wildcard listen
// on "tcp" into a dual-stack socket, so a later IPv6 wildcard on the same port
// fails as already in use. "tcp6" sets IPV6_V6ONLY, which lets an IPv4 and an
// IPv6 listener share a port.
func TCP(ctx context.Context, addr netip.Addr, port int) (net.Listener, error) {
	if !addr.IsValid() {
		return nil, fmt.Errorf("listen on port %d: no address", port)
	}

	network := "tcp6"
	if addr.Is4() || addr.Is4In6() {
		network = "tcp4"
	}

	var lc net.ListenConfig
	l, err := lc.Listen(ctx, network, net.JoinHostPort(addr.String(), strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("listen %s on %s port %d: %w", network, addr, port, err)
	}

	return l, nil
}

// UDP listens on addr, serving only addr's address family.
//
// The network is selected per family for the same reason as in [TCP]: a bare
// "udp" listen on a wildcard address yields a dual-stack socket, so binding
// both families to one port fails on the second bind. Services that listen on
// the same port in both families, such as TFTP and syslog, need this.
func UDP(addr netip.AddrPort) (*net.UDPConn, error) {
	// A zero address resolves to a nil IP, which binds the IPv6 wildcard and
	// serves a family the caller never asked for.
	if !addr.Addr().IsValid() {
		return nil, fmt.Errorf("listen on port %d: no address", addr.Port())
	}

	network := networkUDP6
	if a := addr.Addr(); a.Is4() || a.Is4In6() {
		network = networkUDP4
	}

	c, err := net.ListenUDP(network, net.UDPAddrFromAddrPort(addr))
	if err != nil {
		return nil, fmt.Errorf("listen %s on %s port %d: %w", network, addr.Addr(), addr.Port(), err)
	}

	return c, nil
}
