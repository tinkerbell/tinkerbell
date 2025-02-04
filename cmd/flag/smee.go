package flag

import (
	"fmt"
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/backend/kube"
	ntip "github.com/tinkerbell/tinkerbell/cmd/flag/netip"
	"github.com/tinkerbell/tinkerbell/cmd/flag/url"
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
	// DHCP flags
	fs.Register(DHCPEnabled, ffval.NewValueDefault(&sc.Config.DHCP.Enabled, sc.Config.DHCP.Enabled))
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

	// IPXE flags
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

	// ISO Flags
	fs.Register(ISOEnabled, ffval.NewValueDefault(&sc.Config.ISO.Enabled, sc.Config.ISO.Enabled))
	fs.Register(ISOUpstreamURL, &url.URL{URL: sc.Config.ISO.UpstreamURL})
	fs.Register(ISOPatchMagicString, ffval.NewValueDefault(&sc.Config.ISO.PatchMagicString, sc.Config.ISO.PatchMagicString))
	fs.Register(ISOStaticIPAMEnabled, ffval.NewValueDefault(&sc.Config.ISO.StaticIPAMEnabled, sc.Config.ISO.StaticIPAMEnabled))

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

	// Tink Server Flags
	fs.Register(TinkServerAddrPort, ffval.NewValueDefault(&sc.Config.TinkServer.AddrPort, sc.Config.TinkServer.AddrPort))
	fs.Register(TinkServerUseTLS, ffval.NewValueDefault(&sc.Config.TinkServer.UseTLS, sc.Config.TinkServer.UseTLS))
	fs.Register(TinkServerInsecureTLS, ffval.NewValueDefault(&sc.Config.TinkServer.InsecureTLS, sc.Config.TinkServer.InsecureTLS))
}

// Convert CLI specific fields to smee.Config fields.
func (s *SmeeConfig) Convert(trustedProxies *[]netip.Prefix) {
	s.Config.IPXE.HTTPScriptServer.TrustedProxies = ntip.ToPrefixList(trustedProxies).Slice()
	s.Config.DHCP.IPXEHTTPScript.URL.Host = func() string {
		addr, port := splitHostPort(s.Config.DHCP.IPXEHTTPScript.URL.Host)
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
		addr, port := splitHostPort(s.Config.DHCP.IPXEHTTPBinaryURL.Host)
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

// iPXE HTTP binary flags.
var IPXEHTTPBinaryEnabled = Config{
	Name:  "ipxe-http-binary-enabled",
	Usage: "[ipxe] enable iPXE HTTP binary server",
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

// iPXE flags.
var IPXEEmbeddedScriptPatch = Config{
	Name:  "ipxe-embedded-script-patch",
	Usage: "[ipxe] iPXE script fragment to patch into served iPXE binaries served via TFTP or HTTP",
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
	Usage: "[tink] Tink server address and port",
}

var TinkServerUseTLS = Config{
	Name:  "ipxe-script-tink-server-use-tls",
	Usage: "[tink] Use TLS to connect to the Tink server",
}

var TinkServerInsecureTLS = Config{
	Name:  "ipxe-script-tink-server-insecure-tls",
	Usage: "[tink] Skip TLS verification when connecting to the Tink server",
}
