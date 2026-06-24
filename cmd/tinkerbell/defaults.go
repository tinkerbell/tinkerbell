package main

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

const (
	smeePublicIPInterface          = "TINKERBELL_PUBLIC_IP_INTERFACE"
	defaultLeaderElectionNamespace = "default"
	defaultSecondStarPort          = 2222
	defaultHTTPPort                = 7080
	defaultHTTPSPort               = 7443
	defaultTinkServerPort          = 42113
)

func detectPublicIPv4() netip.Addr {
	if netint := os.Getenv(smeePublicIPInterface); netint != "" {
		if ip := ipByInterface(netint, func(ip net.IP) bool { return ip.To4() != nil }); ip.String() != "" && ip.IsValid() {
			return ip
		}
	}
	ipDgw, err := autoDetectPublicIpv4WithDefaultGateway()
	if err == nil {
		return ipDgw
	}

	ip, err := autoDetectPublicIPv4()
	if err != nil {
		return netip.Addr{}
	}

	return ip
}

// ipByInterface returns the first address on the named network interface that matches keep.
func ipByInterface(name string, keep func(net.IP) bool) netip.Addr {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return netip.Addr{}
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return netip.Addr{}
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		if keep(ipNet.IP) {
			ip, ok := netip.AddrFromSlice(ipNet.IP)
			if ok {
				return ip.Unmap()
			}
		}
	}

	return netip.Addr{}
}

func autoDetectPublicIPv4() (netip.Addr, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("unable to auto-detect public IPv4: %w", err)
	}
	for _, addr := range addrs {
		ip, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		v4 := ip.IP.To4()
		if v4 == nil || !v4.IsGlobalUnicast() {
			continue
		}

		return netip.AddrFrom4([4]byte(v4.To4())), nil
	}

	return netip.Addr{}, errors.New("unable to auto-detect public IPv4")
}

// autoDetectPublicIpv4WithDefaultGateway finds the network interface with a default gateway
// and returns the first net.IP address of the first interface that has a default gateway.
func autoDetectPublicIpv4WithDefaultGateway() (netip.Addr, error) {
	// Get the list of routes from netlink
	routes, err := netlink.RouteList(nil, unix.AF_INET)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("failed to list routes: %v", err)
	}

	// Find the route with a default gateway (Dst == nil)
	for _, route := range routes {
		if route.Dst == nil || route.Dst.IP.Equal(net.IPv4(0, 0, 0, 0)) && route.Gw != nil {
			// Get the interface associated with this route
			iface, err := net.InterfaceByIndex(route.LinkIndex)
			if err != nil {
				return netip.Addr{}, fmt.Errorf("failed to get interface by index: %v", err)
			}

			// Get the addresses assigned to this interface
			addrs, err := iface.Addrs()
			if err != nil {
				return netip.Addr{}, fmt.Errorf("failed to get addresses for interface %v: %v", iface.Name, err)
			}

			// Return the first valid IP address found
			for _, addr := range addrs {
				if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
					if ipNet.IP.To4() != nil {
						return netip.AddrFrom4([4]byte(ipNet.IP.To4())), nil
					}
				}
			}
		}
	}

	return netip.Addr{}, fmt.Errorf("no default gateway found")
}

func detectPublicIPv6() netip.Addr {
	if netint := os.Getenv(smeePublicIPInterface); netint != "" {
		if ip := ipByInterface(netint, isPublicInterfaceIPv6); ip.String() != "" && ip.IsValid() {
			return ip
		}
	}
	if ip, err := autoDetectPublicIPv6WithDefaultGateway(); err == nil {
		return ip
	}
	if ip, err := autoDetectPublicIPv6(); err == nil {
		return ip
	}
	return netip.Addr{}
}

func isPublicInterfaceIPv6(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	return addr.Is6() && addr.IsGlobalUnicast() && !addr.IsLinkLocalUnicast()
}

func autoDetectPublicIPv6() (netip.Addr, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("unable to auto-detect public IPv6: %w", err)
	}
	for _, addr := range addrs {
		ip, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		a, ok := netip.AddrFromSlice(ip.IP)
		if !ok || !a.Is6() || !a.IsGlobalUnicast() {
			continue
		}

		return a, nil
	}

	return netip.Addr{}, errors.New("unable to auto-detect public IPv6")
}

// autoDetectPublicIPv6WithDefaultGateway finds the network interface with an IPv6 default gateway
// and returns the first global unicast IPv6 address of the first interface that has a default gateway.
func autoDetectPublicIPv6WithDefaultGateway() (netip.Addr, error) {
	routes, err := netlink.RouteList(nil, unix.AF_INET6)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("failed to list IPv6 routes: %v", err)
	}

	for _, route := range routes {
		if route.Dst != nil || route.Gw == nil {
			continue
		}

		iface, err := net.InterfaceByIndex(route.LinkIndex)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("failed to get interface by index: %v", err)
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return netip.Addr{}, fmt.Errorf("failed to get addresses for interface %v: %v", iface.Name, err)
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipNet.IP)
			if ok && ip.Is6() && ip.IsGlobalUnicast() {
				return ip, nil
			}
		}
	}

	return netip.Addr{}, fmt.Errorf("no IPv6 default gateway found")
}

func defaultBindAddr(publicIP, publicIPv6 netip.Addr, dualStack bool) netip.Addr {
	hasIPv4 := publicIP.IsValid() && !publicIP.IsUnspecified()
	hasIPv6 := publicIPv6.IsValid() && !publicIPv6.IsUnspecified()

	if dualStack && hasIPv4 && hasIPv6 {
		return netip.IPv6Unspecified()
	}
	if hasIPv4 {
		return publicIP
	}
	if hasIPv6 {
		return netip.IPv6Unspecified()
	}
	return netip.MustParseAddr("0.0.0.0")
}

func kubeConfig() string {
	hd, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(hd, ".kube", "config")
	// if this default location doesn't exist it's highly
	// likely that Tinkerbell is being run from within the
	// cluster. In that case, the loading of the Kubernetes
	// client will only look for in cluster configuration/environment
	// variables if this is empty.
	_, oserr := os.Stat(p)
	if oserr != nil {
		return ""
	}
	return p
}

func leaderElectionNamespace(inCluster, enabled bool, namespace string) string {
	if !inCluster && enabled && namespace == "" {
		return defaultLeaderElectionNamespace
	}
	return namespace
}
