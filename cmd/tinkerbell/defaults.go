package main

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peterbourgon/ff/v4"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
	ntip "github.com/tinkerbell/tinkerbell/pkg/flag/netip"
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
	_, ok := publicIPv6Addr(ip)
	return ok
}

// publicIPv6Addr normalizes ip before checking its address family. The net
// package commonly represents IPv4 addresses as 16-byte IPv4-mapped values,
// which netip otherwise reports as IPv6.
func publicIPv6Addr(ip net.IP) (netip.Addr, bool) {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	addr = addr.Unmap()
	if !addr.Is6() || !addr.IsGlobalUnicast() || addr.IsLinkLocalUnicast() {
		return netip.Addr{}, false
	}
	return addr, true
}

func firstPublicIPv6(addrs []net.Addr) (netip.Addr, bool) {
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ip, ok := publicIPv6Addr(ipNet.IP); ok {
			return ip, true
		}
	}
	return netip.Addr{}, false
}

func autoDetectPublicIPv6() (netip.Addr, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("unable to auto-detect public IPv6: %w", err)
	}
	if ip, ok := firstPublicIPv6(addrs); ok {
		return ip, nil
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

		if ip, ok := firstPublicIPv6(addrs); ok {
			return ip, nil
		}
	}

	return netip.Addr{}, fmt.Errorf("no IPv6 default gateway found")
}

// defaultBindAddrV4 chooses the IPv4 bind address from an address detected on
// the local host. A configured public address is an advertised address and must
// not be used here because it may belong to a load balancer. The wildcard is the
// fallback so a host with no detected address still serves.
func defaultBindAddrV4(detectedIPv4 netip.Addr) netip.Addr {
	if detectedIPv4.Is4() && !detectedIPv4.IsUnspecified() {
		return detectedIPv4
	}

	return netip.IPv4Unspecified()
}

// resolveListenerFamilies records, for every service, which address families it
// serves. The global listener families are a ceiling: a service can narrow the
// set with its own enable flag but can never widen it.
//
// Configuration for an unserved family is dropped rather than rejected, so one
// set of flags or one Helm values file can drive any family, with only the
// listener families changing between deployments.
func resolveListenerFamilies(globals *flag.GlobalConfig, s *flag.SmeeConfig, ts *flag.TinkServerConfig, ssc *flag.SecondStarConfig) {
	v4 := globals.ListenerFamilies.HasV4()
	v6 := globals.ListenerFamilies.HasV6()

	s.Config.DHCP.Enabled = s.Config.DHCP.Enabled && v4
	s.Config.DHCPv6.Enabled = s.Config.DHCPv6.Enabled && v6
	s.Config.Syslog.V4.Enabled = v4
	s.Config.Syslog.V6.Enabled = v6
	s.Config.TFTP.V4.Enabled = v4
	s.Config.TFTP.V6.Enabled = v6
	ts.Config.V4.Enabled = v4
	ts.Config.V6.Enabled = v6
	ssc.Config.V4.Enabled = v4
	ssc.Config.V6.Enabled = v6

	if !v4 {
		globals.BindAddr = netip.Addr{}
		globals.PublicIP = netip.Addr{}
		s.Config.Syslog.V4.Addr = netip.Addr{}
		s.Config.TFTP.V4.Addr = netip.Addr{}
	}
	if !v6 {
		globals.BindAddrV6 = netip.Addr{}
		globals.PublicIPv6 = netip.Addr{}
		s.Config.Syslog.V6.Addr = netip.Addr{}
		s.Config.TFTP.V6.Addr = netip.Addr{}
	}
}

