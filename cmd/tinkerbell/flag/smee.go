package flag

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/ccoveille/go-safecast/v2"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
	ntip "github.com/tinkerbell/tinkerbell/pkg/flag/netip"
	"github.com/tinkerbell/tinkerbell/pkg/flag/url"
	"github.com/tinkerbell/tinkerbell/smee"
)

type SmeeConfig struct {
	Config *smee.Config
	// DHCPIPXEBinary splits out some url.URL fields so they can be set individually.
	// The cmd package is responsible for putting the fields back together into a url.URL for use in service package configs.
	DHCPIPXEBinary URLBuilder
	// DHCPIPXEScript splits out some url.URL fields so they can be set individually.
	// The cmd package is responsible for putting the fields back together into a url.URL for use in service package configs.
	DHCPIPXEScript URLBuilder
	// DHCPv6IPXEBinary splits out some url.URL fields so they can be set individually.
	// The cmd package is responsible for putting the fields back together into a url.URL for use in service package configs.
	DHCPv6IPXEBinary URLBuilder
	// DHCPv6IPXEScript splits out some url.URL fields so they can be set individually.
	// The cmd package is responsible for putting the fields back together into a url.URL for use in service package configs.
	DHCPv6IPXEScript URLBuilder
	LogLevel         int
}

var KubeIndexesSmee = map[kube.IndexType]kube.Index{
	kube.IndexTypeMACAddr: kube.Indexes[kube.IndexTypeMACAddr],
	kube.IndexTypeIPAddr:  kube.Indexes[kube.IndexTypeIPAddr],
}

// URLBuilder breaks out the fields of a url.URL so they can be set individually from the CLI.
type URLBuilder struct {
	// Host is required.
	Host string
	// Port is optional.
	Port int
}

