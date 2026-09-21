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
	"github.com/tinkerbell/tinkerbell/pkg/flag/delimitedlist"
	ntip "github.com/tinkerbell/tinkerbell/pkg/flag/netip"
	"github.com/tinkerbell/tinkerbell/pkg/flag/url"
	"github.com/tinkerbell/tinkerbell/smee"
	"k8s.io/apimachinery/pkg/util/validation"
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
	fs.RegisterFamily(DHCPEnabled, V4, ffval.NewValueDefault(&sc.Config.DHCP.Enabled, sc.Config.DHCP.Enabled))
	fs.RegisterFamily(DHCPEnabled, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.Enabled, sc.Config.DHCPv6.Enabled))
	fs.RegisterFamily(DHCPEnableNetbootOptions, V4, ffval.NewValueDefault(&sc.Config.DHCP.EnableNetbootOptions, sc.Config.DHCP.EnableNetbootOptions))
	fs.RegisterFamily(DHCPEnableNetbootOptions, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.EnableNetbootOptions, sc.Config.DHCPv6.EnableNetbootOptions))
	fs.RegisterFamily(DHCPModeFlag, V4, &sc.Config.DHCP.Mode)
	fs.RegisterFamily(DHCPModeFlag, V6, &sc.Config.DHCPv6.Mode)
	fs.RegisterFamily(DHCPDefaultNameServers, V4, delimitedlist.NewParsed(&sc.Config.DHCP.DefaultNameServers, ',', parseDefaultIPv4NameServer))
	fs.RegisterFamily(DHCPDefaultNameServers, V6, delimitedlist.NewParsed(&sc.Config.DHCPv6.DefaultNameServers, ',', parseDefaultIPv6NameServer))
	fs.RegisterFamily(DHCPDefaultDomainSearchList, V4, delimitedlist.NewParsed(&sc.Config.DHCP.DefaultDomainSearchList, ',', parseDomainSearchSuffix))
	fs.RegisterFamily(DHCPDefaultDomainSearchList, V6, delimitedlist.NewParsed(&sc.Config.DHCPv6.DefaultDomainSearchList, ',', parseDomainSearchSuffix))
	fs.RegisterFamily(DHCPServerDUID, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.ServerDUID, sc.Config.DHCPv6.ServerDUID))
	fs.RegisterFamily(DHCPDerivedDirectAddressPool, V6, &ntip.Prefix{Prefix: &sc.Config.DHCPv6.DerivedDirectAddressPool})
	fs.RegisterFamily(DHCPDerivedRelayAddressPrefix, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.DerivedRelayAddressPrefix, sc.Config.DHCPv6.DerivedRelayAddressPrefix))
	fs.RegisterFamily(DHCPBindAddr, V4, &ntip.Addr{Addr: &sc.Config.DHCP.BindAddr})
	fs.RegisterFamily(DHCPBindAddr, V6, &ntip.Addr{Addr: &sc.Config.DHCPv6.BindAddr})
	fs.RegisterFamily(DHCPBindPort, V4, ffval.NewValueDefault(&sc.Config.DHCP.BindPort, sc.Config.DHCP.BindPort))
	fs.RegisterFamily(DHCPBindPort, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.BindPort, sc.Config.DHCPv6.BindPort))
	fs.RegisterFamily(DHCPBindInterface, V4, ffval.NewValueDefault(&sc.Config.DHCP.BindInterface, sc.Config.DHCP.BindInterface))
	fs.RegisterFamily(DHCPBindInterface, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.BindInterface, sc.Config.DHCPv6.BindInterface))
	fs.RegisterFamily(DHCPIPForPacket, V4, &ntip.Addr{Addr: &sc.Config.DHCP.IPForPacket})
	fs.RegisterFamily(DHCPSyslogIP, V4, &ntip.Addr{Addr: &sc.Config.DHCP.SyslogIP})
	fs.RegisterFamily(DHCPSyslogIP, V6, &ntip.Addr{Addr: &sc.Config.DHCPv6.SyslogIP})
	fs.RegisterFamily(DHCPTftpIP, V4, &ntip.Addr{Addr: &sc.Config.DHCP.TFTPIP})
	fs.RegisterFamily(DHCPTftpIP, V6, &ntip.Addr{Addr: &sc.Config.DHCPv6.TFTPIP})
	fs.RegisterFamily(DHCPTftpPort, V4, ffval.NewValueDefault(&sc.Config.DHCP.TFTPPort, sc.Config.DHCP.TFTPPort))
	fs.RegisterFamily(DHCPTftpPort, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.TFTPPort, sc.Config.DHCPv6.TFTPPort))
	fs.RegisterFamily(DHCPIPXEHTTPScriptInjectMac, V4, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPScript.InjectMacAddress, sc.Config.DHCP.IPXEHTTPScript.InjectMacAddress))
	fs.RegisterFamily(DHCPIPXEHTTPScriptInjectMac, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPScript.InjectMacAddress, sc.Config.DHCPv6.IPXEHTTPScript.InjectMacAddress))
	fs.RegisterFamily(DHCPIPXEHTTPBinaryURLScheme, V4, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPBinaryURL.Scheme, sc.Config.DHCP.IPXEHTTPBinaryURL.Scheme))
	fs.RegisterFamily(DHCPIPXEHTTPBinaryURLScheme, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPBinaryURL.Scheme, sc.Config.DHCPv6.IPXEHTTPBinaryURL.Scheme))
	fs.RegisterFamily(DHCPIPXEHTTPBinaryURLHost, V4, ffval.NewValueDefault(&sc.DHCPIPXEBinary.Host, sc.DHCPIPXEBinary.Host))
	fs.RegisterFamily(DHCPIPXEHTTPBinaryURLHost, V6, ffval.NewValueDefault(&sc.DHCPv6IPXEBinary.Host, sc.DHCPv6IPXEBinary.Host))
	fs.RegisterFamily(DHCPIPXEHTTPBinaryURLPort, V4, ffval.NewValueDefault(&sc.DHCPIPXEBinary.Port, sc.DHCPIPXEBinary.Port))
	fs.RegisterFamily(DHCPIPXEHTTPBinaryURLPort, V6, ffval.NewValueDefault(&sc.DHCPv6IPXEBinary.Port, sc.DHCPv6IPXEBinary.Port))
	fs.RegisterFamily(DHCPIPXEHTTPBinaryURLPath, V4, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPBinaryURL.Path, sc.Config.DHCP.IPXEHTTPBinaryURL.Path))
	fs.RegisterFamily(DHCPIPXEHTTPBinaryURLPath, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPBinaryURL.Path, sc.Config.DHCPv6.IPXEHTTPBinaryURL.Path))
	fs.RegisterFamily(DHCPIPXEHTTPScriptScheme, V4, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPScript.URL.Scheme, sc.Config.DHCP.IPXEHTTPScript.URL.Scheme))
	fs.RegisterFamily(DHCPIPXEHTTPScriptScheme, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPScript.URL.Scheme, sc.Config.DHCPv6.IPXEHTTPScript.URL.Scheme))
	fs.RegisterFamily(DHCPIPXEHTTPScriptHost, V4, ffval.NewValueDefault(&sc.DHCPIPXEScript.Host, sc.DHCPIPXEScript.Host))
	fs.RegisterFamily(DHCPIPXEHTTPScriptHost, V6, ffval.NewValueDefault(&sc.DHCPv6IPXEScript.Host, sc.DHCPv6IPXEScript.Host))
	fs.RegisterFamily(DHCPIPXEHTTPScriptPort, V4, ffval.NewValueDefault(&sc.DHCPIPXEScript.Port, sc.DHCPIPXEScript.Port))
	fs.RegisterFamily(DHCPIPXEHTTPScriptPort, V6, ffval.NewValueDefault(&sc.DHCPv6IPXEScript.Port, sc.DHCPv6IPXEScript.Port))
	fs.RegisterFamily(DHCPIPXEHTTPScriptPath, V4, ffval.NewValueDefault(&sc.Config.DHCP.IPXEHTTPScript.URL.Path, sc.Config.DHCP.IPXEHTTPScript.URL.Path))
	fs.RegisterFamily(DHCPIPXEHTTPScriptPath, V6, ffval.NewValueDefault(&sc.Config.DHCPv6.IPXEHTTPScript.URL.Path, sc.Config.DHCPv6.IPXEHTTPScript.URL.Path))

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
	fs.RegisterFamily(IPXEHTTPScriptExtraKernelArgs, V4, ffval.NewList(&sc.Config.IPXE.HTTPScriptServer.ExtraKernelArgs))
	fs.RegisterFamily(IPXEHTTPScriptExtraKernelArgs, V6, ffval.NewList(&sc.Config.IPXE.HTTPScriptServer.ExtraKernelArgsV6))
	fs.Register(IPXEHTTPScriptKernelName, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.KernelName, sc.Config.IPXE.HTTPScriptServer.KernelName))
	fs.Register(IPXEHTTPScriptInitrdName, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.InitrdName, sc.Config.IPXE.HTTPScriptServer.InitrdName))
	fs.Register(IPXEHTTPScriptTrustedProxies, ffval.NewList(&sc.Config.IPXE.HTTPScriptServer.TrustedProxies))
	fs.Register(IPXEHTTPScriptRetries, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.Retries, sc.Config.IPXE.HTTPScriptServer.Retries))
	fs.Register(IPXEHTTPScriptRetryDelay, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.RetryDelay, sc.Config.IPXE.HTTPScriptServer.RetryDelay))
	fs.RegisterFamily(IPXEHTTPScriptOSIEURL, V4, &url.URL{URL: sc.Config.IPXE.HTTPScriptServer.OSIEURL})
	fs.RegisterFamily(IPXEHTTPScriptOSIEURL, V6, &url.URL{URL: sc.Config.IPXE.HTTPScriptServer.OSIEURLv6})
	fs.RegisterFamily(IPXEScriptSyslogFQDN, V4, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.SyslogFQDN, sc.Config.IPXE.HTTPScriptServer.SyslogFQDN))
	fs.RegisterFamily(IPXEScriptSyslogFQDN, V6, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.SyslogFQDNV6, sc.Config.IPXE.HTTPScriptServer.SyslogFQDNV6))
	fs.Register(IPXEBinaryInjectMacAddrFormat, &ffval.Enum[constant.MACFormat]{
		ParseFunc: macAddrFormatParser,
		Valid:     []constant.MACFormat{constant.MacAddrFormatColon, constant.MacAddrFormatDot, constant.MacAddrFormatDash, constant.MacAddrFormatNoDelimiter},
		Pointer:   &sc.Config.IPXE.IPXEBinary.InjectMacAddrFormat,
		Default:   constant.MacAddrFormatColon,
	})

	// iPXE Tink Server Flags
	fs.RegisterFamily(TinkServerAddrPort, V4, ffval.NewValueDefault(&sc.Config.TinkServer.AddrPort, sc.Config.TinkServer.AddrPort))
	fs.RegisterFamily(TinkServerAddrPort, V6, ffval.NewValueDefault(&sc.Config.TinkServer.AddrPortV6, sc.Config.TinkServer.AddrPortV6))
	fs.Register(TinkServerUseTLS, ffval.NewValueDefault(&sc.Config.TinkServer.UseTLS, sc.Config.TinkServer.UseTLS))
	fs.Register(TinkServerInsecureTLS, ffval.NewValueDefault(&sc.Config.TinkServer.InsecureTLS, sc.Config.TinkServer.InsecureTLS))

	// ISO Flags
	fs.Register(ISOEnabled, ffval.NewValueDefault(&sc.Config.ISO.Enabled, sc.Config.ISO.Enabled))
	fs.Register(ISOUpstreamURL, &url.URL{URL: sc.Config.ISO.UpstreamURL})
	fs.Register(ISOPatchMagicString, ffval.NewValueDefault(&sc.Config.ISO.PatchMagicString, sc.Config.ISO.PatchMagicString))
	fs.RegisterFamily(ISOStaticIPAMEnabled, V4, ffval.NewValueDefault(&sc.Config.ISO.StaticIPAMEnabled, sc.Config.ISO.StaticIPAMEnabled))

	// Log level
	fs.Register(SmeeLogLevel, ffval.NewValueDefault(&sc.LogLevel, sc.LogLevel))

	// Syslog Flags
	fs.Register(SyslogEnabled, ffval.NewValueDefault(&sc.Config.Syslog.Enabled, sc.Config.Syslog.Enabled))
	fs.RegisterFamily(SyslogBindAddr, V4, &ntip.Addr{Addr: &sc.Config.Syslog.V4.Addr})
	fs.RegisterFamily(SyslogBindAddr, V6, &ntip.Addr{Addr: &sc.Config.Syslog.V6.Addr})
	fs.RegisterFamily(SyslogBindPort, V4, ffval.NewValueDefault(&sc.Config.Syslog.V4.Port, sc.Config.Syslog.V4.Port))
	fs.RegisterFamily(SyslogBindPort, V6, ffval.NewValueDefault(&sc.Config.Syslog.V6.Port, sc.Config.Syslog.V6.Port))

	// TFTP Flags
	fs.Register(TFTPServerEnabled, ffval.NewValueDefault(&sc.Config.TFTP.Enabled, sc.Config.TFTP.Enabled))
	fs.RegisterFamily(TFTPServerBindAddr, V4, &ntip.Addr{Addr: &sc.Config.TFTP.V4.Addr})
	fs.RegisterFamily(TFTPServerBindAddr, V6, &ntip.Addr{Addr: &sc.Config.TFTP.V6.Addr})
	fs.RegisterFamily(TFTPServerBindPort, V4, ffval.NewValueDefault(&sc.Config.TFTP.V4.Port, sc.Config.TFTP.V4.Port))
	fs.RegisterFamily(TFTPServerBindPort, V6, ffval.NewValueDefault(&sc.Config.TFTP.V6.Port, sc.Config.TFTP.V6.Port))
	fs.Register(TFTPTimeout, ffval.NewValueDefault(&sc.Config.TFTP.Timeout, sc.Config.TFTP.Timeout))
	fs.Register(TFTPBlockSize, ffval.NewValueDefault(&sc.Config.TFTP.BlockSize, sc.Config.TFTP.BlockSize))
	fs.Register(TFTPSinglePort, ffval.NewValueDefault(&sc.Config.TFTP.SinglePort, sc.Config.TFTP.SinglePort))
	fs.Register(TFTPAssetDir, ffval.NewValueDefault(&sc.Config.TFTP.AssetDir, sc.Config.TFTP.AssetDir))

	// PXE-over-HTTP flags
	fs.Register(PXEHTTPEnabled, ffval.NewValueDefault(&sc.Config.PXEHTTP.Enabled, sc.Config.PXEHTTP.Enabled))
	fs.Register(PXEHTTPPathPrefix, ffval.NewValueDefault(&sc.Config.PXEHTTP.PathPrefix, sc.Config.PXEHTTP.PathPrefix))
}

