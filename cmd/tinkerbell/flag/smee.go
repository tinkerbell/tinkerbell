package flag

import (
	"fmt"
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
	LogLevel       int
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

	// HTTPS flags
	fs.Register(HTTPSBindPort, ffval.NewValueDefault(&sc.Config.HTTP.BindHTTPSPort, sc.Config.HTTP.BindHTTPSPort))

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
	fs.Register(IPXEHTTPScriptBindAddr, &ntip.Addr{Addr: &sc.Config.IPXE.HTTPScriptServer.BindAddr})
	fs.Register(IPXEHTTPScriptBindPort, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.BindPort, sc.Config.IPXE.HTTPScriptServer.BindPort))
	fs.Register(IPXEHTTPScriptExtraKernelArgs, ffval.NewList(&sc.Config.IPXE.HTTPScriptServer.ExtraKernelArgs))
	fs.Register(IPXEHTTPScriptTrustedProxies, ffval.NewList(&sc.Config.IPXE.HTTPScriptServer.TrustedProxies))
	fs.Register(IPXEHTTPScriptRetries, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.Retries, sc.Config.IPXE.HTTPScriptServer.Retries))
	fs.Register(IPXEHTTPScriptRetryDelay, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.RetryDelay, sc.Config.IPXE.HTTPScriptServer.RetryDelay))
	fs.Register(IPXEHTTPScriptOSIEURL, &url.URL{URL: sc.Config.IPXE.HTTPScriptServer.OSIEURL})
	fs.Register(IPXEScriptSyslogFQDN, ffval.NewValueDefault(&sc.Config.IPXE.HTTPScriptServer.SyslogFQDN, sc.Config.IPXE.HTTPScriptServer.SyslogFQDN))
	fs.Register(IPXEBinaryInjectMacAddrFormat, &ffval.Enum[constant.MACFormat]{
		ParseFunc: macAddrFormatParser,
		Valid:     []constant.MACFormat{constant.MacAddrFormatColon, constant.MacAddrFormatDot, constant.MacAddrFormatDash, constant.MacAddrFormatNoDelimiter},
		Pointer:   &sc.Config.IPXE.IPXEBinary.InjectMacAddrFormat,
		Default:   constant.MacAddrFormatColon,
	})

	// iPXE Tink Server Flags
	fs.Register(TinkServerAddrPort, ffval.NewValueDefault(&sc.Config.TinkServer.AddrPort, sc.Config.TinkServer.AddrPort))
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
func (s *SmeeConfig) Convert(trustedProxies *[]netip.Prefix, publicIP netip.Addr, bindAddr netip.Addr) {
	s.Config.IPXE.HTTPScriptServer.TrustedProxies = ntip.ToPrefixList(trustedProxies).Slice()
	s.Config.DHCP.IPXEHTTPScript.URL.Host = func() string {
		var addr string                                 // Defaults
		port := fmt.Sprintf("%d", smee.DefaultHTTPPort) // Defaults
		if !publicIP.IsUnspecified() && publicIP.IsValid() {
			addr = publicIP.String()
		}
		// CLI flag
		if s.DHCPIPXEScript.Host != "" {
			addr = s.DHCPIPXEScript.Host
		}
		if s.DHCPIPXEScript.Port != 0 {
			port = fmt.Sprintf("%d", s.DHCPIPXEScript.Port)
		}

		if port != "" {
			return fmt.Sprintf("%s:%s", addr, port)
		}
		return addr
	}()

	s.Config.DHCP.IPXEHTTPBinaryURL.Host = func() string {
		var addr string                                 // Defaults
		port := fmt.Sprintf("%d", smee.DefaultHTTPPort) // Defaults
		if !publicIP.IsUnspecified() && publicIP.IsValid() {
			addr = publicIP.String()
		}
		// CLI flag
		if s.DHCPIPXEBinary.Host != "" {
			addr = s.DHCPIPXEBinary.Host
		}
		if s.DHCPIPXEBinary.Port != 0 {
			port = fmt.Sprintf("%d", s.DHCPIPXEBinary.Port)
		}

		if port != "" {
			return fmt.Sprintf("%s:%s", addr, port)
		}
		return addr
	}()

	// publicIP is used to set IPForPacket, SyslogIP, TFTPIP, IPXEHTTPBinaryURL.Host, IPXEHTTPScript.URL.Host, and TinkServer.AddrPort.
	if publicIP.IsUnspecified() || !publicIP.IsValid() {
		return
	}
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
		host, port := splitHostPort(s.Config.TinkServer.AddrPort)
		if port == "" {
			port = fmt.Sprintf("%d", smee.DefaultTinkServerPort)
		}
		// Only use publicIP as fallback when host is empty/unset
		if host == "" && publicIP.IsValid() && !publicIP.IsUnspecified() {
			host = publicIP.String()
		}
		return fmt.Sprintf("%s:%s", host, port)
	}()

	// Set bind addresses if bindAddr is specified.
	if bindAddr.IsValid() {
		// iPXE HTTP Script Server
		s.Config.IPXE.HTTPScriptServer.BindAddr = bindAddr
		// syslog server
		s.Config.Syslog.BindAddr = bindAddr
		// TFTP server
		s.Config.TFTP.BindAddr = bindAddr
	}
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

var IPXEHTTPScriptBindAddr = Config{
	Name:  "ipxe-http-script-bind-addr",
	Usage: "[ipxe] local IP to listen on for iPXE HTTP script requests",
}

var IPXEHTTPScriptBindPort = Config{
	Name:  "ipxe-http-script-bind-port",
	Usage: "[ipxe] local port to listen on for iPXE HTTP script requests",
}

var IPXEHTTPScriptExtraKernelArgs = Config{
	Name:  "ipxe-http-script-extra-kernel-args",
	Usage: "[ipxe] extra set of kernel args (k=v k=v) that are appended to the kernel cmdline iPXE script",
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
	Usage: "[ipxe] syslog server hostname/FQDN for iPXE scripts (if empty, falls back to --dhcp-syslog-ip)",
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

var HTTPSBindPort = Config{
	Name:  "https-bind-port",
	Usage: "[https] local port to listen on for HTTPS requests",
}