func RegisterSmeeFlags(fs *Set, sc *SmeeConfig) {
	// The order in which flags are registered here is the order they will appear in the help text.
	// DHCP flags
	fs.Register(DHCPEnabled, ffval.NewValueDefault(&sc.Config.DHCP.Enabled, sc.Config.DHCP.Enabled))
	fs.Register(DHCPEnableNetbootOptions, ffval.NewValueDefault(&sc.Config.DHCP.EnableNetbootOptions, sc.Config.DHCP.EnableNetbootOptions))
	fs.Register(DHCPModeFlag, &sc.Config.DHCP.Mode)
	fs.Register(DHCPv6Enabled, ffval.NewValueDefault(&sc.Config.DHCPv6.Enabled, sc.Config.DHCPv6.Enabled))
	fs.Register(DHCPv6EnableNetbootOptions, ffval.NewValueDefault(&sc.Config.DHCPv6.EnableNetbootOptions, sc.Config.DHCPv6.EnableNetbootOptions))
	fs.Register(DHCPv6ModeFlag, &sc.Config.DHCPv6.Mode)
	fs.Register(DHCPv6ServerDUID, ffval.NewValueDefault(&sc.Config.DHCPv6.ServerDUID, sc.Config.DHCPv6.ServerDUID))
	fs.Register(DHCPv6DerivedDirectAddressPool, &ntip.Prefix{Prefix: &sc.Config.DHCPv6.DerivedDirectAddressPool})
	fs.Register(DHCPv6DerivedRelayAddressPrefix, ffval.NewValueDefault(&sc.Config.DHCPv6.DerivedRelayAddressPrefix, sc.Config.DHCPv6.DerivedRelayAddressPrefix))
	fs.Register(DHCPv6BindAddr, &ntip.Addr{Addr: &sc.Config.DHCPv6.BindAddr})
	fs.Register(DHCPv6BindPort, ffval.NewValueDefault(&sc.Config.DHCPv6.BindPort, sc.Config.DHCPv6.BindPort))
	fs.Register(DHCPv6BindInterface, ffval.NewValueDefault(&sc.Config.DHCPv6.BindInterface, sc.Config.DHCPv6.BindInterface))
	fs.Register(DHCPv6SyslogIP, &ntip.Addr{Addr: &sc.Config.DHCPv6.SyslogIP})
	fs.Register(DHCPv6TftpIP, &ntip.Addr{Addr: &sc.Config.DHCPv6.TFTPIP})
	fs.Register(DHCPv6TftpPort, ffval.NewValueDefault(&sc.Config.DHCPv6.TFTPPort, sc.Config.DHCPv6.TFTPPort))
	fs.Register(DHCPv6IPXEHTTPScriptInjectMac, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPScript.InjectMacAddress, sc.Config.DHCPv6.IPXEHTTPScript.InjectMacAddress))
	fs.Register(DHCPv6IPXEHTTPBinaryURLScheme, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPBinaryURL.Scheme, sc.Config.DHCPv6.IPXEHTTPBinaryURL.Scheme))
	fs.Register(DHCPv6IPXEHTTPBinaryURLHost, ffval.NewValueDefault(&sc.DHCPv6IPXEBinary.Host, sc.DHCPv6IPXEBinary.Host))
	fs.Register(DHCPv6IPXEHTTPBinaryURLPort, ffval.NewValueDefault(&sc.DHCPv6IPXEBinary.Port, sc.DHCPv6IPXEBinary.Port))
	fs.Register(DHCPv6IPXEHTTPBinaryURLPath, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPBinaryURL.Path, sc.Config.DHCPv6.IPXEHTTPBinaryURL.Path))
	fs.Register(DHCPv6IPXEHTTPScriptScheme, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPScript.URL.Scheme, sc.Config.DHCPv6.IPXEHTTPScript.URL.Scheme))
	fs.Register(DHCPv6IPXEHTTPScriptHost, ffval.NewValueDefault(&sc.DHCPv6IPXEScript.Host, sc.DHCPv6IPXEScript.Host))
	fs.Register(DHCPv6IPXEHTTPScriptPort, ffval.NewValueDefault(&sc.DHCPv6IPXEScript.Port, sc.DHCPv6IPXEScript.Port))
	fs.Register(DHCPv6IPXEHTTPScriptPath, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPScript.URL.Path, sc.Config.DHCPv6.IPXEHTTPScript.URL.Path))
	fs.Register(DHCPBindAddr, &ntip.Addr{Addr: &sc.Config.DHCP.BindAddr})
	fs.Register(DHCPBindInterface, ffval.NewValueDefault(&sc.Config.DHCP.BindInterface, sc.Config.DHCP.BindInterface))
	fs.Register(DHCPIPForPacket, &ntip.Addr{Addr: &sc.Config.DHCP.IPForPacket})
	fs.Register(DHCPSyslogIP, &ntip.Addr{Addr: &sc.Config.DHCP.SyslogIP})
	fs.Register(DHCPTftpIP, &ntip.Addr{Addr: &sc.Config.DHCP.TFTPIP})
	fs.Register(DHCPTftpPort, ffval.NewValueDefault(&sc.Config.DHCP.TFTPPort, sc.Config.DHCP.TFTPPort))
	fs.Register(DHCPIPXEHTTPScriptInjectMac, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPScript.InjectMacAddress, sc.Config.DHCP.IPXEHTTPScript.InjectMacAddress))
	fs.Register(DHCPIPXEHTTPBinaryURLScheme, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPBinaryURL.Scheme, sc.Config.DHCP.IPXEHTTPBinaryURL.Scheme))
	fs.Register(DHCPIPXEHTTPBinaryURLHost, ffval.NewValueDefault(&sc.DHCPIPXEBinary.Host, sc.DHCPIPXEBinary.Host))
	fs.Register(DHCPIPXEHTTPBinaryURLPort, ffval.NewValueDefault(&sc.DHCPIPXEBinary.Port, sc.DHCPIPXEBinary.Port))
	fs.Register(DHCPIPXEHTTPBinaryURLPath, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPBinaryURL.Path, sc.Config.DHCP.IPXEHTTPBinaryURL.Path))
	fs.Register(DHCPIPXEHTTPScriptScheme, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPScript.URL.Scheme, sc.Config.DHCP.IPXEHTTPScript.URL.Scheme))
	fs.Register(DHCPIPXEHTTPScriptHost, ffval.NewValueDefault(&sc.DHCPIPXEScript.Host, sc.DHCPIPXEScript.Host))
	fs.Register(DHCPIPXEHTTPScriptPort, ffval.NewValueDefault(&sc.DHCPIPXEScript.Port, sc.DHCPIPXEScript.Port))
	fs.Register(DHCPIPXEHTTPScriptPath, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPScript.URL.Path, sc.Config.DHCP.IPXEHTTPScript.URL.Path))

	// IPXE flags
	fs.Register(IPXEArchMapping, &ffval.Value[map[iana.Arch]constant.IPXEBinary]{
		ParseFunc: func(s string) (map[iana.Arch]constant.IPXEBinary, error) {
			if s == "" {
				return nil, nil
			}
			split := strings.Split(s, ",")
			m := make(map[iana.Arch]constant.IPXEBinary, len(split))
			for _, pair := range split {
				kv := strings.SplitN(pair, "=", 2)
				if len(kv) != 2 {
					return nil, fmt.Errorf("invalid format for IPXEArchMapping: %v, expected <arch>=<binary>, see the iPXE Architecture Mapping documentation for more details", kv)
				}
				// convert the key to an uint16
				// convert the value to a smee.IPXEBinary
				key, err := strconv.Atoi(strings.TrimSpace(kv[0]))
				if err != nil {
					return nil, fmt.Errorf("invalid architecture in IPXEArchMapping: %q, must be a number, see the iPXE Architecture Mapping documentation for more details", kv[0])
				}
				ukey, err := safecast.Convert[uint16](key)
				if err != nil {
					return nil, fmt.Errorf("invalid architecture in IPXEArchMapping: %q, must be a number (uint16), see the iPXE Architecture Mapping documentation for more details", kv[0])
				}
				arch := iana.Arch(ukey)
				binary := constant.IPXEBinary(strings.TrimSpace(kv[1]))

				m[arch] = binary
			}

			return m, nil
		},
		Pointer: &sc.Config.IPXE.IPXEBinary.IPXEArchMapping,
		Default: sc.Config.IPXE.IPXEBinary.IPXEArchMapping,
	})
	fs.Register(IPXEEmbeddedScriptPatch, ffval.NewValueDefault(&sc.Config.IPXE.EmbeddedScriptPatch, sc.Config.IPXE.EmbeddedScriptPatch))
	fs.Register(IPXEHTTPBinaryEnabled, ffval.NewValueDefault(&sc.Config.IPXE.HTTPBinaryServer.Enabled, sc.Config.IPXE.HTTPBinaryServer.Enabled))
	fs.Register(IPXEHTTPScriptEnabled, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.Enabled, sc.Config.IPXE.HTTPScriptServer.Enabled))
	fs.Register(IPXEHTTPScriptExtraKernelArgs, ffval.NewList(&sc.Config.IPXE.HTTPScriptServer.ExtraKernelArgs))
	fs.Register(IPXEHTTPScriptKernelName, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.KernelName, sc.Config.IPXE.HTTPScriptServer.KernelName))
	fs.Register(IPXEHTTPScriptInitrdName, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.InitrdName, sc.Config.IPXE.HTTPScriptServer.InitrdName))
	fs.Register(IPXEHTTPScriptTrustedProxies, ffval.NewList(&sc.Config.IPXE.HTTPScriptServer.TrustedProxies))
	fs.Register(IPXEHTTPScriptRetries, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.Retries, sc.Config.IPXE.HTTPScriptServer.Retries))
	fs.Register(IPXEHTTPScriptRetryDelay, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.RetryDelay, sc.Config.IPXE.HTTPScriptServer.RetryDelay))
	fs.Register(IPXEHTTPScriptOSIEURL, &url.URL{URL: sc.Config.IPXE.HTTPScriptServer.OSIEURL})
	fs.Register(IPXEHTTPScriptOSIEURLv6, &url.URL{URL: sc.Config.IPXE.HTTPScriptServer.OSIEURLv6})
	fs.Register(IPXEBinaryInjectMacAddrFormat, &ffval.Enum[constant.MACFormat]{
		ParseFunc: macAddrFormatParser,
		Valid:     []constant.MACFormat{constant.MacAddrFormatColon, constant.MacAddrFormatDot, constant.MacAddrFormatDash, constant.MacAddrFormatNoDelimiter},
		Pointer:   &sc.Config.IPXE.IPXEBinary.InjectMacAddrFormat,
		Default:   constant.MacAddrFormatColon,
	})

	// iPXE Tink Server Flags
	fs.Register(TinkServerAddrPort, ffval.NewValueDefault(&sc.Config.TinkServer.AddrPort, sc.Config.TinkServer.AddrPort))
	fs.Register(TinkServerAddrPortV6, ffval.NewValueDefault(&sc.Config.TinkServer.AddrPortV6, sc.Config.TinkServer.AddrPortV6))
	fs.Register(TinkServerUseTLS, ffval.NewValueDefault(&sc.Config.TinkServer.UseTLS, sc.Config.TinkServer.UseTLS))
	fs.Register(TinkServerInsecureTLS, ffval.NewValueDefault(&sc.Config.TinkServer.InsecureTLS, sc.Config.TinkServer.InsecureTLS))

	// ISO Flags
	fs.Register(ISOEnabled, ffval.NewValueDefault(&sc.Config.ISO.Enabled, sc.Config.ISO.Enabled))
	fs.Register(ISOUpstreamURL, &url.URL{URL: sc.Config.ISO.UpstreamURL})
	fs.Register(ISOPatchMagicString, ffval.NewValueDefault(&sc.Config.ISO.PatchMagicString, sc.Config.ISO.PatchMagicString))
	fs.Register(ISOStaticIPAMEnabled, ffval.NewValueDefault(&sc.Config.ISO.StaticIPAMEnabled, sc.Config.ISO.StaticIPAMEnabled))

	// Log level
	fs.Register(SmeeLogLevel, ffval.NewValueDefault(&sc.LogLevel, sc.LogLevel))

	// Syslog Flags
	fs.Register(SyslogEnabled, ffval.NewValueDefault(&sc.Config.Syslog.Enabled, sc.Config.Syslog.Enabled))
	fs.Register(SyslogBindAddr, &ntip.Addr{Addr: &sc.Config.Syslog.BindAddr})
	fs.Register(SyslogBindPort, ffval.NewValueDefault(&sc.Config.Syslog.BindPort, sc.Config.Syslog.BindPort))

	// TFTP Flags
	fs.Register(TFTPServerEnabled, ffval.NewValueDefault(&sc.Config.TFTP.Enabled, sc.Config.TFTP.Enabled))
	fs.Register(TFTPServerBindAddr, &ntip.Addr{Addr: &sc.Config.TFTP.BindAddr})
	fs.Register(TFTPServerBindPort, ffval.NewValueDefault(&sc.Config.TFTP.BindPort, sc.Config.TFTP.BindPort))
	fs.Register(TFTPTimeout, ffval.NewValueDefault(&sc.Config.TFTP.Timeout, sc.Config.TFTP.Timeout))
	fs.Register(TFTPBlockSize, ffval.NewValueDefault(&sc.Config.TFTP.BlockSize, sc.Config.TFTP.BlockSize))
	fs.Register(TFTPSinglePort, ffval.NewValueDefault(&sc.Config.TFTP.SinglePort, sc.Config.TFTP.SinglePort))
}