// Convert CLI specific fields to smee.Config fields.
func (s *SmeeConfig) Convert(trustedProxies *[]netip.Prefix, publicIP, publicIPv6 netip.Addr, bindAddr netip.Addr, defaultPort int) {
	s.Config.IPXE.HTTPScriptServer.TrustedProxies = ntip.ToPrefixList(trustedProxies).Slice()
	s.Config.DHCP.IPXEHTTPScript.URL.Host = s.advertisedHost(s.DHCPIPXEScript, publicIP, defaultPort)
	s.Config.DHCP.IPXEHTTPBinaryURL.Host = s.advertisedHost(s.DHCPIPXEBinary, publicIP, defaultPort)
	hasPublicIPv6 := publicIPv6.IsValid() && !publicIPv6.IsUnspecified()
	if hasPublicIPv6 || s.DHCPv6IPXEScript.Host != "" || s.DHCPv6IPXEScript.Port != 0 {
		s.Config.DHCPv6.IPXEHTTPScript.URL.Host = s.advertisedHost(s.DHCPv6IPXEScript, publicIPv6, defaultPort)
	}
	if hasPublicIPv6 || s.DHCPv6IPXEBinary.Host != "" || s.DHCPv6IPXEBinary.Port != 0 {
		s.Config.DHCPv6.IPXEHTTPBinaryURL.Host = s.advertisedHost(s.DHCPv6IPXEBinary, publicIPv6, defaultPort)
	}

	// Service-specific bind addresses take precedence over the global bind address.
	if bindAddr.IsValid() {
		if !s.Config.Syslog.V4.Addr.IsValid() {
			s.Config.Syslog.V4.Addr = bindAddr
		}
		if !s.Config.TFTP.V4.Addr.IsValid() {
			s.Config.TFTP.V4.Addr = bindAddr
		}
	}

	// Preserve explicit Tink Server hosts, falling back to the corresponding public
	// address only when the host portion was not configured.
	s.Config.TinkServer.AddrPort = advertisedAddrPort(s.Config.TinkServer.AddrPort, publicIP)
	s.Config.TinkServer.AddrPortV6 = advertisedAddrPort(s.Config.TinkServer.AddrPortV6, publicIPv6)

	// publicIP is used to set IPForPacket, SyslogIP, TFTPIP, IPXEHTTPBinaryURL.Host, and IPXEHTTPScript.URL.Host.
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
	}

	// publicIPv6 is used to set v6 SyslogIP, TFTPIP, IPXEHTTPBinaryURL.Host, and IPXEHTTPScript.URL.Host.
	if hasPublicIPv6 {
		if s.Config.DHCPv6.SyslogIP.IsUnspecified() || !s.Config.DHCPv6.SyslogIP.IsValid() {
			s.Config.DHCPv6.SyslogIP = publicIPv6
		}
		if s.Config.DHCPv6.TFTPIP.IsUnspecified() || !s.Config.DHCPv6.TFTPIP.IsValid() {
			s.Config.DHCPv6.TFTPIP = publicIPv6
		}
	}
}

