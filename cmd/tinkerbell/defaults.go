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
	smeePublicIPInterface          = "SMEE_PUBLIC_IP_INTERFACE"
	defaultLeaderElectionNamespace = "default"
)

func detectPublicIPv4() netip.Addr {
	if netint := os.Getenv(smeePublicIPInterface); netint != "" {
		if ip := ipByInterface(netint); ip.String() != "" && ip.IsValid() {
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

// ipByInterface returns the first IPv4 address on the named network interface.
func ipByInterface(name string) netip.Addr {
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

		if ipNet.IP.To4() != nil {
			return netip.AddrFrom4([4]byte(ipNet.IP.To4()))
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