// Convert CLI specific fields to smee.Config fields.
func (s *SmeeConfig) Convert(trustedProxies *[]netip.Prefix, publicIP, publicIPv6 netip.Addr, bindAddr netip.Addr, defaultPort int) {
	s.Config.IPXE.HTTPScriptServer.TrustedProxies = ntip.ToPrefixList(trustedProxies).Slice()
	s.Config.DHCP.IPXEHTTPScript.URL.Host = s.advertisedHost(s.DHCPIPXEScript, publicIP, defaultPort)
	s.Config.DHCP.IPXEHTTPBinaryURL.Host = s.advertisedHost(s.DHCPIPXEBinary, publicIP, defaultPort)
	if publicIPv6.IsValid() && !publicIPv6.IsUnspecified() || s.DHCPv6IPXEScript.Host != "" || s.DHCPv6IPXEScript.Port != 0 {
		s.Config.DHCPv6.IPXEHTTPScript.URL.Host = s.advertisedHost(s.DHCPv6IPXEScript, publicIPv6, defaultPort)
	}
	if publicIPv6.IsValid() && !publicIPv6.IsUnspecified() || s.DHCPv6IPXEBinary.Host != "" || s.DHCPv6IPXEBinary.Port != 0 {
		s.Config.DHCPv6.IPXEHTTPBinaryURL.Host = s.advertisedHost(s.DHCPv6IPXEBinary, publicIPv6, defaultPort)
	}

	// publicIP is used to set IPForPacket, SyslogIP, TFTPIP, IPXEHTTPBinaryURL.Host, IPXEHTTPScript.URL.Host, and TinkServer.AddrPort.
	if publicIP.IsValid() && !publicIP.IsUnspecified() {
		// the order of precedence is: CLI flag, publicIP, default.
		if s.Config.DHCP.IPForPacket.IsUnspecified() || !s.Config.DHCP.IPForPacket.IsValid() {
			s.Config.DHCP.IPForPacket = publicIP
		}
		if s.Config.DHCP.SyslogIP.IsUnspecified() || !s.Config.DHCP.SyslogIP.IsValid() {
			s.Config.DHCP.SyslogIP = publicIP
		}
		if s.Config.DHCP.TFTPIP.IsUnspecified() || !s.Config.DHCP.TFTPIP.IsValid() {
			s.Config.DHCP.TFTPIP = publicIP
		}

		s.Config.TinkServer.AddrPort = func() string {
			_, port := splitHostPort(s.Config.TinkServer.AddrPort)
			if port == "" {
				port = fmt.Sprintf("%d", smee.DefaultTinkServerPort)
			}
			return joinHostPort(publicIP.String(), port)
		}()
	}

	// publicIPv6 is used to set v6 SyslogIP, TFTPIP, IPXEHTTPBinaryURL.Host, IPXEHTTPScript.URL.Host, and TinkServer.AddrPortV6.
	if publicIPv6.IsValid() && !publicIPv6.IsUnspecified() {
		if s.Config.DHCPv6.SyslogIP.IsUnspecified() || !s.Config.DHCPv6.SyslogIP.IsValid() {
			s.Config.DHCPv6.SyslogIP = publicIPv6
		}
		if s.Config.DHCPv6.TFTPIP.IsUnspecified() || !s.Config.DHCPv6.TFTPIP.IsValid() {
			s.Config.DHCPv6.TFTPIP = publicIPv6
		}

		s.Config.TinkServer.AddrPortV6 = func() string {
			_, port := splitHostPort(s.Config.TinkServer.AddrPortV6)
			if port == "" {
				port = fmt.Sprintf("%d", smee.DefaultTinkServerPort)
			}
			return joinHostPort(publicIPv6.String(), port)
		}()
	}

	// Set bind addresses if bindAddr is specified.
	if bindAddr.IsValid() {
		// syslog server
		s.Config.Syslog.BindAddr = bindAddr
		// TFTP server
		s.Config.TFTP.BindAddr = bindAddr
	}
}

