// Package listener creates listeners that serve a single IP address family.
package listener

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

// TCP listens on addr and port, serving only addr's address family.
//
// The network is selected per family deliberately. Go turns a wildcard listen
// on "tcp" into a dual-stack socket, so a later IPv6 wildcard on the same port
// fails as already in use. "tcp6" sets IPV6_V6ONLY, which lets an IPv4 and an
// IPv6 listener share a port.
func TCP(ctx context.Context, addr netip.Addr, port int) (net.Listener, error) {
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