// servedFamilies reports the address families each service ended up serving, so
// an operator can read the resolved plan instead of inferring it from flags.
func servedFamilies(globals *flag.GlobalConfig, s *flag.SmeeConfig) map[string]string {
	families := func(v4, v6 bool) string {
		switch {
		case v4 && v6:
			return constant.ListenerFamiliesDual.String()
		case v4:
			return constant.ListenerFamiliesIPv4.String()
		case v6:
			return constant.ListenerFamiliesIPv6.String()
		default:
			return "none"
		}
	}
	v4 := globals.ListenerFamilies.HasV4()
	v6 := globals.ListenerFamilies.HasV6()

	served := map[string]string{"http": families(v4, v6)}
	if globals.EnableSmee {
		served["dhcp"] = families(s.Config.DHCP.Enabled, s.Config.DHCPv6.Enabled)
		if s.Config.TFTP.Enabled {
			served["tftp"] = families(s.Config.TFTP.V4.Enabled, s.Config.TFTP.V6.Enabled)
		}
		if s.Config.Syslog.Enabled {
			served["syslog"] = families(s.Config.Syslog.V4.Enabled, s.Config.Syslog.V6.Enabled)
		}
	}
	if globals.EnableTinkServer {
		served["tink-server"] = families(v4, v6)
	}
	if globals.EnableSecondStar {
		served["secondstar"] = families(v4, v6)
	}

	return served
}

// ignoredFamilyFlags returns the set flags scoped to a family that is not
// served. The family suffix is the one RegisterFamily builds the name from, so
// this covers every family-scoped flag without enumerating them. These are
// reported rather than rejected so one configuration can drive any family, with
// only the listener families changing between deployments.
//
// Values matching the default are skipped. The Helm chart emits every
// family-scoped environment variable, empty ones included, which counts as set;
// reporting those would bury the handful the operator actually chose.
func ignoredFamilyFlags(fs ff.Flags, families constant.ListenerFamilies) []string {
	var ignored []string
	_ = fs.WalkFlags(func(f ff.Flag) error {
		name, ok := f.GetLongName()
		if !ok || !f.IsSet() || f.GetValue() == f.GetDefault() {
			return nil
		}
		if (!families.HasV6() && strings.HasSuffix(name, "-v6")) ||
			(!families.HasV4() && strings.HasSuffix(name, "-v4")) {
			ignored = append(ignored, name)
		}

		return nil
	})
	sort.Strings(ignored)

	return ignored
}

func validatePublicAddressFamilies(publicIP, publicIPv6 netip.Addr) error {
	if publicIP.IsValid() {
		if err := ntip.ValidIPv4(publicIP); err != nil {
			return fmt.Errorf("public IPv4 address %w", err)
		}
	}
	if publicIPv6.IsValid() {
		if err := ntip.ValidIPv6(publicIPv6); err != nil {
			return fmt.Errorf("public IPv6 address %w", err)
		}
	}
	return nil
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

// leaderElectionNamespace returns the namespace for leader-election Leases:
// namespace if set, otherwise backendNamespace, otherwise "default" when running
// out of cluster with leader election enabled. An empty result makes
// controller-runtime use the pod's namespace.
func leaderElectionNamespace(inCluster, enabled bool, namespace, backendNamespace string) string {
	switch {
	case namespace != "":
		return namespace
	case backendNamespace != "":
		return backendNamespace
	case !inCluster && enabled:
		return defaultLeaderElectionNamespace
	}
	return namespace
}

// followBackendNamespace defaults the Tink Controller, Rufio, auto-discovery,
// leader-election and CRD-migration settings to follow the backend namespace.
// Settings for which isSet reports true are left as configured.
func followBackendNamespace(globals *flag.GlobalConfig, ts *flag.TinkServerConfig, tc *flag.TinkControllerConfig, rc *flag.RufioConfig, isSet func(name string) bool, inCluster bool) {
	ns := globals.BackendKubeNamespace
	tc.Config.LeaderElectionNamespace = leaderElectionNamespace(inCluster, tc.Config.EnableLeaderElection, tc.Config.LeaderElectionNamespace, ns)
	rc.Config.LeaderElectionNamespace = leaderElectionNamespace(inCluster, rc.Config.EnableLeaderElection, rc.Config.LeaderElectionNamespace, ns)
	if ns == "" {
		return
	}
	tc.Config.Namespace = ns
	rc.Config.Namespace = ns
	if !isSet(flag.TinkerbellAutoDiscoveryNamespace.Name) {
		ts.Config.Auto.Discovery.Namespace = ns
	}
	// A backend namespace usually means a namespaced credential, which cannot write cluster-scoped CRDs.
	if !isSet(flag.EnableCRDMigrations.Name) {
		globals.EnableCRDMigrations = false
	}
}