func (s *SmeeConfig) advertisedHost(builder URLBuilder, publicIP netip.Addr, defaultPort int) string {
	return func() string {
		var addr string                        // Defaults
		port := fmt.Sprintf("%d", defaultPort) // Defaults
		if !publicIP.IsUnspecified() && publicIP.IsValid() {
			addr = publicIP.String()
		}
		// CLI flag
		if builder.Host != "" {
			addr = builder.Host
		}
		if builder.Port != 0 {
			port = fmt.Sprintf("%d", builder.Port)
		}

		if port != "" {
			return joinHostPort(addr, port)
		}
		return addr
	}()
}

func joinHostPort(host, port string) string {
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	return net.JoinHostPort(host, port)
}

func macAddrFormatParser(s string) (constant.MACFormat, error) {
	switch constant.MACFormat(s) {
	case constant.MacAddrFormatColon:
		return constant.MacAddrFormatColon, nil
	case constant.MacAddrFormatDot:
		return constant.MacAddrFormatDot, nil
	case constant.MacAddrFormatDash:
		return constant.MacAddrFormatDash, nil
	case constant.MacAddrFormatNoDelimiter:
		return constant.MacAddrFormatNoDelimiter, nil
	case "":
		return constant.MacAddrFormatColon, nil // constant.MacAddrFormatColon is the default
	default:
		return "", fmt.Errorf("invalid mac address format: %s, must be one of: [%s]", s, strings.Join([]string{constant.MacAddrFormatColon.String(), constant.MacAddrFormatDot.String(), constant.MacAddrFormatDash.String(), constant.MacAddrFormatNoDelimiter.String()}, ", "))
	}
}

