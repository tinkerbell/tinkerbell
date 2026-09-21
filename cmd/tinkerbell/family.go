package main

import (
	"errors"
	"fmt"
	"net/netip"

	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
)

// topologyNone is reported for a service that serves neither family.
const topologyNone = "none"

// familyAddr is a configured address together with the family its flag promises.
type familyAddr struct {
	flag   string
	family flag.Family
	addr   netip.Addr
}

// validateAddrFamilies reports every address whose family contradicts the flag
// it was set with. Without this an IPv6 literal in a -v4 flag is accepted and
// only surfaces later as a bind or packet failure.
func validateAddrFamilies(addrs []familyAddr) error {
	var errs []error
	for _, a := range addrs {
		if !a.addr.IsValid() {
			continue
		}
		isV4 := a.addr.Is4()
		if a.family == flag.V4 && !isV4 {
			errs = append(errs, fmt.Errorf("--%s: %q is not an IPv4 address", a.flag, a.addr))
		}
		if a.family == flag.V6 && (isV4 || a.addr.Is4In6()) {
			errs = append(errs, fmt.Errorf("--%s: %q is not an IPv6 address", a.flag, a.addr))
		}
	}

	return errors.Join(errs...)
}

// configuredAddrs lists every family-scoped address a user can set.
func configuredAddrs(globals *flag.GlobalConfig, s *flag.SmeeConfig, ts *flag.TinkServerConfig, ssc *flag.SecondStarConfig) []familyAddr {
	return []familyAddr{
		{"public-ip-v4", flag.V4, globals.PublicIP},
		{"public-ip-v6", flag.V6, globals.PublicIPv6},
		{"bind-address-v4", flag.V4, globals.BindAddr},
		{"bind-address-v6", flag.V6, globals.BindAddrV6},
		{"dhcp-bind-addr-v4", flag.V4, s.Config.DHCP.BindAddr},
		{"dhcp-bind-addr-v6", flag.V6, s.Config.DHCPv6.BindAddr},
		{"dhcp-ip-for-packet-v4", flag.V4, s.Config.DHCP.IPForPacket},
		{"dhcp-syslog-ip-v4", flag.V4, s.Config.DHCP.SyslogIP},
		{"dhcp-syslog-ip-v6", flag.V6, s.Config.DHCPv6.SyslogIP},
		{"dhcp-tftp-ip-v4", flag.V4, s.Config.DHCP.TFTPIP},
		{"dhcp-tftp-ip-v6", flag.V6, s.Config.DHCPv6.TFTPIP},
		{"syslog-bind-addr-v4", flag.V4, s.Config.Syslog.V4.Addr},
		{"syslog-bind-addr-v6", flag.V6, s.Config.Syslog.V6.Addr},
		{"tftp-server-bind-addr-v4", flag.V4, s.Config.TFTP.V4.Addr},
		{"tftp-server-bind-addr-v6", flag.V6, s.Config.TFTP.V6.Addr},
		{"tink-server-bind-addr-v4", flag.V4, ts.Config.BindAddrPort.Addr()},
		{"tink-server-bind-addr-v6", flag.V6, ts.Config.BindAddrPortV6.Addr()},
		{"secondstar-bind-addr-v4", flag.V4, ssc.Config.V4.Addr()},
		{"secondstar-bind-addr-v6", flag.V6, ssc.Config.V6.Addr()},
	}
}

// topology names the address families a service serves, so that configuring
// both families is distinguishable from actually serving both.
func topology(v4, v6 bool) string {
	switch {
	case v4 && v6:
		return "dual-stack"
	case v6:
		return "ipv6-only"
	case v4:
		return "ipv4-only"
	default:
		return topologyNone
	}
}

// addressFamilies reports the topology of each service that listens or advertises.
func addressFamilies(globals *flag.GlobalConfig, s *flag.SmeeConfig, ts *flag.TinkServerConfig, ssc *flag.SecondStarConfig) []any {
	return []any{
		"advertised", topology(globals.PublicIP.IsValid(), globals.PublicIPv6.IsValid()),
		"http", topology(globals.BindAddr.IsValid(), globals.BindAddrV6.IsValid()),
		"dhcp", topology(s.Config.DHCP.Enabled, s.Config.DHCPv6.Enabled),
		"syslog", topology(s.Config.Syslog.V4.Addr.IsValid(), s.Config.Syslog.V6.Addr.IsValid()),
		"tftp", topology(s.Config.TFTP.V4.Addr.IsValid(), s.Config.TFTP.V6.Addr.IsValid()),
		"tinkServer", topology(ts.Config.BindAddrPort.Addr().IsValid(), ts.Config.BindAddrPortV6.Addr().IsValid()),
		"secondstar", topology(ssc.Config.V4.Addr().IsValid(), ssc.Config.V6.Addr().IsValid()),
	}
}