func (s *SmeeConfig) advertisedHost(builder URLBuilder, publicIP netip.Addr, defaultPort int) string {
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

	return joinHostPort(addr, port)
}

func advertisedAddrPort(addrPort string, publicIP netip.Addr) string {
	host, port := splitHostPort(addrPort)
	if port == "" {
		port = fmt.Sprintf("%d", smee.DefaultTinkServerPort)
	}
	if host == "" && publicIP.IsValid() && !publicIP.IsUnspecified() {
		host = publicIP.String()
	}
	if host == "" {
		return ""
	}
	return joinHostPort(host, port)
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

func parseDefaultIPv4NameServer(value string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid default DNS server address %q: %w", value, err)
	}
	if !addr.Is4() {
		return netip.Addr{}, fmt.Errorf("invalid default DNS server address %q: must be an IPv4 address", value)
	}
	return addr, nil
}

func parseDefaultIPv6NameServer(value string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid default DNS server address %q: %w", value, err)
	}
	if !addr.Is6() || addr.Is4In6() {
		return netip.Addr{}, fmt.Errorf("invalid default DNS server address %q: must be an IPv6 address", value)
	}
	return addr, nil
}

func parseDomainSearchSuffix(value string) (string, error) {
	normalized := strings.ToLower(value)
	for _, label := range strings.Split(normalized, ".") {
		if len(label) > validation.DNS1123LabelMaxLength {
			return "", fmt.Errorf("invalid DNS search domain %q: label %q must be no more than %d bytes", value, label, validation.DNS1123LabelMaxLength)
		}
	}
	if problems := validation.IsDNS1123Subdomain(normalized); len(problems) > 0 {
		return "", fmt.Errorf("invalid DNS search domain %q: %s", value, strings.Join(problems, "; "))
	}
	return value, nil
}