// DHCP flags.
var DHCPEnabled = Config{
	Name:  "dhcp-enabled",
	Usage: "[dhcp] enable DHCP server",
}

var DHCPModeFlag = Config{
	Name:  "dhcp-mode",
	Usage: fmt.Sprintf("[dhcp] DHCP mode (%s, %s, %s)", smee.DHCPModeReservation, smee.DHCPModeProxy, smee.DHCPModeAutoProxy),
}

var DHCPv6Enabled = Config{
	Name:  "dhcpv6-enabled",
	Usage: "[dhcpv6] enable DHCPv6 server",
}

var DHCPv6EnableNetbootOptions = Config{
	Name:  "dhcpv6-enable-netboot-options",
	Usage: "[dhcpv6] enable sending netboot DHCPv6 options",
}

var DHCPv6ModeFlag = Config{
	Name:  "dhcpv6-mode",
	Usage: fmt.Sprintf("[dhcpv6] DHCPv6 mode (%s, %s, %s, %s)", smee.DHCPv6ModeStateless, smee.DHCPv6ModeAutoStateless, smee.DHCPv6ModeReservation, smee.DHCPv6ModeDerived),
}

var DHCPv6ServerDUID = Config{
	Name:  "dhcpv6-server-duid",
	Usage: "[dhcpv6] stable DHCPv6 server DUID as raw hex bytes; accepts colon, dash, or plain hex separators",
}

var DHCPv6DerivedDirectAddressPool = Config{
	Name:  "dhcpv6-derived-direct-address-pool",
	Usage: "[dhcpv6] usable IPv6 unicast CIDR, /1 through /64, used to derive addresses for direct DHCPv6 requests when Hardware has no IPv6 reservation",
}

var DHCPv6DerivedRelayAddressPrefix = Config{
	Name:  "dhcpv6-derived-relay-address-prefix",
	Usage: "[dhcpv6] relay link-address prefix length, 1-64, used to derive addresses for relayed DHCPv6 requests when Hardware has no IPv6 reservation",
}

var DHCPv6BindAddr = Config{
	Name:  "dhcpv6-bind-addr",
	Usage: "[dhcpv6] DHCPv6 server bind address",
}

var DHCPv6BindPort = Config{
	Name:  "dhcpv6-bind-port",
	Usage: "[dhcpv6] DHCPv6 server bind port",
}

var DHCPv6BindInterface = Config{
	Name:  "dhcpv6-bind-interface",
	Usage: "[dhcpv6] DHCPv6 server bind interface, or comma-separated interfaces",
}

var DHCPv6TftpIP = Config{
	Name:  "dhcpv6-tftp-ip",
	Usage: "[dhcpv6] TFTP server IP address to use in DHCPv6 boot file URLs",
}

var DHCPv6SyslogIP = Config{
	Name:  "dhcpv6-syslog-ip",
	Usage: "[dhcpv6] Syslog server IP address to use for iPXE scripts served to DHCPv6 clients",
}

var DHCPv6TftpPort = Config{
	Name:  "dhcpv6-tftp-port",
	Usage: "[dhcpv6] TFTP server port to use in DHCPv6 boot file URLs",
}

var DHCPv6IPXEHTTPBinaryURLScheme = Config{
	Name:  "dhcpv6-ipxe-http-binary-scheme",
	Usage: "[dhcpv6] HTTP iPXE binaries scheme to use in DHCPv6 packets",
}

var DHCPv6IPXEHTTPBinaryURLHost = Config{
	Name:  "dhcpv6-ipxe-http-binary-host",
	Usage: "[dhcpv6] HTTP iPXE binaries host or IP to use in DHCPv6 packets",
}

var DHCPv6IPXEHTTPBinaryURLPort = Config{
	Name:  "dhcpv6-ipxe-http-binary-port",
	Usage: "[dhcpv6] HTTP iPXE binaries port to use in DHCPv6 packets",
}

var DHCPv6IPXEHTTPBinaryURLPath = Config{
	Name:  "dhcpv6-ipxe-http-binary-path",
	Usage: "[dhcpv6] HTTP iPXE binaries path to use in DHCPv6 packets",
}

var DHCPv6IPXEHTTPScriptScheme = Config{
	Name:  "dhcpv6-ipxe-http-script-scheme",
	Usage: "[dhcpv6] HTTP iPXE script scheme to use in DHCPv6 packets",
}

var DHCPv6IPXEHTTPScriptHost = Config{
	Name:  "dhcpv6-ipxe-http-script-host",
	Usage: "[dhcpv6] HTTP iPXE script host or IP to use in DHCPv6 packets",
}

var DHCPv6IPXEHTTPScriptPort = Config{
	Name:  "dhcpv6-ipxe-http-script-port",
	Usage: "[dhcpv6] HTTP iPXE script port to use in DHCPv6 packets",
}

var DHCPv6IPXEHTTPScriptPath = Config{
	Name:  "dhcpv6-ipxe-http-script-path",
	Usage: "[dhcpv6] HTTP iPXE script path to use in DHCPv6 packets",
}

var DHCPv6IPXEHTTPScriptInjectMac = Config{
	Name:  "dhcpv6-ipxe-http-script-prepend-mac",
	Usage: "[dhcpv6] prepend the hardware MAC address to iPXE script URL base, http://1.2.3.4/auto6.ipxe -> http://1.2.3.4/40:15:ff:89:cc:0e/auto6.ipxe",
}

var DHCPBindAddr = Config{
	Name:  "dhcp-bind-addr",
	Usage: "[dhcp] DHCP server bind address",
}

var DHCPBindInterface = Config{
	Name:  "dhcp-bind-interface",
	Usage: "[dhcp] DHCP server bind interface",
}

var DHCPIPForPacket = Config{
	Name:  "dhcp-ip-for-packet",
	Usage: "[dhcp] DHCP server IP for packet",
}

var DHCPSyslogIP = Config{
	Name:  "dhcp-syslog-ip",
	Usage: "[dhcp] Syslog server IP address to use in DHCP packets (opt 7)",
}