// DHCP flags. Each is registered once per address family by RegisterSmeeFlags,
// so the usage text here must not name a family.
var DHCPEnabled = Config{
	Name:  "dhcp-enabled",
	Usage: "[dhcp] enable DHCP server",
}

var DHCPModeFlag = Config{
	Name:  "dhcp-mode",
	Usage: fmt.Sprintf("[dhcp] DHCP mode, one of [%s, %s, %s] for IPv4 or [%s, %s, %s, %s] for IPv6", smee.DHCPModeReservation, smee.DHCPModeProxy, smee.DHCPModeAutoProxy, smee.DHCPv6ModeStateless, smee.DHCPv6ModeAutoStateless, smee.DHCPv6ModeReservation, smee.DHCPv6ModeDerived),
}

var DHCPDefaultNameServers = Config{
	Name:  "dhcp-default-name-servers",
	Usage: "[dhcp] comma-separated default DNS server addresses used when Hardware has no nameservers",
}

var DHCPDefaultDomainSearchList = Config{
	Name:  "dhcp-default-domain-search-list",
	Usage: "[dhcp] comma-separated default domain search list used when Hardware has none",
}

// DHCPServerDUID has no IPv4 counterpart: DHCPv6 identifies the server by DUID,
// not by address.
var DHCPServerDUID = Config{
	Name:  "dhcp-server-duid",
	Usage: "[dhcp] stable DHCPv6 server DUID as raw hex bytes; accepts colon, dash, or plain hex separators",
}