var DHCPTftpIP = Config{
	Name:  "dhcp-tftp-ip",
	Usage: "[dhcp] TFTP server IP address to use in DHCP packets (opt 66, etc)",
}

var DHCPTftpPort = Config{
	Name:  "dhcp-tftp-port",
	Usage: "[dhcp] TFTP server port to use in DHCP packets (opt 66, etc)",
}

var DHCPIPXEHTTPBinaryURLScheme = Config{
	Name:  "dhcp-ipxe-http-binary-scheme",
	Usage: "[dhcp] HTTP iPXE binaries scheme to use in DHCP packets",
}

var DHCPIPXEHTTPBinaryURLHost = Config{
	Name:  "dhcp-ipxe-http-binary-host",
	Usage: "[dhcp] HTTP iPXE binaries host or IP to use in DHCP packets",
}

var DHCPIPXEHTTPBinaryURLPort = Config{
	Name:  "dhcp-ipxe-http-binary-port",
	Usage: "[dhcp] HTTP iPXE binaries port to use in DHCP packets",
}

var DHCPIPXEHTTPBinaryURLPath = Config{
	Name:  "dhcp-ipxe-http-binary-path",
	Usage: "[dhcp] HTTP iPXE binaries path to use in DHCP packets",
}

var DHCPIPXEHTTPScriptScheme = Config{
	Name:  "dhcp-ipxe-http-script-scheme",
	Usage: "[dhcp] HTTP iPXE script scheme to use in DHCP packets",
}

var DHCPIPXEHTTPScriptHost = Config{
	Name:  "dhcp-ipxe-http-script-host",
	Usage: "[dhcp] HTTP iPXE script host or IP to use in DHCP packets",
}

var DHCPIPXEHTTPScriptPort = Config{
	Name:  "dhcp-ipxe-http-script-port",
	Usage: "[dhcp] HTTP iPXE script port to use in DHCP packets",
}

var DHCPIPXEHTTPScriptPath = Config{
	Name:  "dhcp-ipxe-http-script-path",
	Usage: "[dhcp] HTTP iPXE script path to use in DHCP packets",
}

var DHCPIPXEHTTPScriptInjectMac = Config{
	Name:  "dhcp-ipxe-http-script-prepend-mac",
	Usage: "[dhcp] prepend the hardware MAC address to iPXE script URL base, http://1.2.3.4/auto.ipxe -> http://1.2.3.4/40:15:ff:89:cc:0e/auto.ipxe",
}

// iPXE HTTP script flags.
var IPXEHTTPScriptEnabled = Config{
	Name:  "ipxe-http-script-enabled",
	Usage: "[ipxe] enable iPXE HTTP script serving",
}

var IPXEHTTPScriptExtraKernelArgs = Config{
	Name:  "ipxe-http-script-extra-kernel-args",
	Usage: "[ipxe] extra set of kernel args (k=v k=v) that are appended to the kernel cmdline iPXE script",
}

var IPXEHTTPScriptKernelName = Config{
	Name:  "ipxe-http-script-kernel-name",
	Usage: "[ipxe] name of the kernel file to fetch in the iPXE script, defaults to vmlinuz, which becomes vmlinuz-<arch> in the script",
}

var IPXEHTTPScriptInitrdName = Config{
	Name:  "ipxe-http-script-initrd-name",
	Usage: "[ipxe] name of the initrd file to fetch in the iPXE script, defaults to initramfs, which becomes initramfs-<arch> in the script",
}

var IPXEHTTPScriptTrustedProxies = Config{
	Name:  "ipxe-http-script-trusted-proxies",
	Usage: "[ipxe] comma separated list of trusted proxies in CIDR notation",
}

var IPXEHTTPScriptOSIEURL = Config{
	Name:  "ipxe-http-script-osie-url",
	Usage: "[ipxe] URL where OSIE (HookOS) images are located",
}

var IPXEHTTPScriptOSIEURLv6 = Config{
	Name:  "ipxe-http-script-osie-url-v6",
	Usage: "[ipxe] URL where OSIE (HookOS) images are located for IPv6 clients",
}

var IPXEHTTPScriptRetries = Config{
	Name:  "ipxe-http-script-retries",
	Usage: "[ipxe] number of retries to attempt when fetching kernel and initrd files in the iPXE script",
}

var IPXEHTTPScriptRetryDelay = Config{
	Name:  "ipxe-http-script-retry-delay",
	Usage: "[ipxe] delay (in seconds) between retries when fetching kernel and initrd files in the iPXE script",
}

// iPXE HTTP binary flags.
var IPXEHTTPBinaryEnabled = Config{
	Name:  "ipxe-http-binary-enabled",
	Usage: "[ipxe] enable iPXE HTTP binary server",
}

var IPXEArchMapping = Config{
	Name:  "ipxe-override-arch-mapping",
	Usage: "[ipxe] override the iPXE architecture to binary mapping, see the iPXE Architecture Mapping documentation for detailed usage",
}

// TFTP flags.
var TFTPServerEnabled = Config{
	Name:  "tftp-server-enabled",
	Usage: "[tftp] enable iPXE TFTP binary server",
}

var TFTPServerBindAddr = Config{
	Name:  "tftp-server-bind-addr",
	Usage: "[tftp] local IP to listen on for iPXE binary TFTP requests",
}

var TFTPServerBindPort = Config{
	Name:  "tftp-server-bind-port",
	Usage: "[tftp] local port to listen on for iPXE binary TFTP requests",
}

var TFTPTimeout = Config{
	Name:  "tftp-timeout",
	Usage: "[tftp] timeout (in seconds) for TFTP requests",
}

var TFTPBlockSize = Config{
	Name:  "tftp-block-size",
	Usage: "[tftp] TFTP block size a value between 512 (the default block size for TFTP) and 65456 (the max size a UDP packet payload can be)",
}

var TFTPSinglePort = Config{
	Name:  "tftp-single-port",
	Usage: "[tftp] Use a single port for TFTP transfers",
}

// iPXE flags.
var IPXEEmbeddedScriptPatch = Config{
	Name:  "ipxe-embedded-script-patch",
	Usage: "[ipxe] iPXE script fragment to patch into served iPXE binaries served via TFTP or HTTP",
}

var IPXEBinaryInjectMacAddrFormat = Config{
	Name:  "ipxe-binary-inject-mac-addr-format",
	Usage: fmt.Sprintf("[ipxe] format to use when injecting the mac address into the iPXE binary URL. one of: [%s, %s, %s, %s, %s]", constant.MacAddrFormatColon.String(), constant.MacAddrFormatDot.String(), constant.MacAddrFormatDash.String(), constant.MacAddrFormatNoDelimiter.String(), constant.MacAddrFormatEmpty.String()),
}

// Syslog flags.
var SyslogEnabled = Config{
	Name:  "syslog-enabled",
	Usage: "[syslog] enable Syslog server(receiver)",
}

var SyslogBindAddr = Config{
	Name:  "syslog-bind-addr",
	Usage: "[syslog] local IP to listen on for Syslog messages",
}

var SyslogBindPort = Config{
	Name:  "syslog-bind-port",
	Usage: "[syslog] local port to listen on for Syslog messages",
}

// ISO flags.
var ISOEnabled = Config{
	Name:  "iso-enabled",
	Usage: "[iso] enable OSIE ISO patching service",
}

var ISOUpstreamURL = Config{
	Name:  "iso-upstream-url",
	Usage: "[iso] an ISO source (upstream) URL target for patching kernel command line parameters",
}

var ISOPatchMagicString = Config{
	Name:  "iso-patch-magic-string",
	Usage: "[iso] the string pattern to match for in the source (upstream) ISO, defaults to the one defined in HookOS",
}

var ISOStaticIPAMEnabled = Config{
	Name:  "iso-static-ipam-enabled",
	Usage: "[iso] enable static IPAM when patching the source (upstream) ISO",
}

// Tink Server flags.
var TinkServerAddrPort = Config{
	Name:  "ipxe-script-tink-server-addr-port",
	Usage: "[ipxe] Tink server address and port",
}

var TinkServerAddrPortV6 = Config{
	Name:  "ipxe-script-tink-server-addr-port-v6",
	Usage: "[ipxe] Tink server IPv6 address and port",
}

var TinkServerUseTLS = Config{
	Name:  "ipxe-script-tink-server-use-tls",
	Usage: "[ipxe] Use TLS to connect to the Tink server",
}

var TinkServerInsecureTLS = Config{
	Name:  "ipxe-script-tink-server-insecure-tls",
	Usage: "[ipxe] Skip TLS verification when connecting to the Tink server",
}

var SmeeLogLevel = Config{
	Name:  "smee-log-level",
	Usage: "the higher the number the more verbose, level 0 inherits the global log level, a negative number disables logging",
}

var DHCPEnableNetbootOptions = Config{
	Name:  "dhcp-enable-netboot-options",
	Usage: "[dhcp] enable sending netboot DHCP options",
}