// DHCPDerivedDirectAddressPool has no IPv4 counterpart: derived addressing is
// DHCPv6-only.
var DHCPDerivedDirectAddressPool = Config{
	Name:  "dhcp-derived-direct-address-pool",
	Usage: "[dhcp] usable IPv6 unicast CIDR, /1 through /64, used to derive addresses for direct DHCPv6 requests when Hardware has no IPv6 reservation",
}

// DHCPDerivedRelayAddressPrefix has no IPv4 counterpart: derived addressing is
// DHCPv6-only.
var DHCPDerivedRelayAddressPrefix = Config{
	Name:  "dhcp-derived-relay-address-prefix",
	Usage: "[dhcp] relay link-address prefix length, 1-64, used to derive addresses for relayed DHCPv6 requests when Hardware has no IPv6 reservation",
}

var DHCPBindAddr = Config{
	Name:  "dhcp-bind-addr",
	Usage: "[dhcp] DHCP server bind address",
}

var DHCPBindPort = Config{
	Name:  "dhcp-bind-port",
	Usage: "[dhcp] DHCP server bind port",
}

var DHCPBindInterface = Config{
	Name:  "dhcp-bind-interface",
	Usage: "[dhcp] DHCP server bind interface, or comma-separated interfaces",
}

// DHCPIPForPacket has no IPv6 counterpart: it is the DHCPv4 option 54 server
// identifier, and DHCPv6 identifies the server by DUID.
var DHCPIPForPacket = Config{
	Name:  "dhcp-ip-for-packet",
	Usage: "[dhcp] DHCP server IP for packet (opt 54)",
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

var IPXEHTTPScriptRetries = Config{
	Name:  "ipxe-http-script-retries",
	Usage: "[ipxe] number of retries to attempt when fetching kernel and initrd files in the iPXE script",
}

var IPXEHTTPScriptRetryDelay = Config{
	Name:  "ipxe-http-script-retry-delay",
	Usage: "[ipxe] delay (in seconds) between retries when fetching kernel and initrd files in the iPXE script",
}

var IPXEScriptSyslogFQDN = Config{
	Name:  "ipxe-script-syslog-fqdn",
	Usage: "[ipxe] syslog server hostname/FQDN or address, resolved by iPXE at boot (if empty, falls back to the matching --dhcp-syslog-ip)",
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

var TFTPAssetDir = Config{
	Name:  "tftp-asset-dir",
	Usage: "[tftp] Directory to serve extra TFTP assets from (disabled if empty)",
}

// PXE-over-HTTP flags.
var PXEHTTPEnabled = Config{
	Name:  "pxe-http-enabled",
	Usage: "[pxe-http] enable serving pxelinux.cfg and the TFTP asset dir over HTTP (for u-boot pxe-over-http)",
}

var PXEHTTPPathPrefix = Config{
	Name:  "pxe-http-path-prefix",
	Usage: "[pxe-http] URL path prefix to serve pxelinux.cfg and TFTP assets under over HTTP",
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

// ISOStaticIPAMEnabled has no IPv6 counterpart: static IPAM in patched ISOs is
// IPv4-only. IPv6 machines use SLAAC or DHCPv6.
var ISOStaticIPAMEnabled = Config{
	Name:  "iso-static-ipam-enabled",
	Usage: "[iso] enable static IPAM when patching the source (upstream) ISO",
}

// Tink Server flags.
var TinkServerAddrPort = Config{
	Name:  "ipxe-script-tink-server-addr-port",
	Usage: "[ipxe] Tink server address and port",
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
	Usage: logLevelUsage,
}

var DHCPEnableNetbootOptions = Config{
	Name:  "dhcp-enable-netboot-options",
	Usage: "[dhcp] enable sending netboot DHCP options",
}
