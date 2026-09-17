package dhcpv6

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp"
)

// NetbootConfig contains the inputs required to configure DHCPv6 netboot.
type NetbootConfig struct {
	// IPXEBinServerTFTP is the required TFTP server address advertised in boot URLs.
	// It must be a usable non-loopback IPv6 unicast address, must not be IPv4-mapped,
	// and must have a nonzero port.
	IPXEBinServerTFTP netip.AddrPort
	// IPXEBinServerHTTP is the required HTTP or HTTPS base URL for iPXE binaries.
	// It must include a host; any explicit port must be between 1 and 65535.
	// Literal IP hosts have the same address restrictions as IPXEBinServerTFTP.
	IPXEBinServerHTTP *url.URL
	// IPXEScriptURL is the required default iPXE script URL. It has the same URL
	// restrictions as IPXEBinServerHTTP. Hardware may override it for allowed netboot.
	IPXEScriptURL *url.URL
	// InjectMacAddress inserts the colon-delimited client MAC address before the
	// default script filename. Disable it for script URLs that do not use this layout.
	InjectMacAddress bool
	// InjectMacAddrFormat controls MAC formatting in binary URLs: "colon", "dot",
	// "dash", "no-delimiter", or "empty". The zero value uses colon delimiters.
	InjectMacAddrFormat constant.MACFormat
	// IPXEArchMapping overrides the default architecture-to-binary mapping.
	// NewNetboot copies the map; nil uses the default mapping.
	IPXEArchMapping map[iana.Arch]constant.IPXEBinary
}

// Netboot provides validated DHCPv6 netboot behavior.
// Use NewNetboot to construct one.
type Netboot struct {
	ipxeBinServerTFTP   netip.AddrPort
	ipxeBinServerHTTP   url.URL
	ipxeScriptURL       url.URL
	injectMacAddress    bool
	injectMacAddrFormat constant.MACFormat
	ipxeArchMapping     map[iana.Arch]constant.IPXEBinary
}

// DisabledNetboot omits DHCPv6 netboot options.
type DisabledNetboot struct{}

// NewNetboot validates and constructs an enabled DHCPv6 netboot configuration.
func NewNetboot(config NetbootConfig) (*Netboot, error) {
	if !config.IPXEBinServerTFTP.IsValid() || config.IPXEBinServerTFTP.Port() == 0 {
		return nil, errors.New("invalid TFTP server address or port")
	}
	if addr := config.IPXEBinServerTFTP.Addr(); !isUsableNetbootServerAddress(addr) {
		return nil, fmt.Errorf("invalid TFTP server address %s: must be a usable non-loopback IPv6 unicast address and not IPv4-mapped", addr)
	}
	httpBinaryURL, err := validateHTTPURL("iPXE binary", config.IPXEBinServerHTTP)
	if err != nil {
		return nil, err
	}
	httpScriptURL, err := validateHTTPURL("iPXE script", config.IPXEScriptURL)
	if err != nil {
		return nil, err
	}

	return &Netboot{
		ipxeBinServerTFTP:   config.IPXEBinServerTFTP,
		ipxeBinServerHTTP:   httpBinaryURL,
		ipxeScriptURL:       httpScriptURL,
		injectMacAddress:    config.InjectMacAddress,
		injectMacAddrFormat: config.InjectMacAddrFormat,
		ipxeArchMapping:     maps.Clone(config.IPXEArchMapping),
	}, nil
}

func validateHTTPURL(name string, configured *url.URL) (url.URL, error) {
	if configured == nil {
		return url.URL{}, fmt.Errorf("%s URL is required", name)
	}

	parsed, err := url.Parse(configured.String())
	if err != nil {
		return url.URL{}, fmt.Errorf("invalid %s URL: %w", name, err)
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return url.URL{}, fmt.Errorf("%s URL must use HTTP or HTTPS", name)
	}
	if parsed.Hostname() == "" {
		return url.URL{}, fmt.Errorf("%s URL requires a host", name)
	}
	if port := parsed.Port(); port != "" {
		parsedPort, err := strconv.ParseUint(port, 10, 16)
		if err != nil || parsedPort == 0 {
			return url.URL{}, fmt.Errorf("%s URL port %q must be between 1 and 65535", name, port)
		}
	}
	if addr, err := netip.ParseAddr(parsed.Hostname()); err == nil && !isUsableNetbootServerAddress(addr) {
		return url.URL{}, fmt.Errorf("%s URL host %s must be a usable non-loopback IPv6 unicast address and not IPv4-mapped", name, addr)
	}

	return *parsed, nil
}

func isUsableNetbootServerAddress(addr netip.Addr) bool {
	return addr.Is6() &&
		!addr.Is4In6() &&
		!addr.IsUnspecified() &&
		!addr.IsLoopback() &&
		!addr.IsMulticast()
}

// InfoOptions returns the request parsing options associated with the netboot configuration.
func (n *Netboot) InfoOptions() []InfoOption {
	return []InfoOption{
		WithMacAddrFormat(n.injectMacAddrFormat),
		WithArchMappingOverride(maps.Clone(n.ipxeArchMapping)),
	}
}

// BootURL returns a boot URL using the validated global netboot configuration.
func (n *Netboot) BootURL(info Info, hardware *dhcp.Netboot, traceparent string) (string, error) {
	if hardware != nil && !hardware.AllowNetboot {
		return n.notAllowedURL(info.Mac), nil
	}

	if info.hasUserClass(dhcp.Tinkerbell) || info.hasUserClass(dhcp.IPXE) {
		if hardware != nil && hardware.IPXEScriptURL != nil {
			return hardware.IPXEScriptURL.String(), nil
		}
		return n.scriptURL(info.Mac).String(), nil
	}

	binary := info.IPXEBinary
	if hardware != nil && hardware.IPXEBinary != "" {
		binary = hardware.IPXEBinary
	}
	if binary == "" {
		return "", fmt.Errorf("%w: missing iPXE binary", ErrNoBootURL)
	}
	if traceparent != "" {
		binary = fmt.Sprintf("%s-%v", binary, traceparent)
	}

	if info.IsHTTPBootClient() {
		return httpBootURL(&n.ipxeBinServerHTTP, info.Mac, info.MacAddrFormat, binary), nil
	}

	bootTFTPURL := tftpBootURL(n.ipxeBinServerTFTP, info.Mac, info.MacAddrFormat, binary)
	return strings.Replace(bootTFTPURL, "]:69/", "]/", 1), nil
}

// InfoOptions returns no request parsing options when netboot is disabled.
func (DisabledNetboot) InfoOptions() []InfoOption {
	return nil
}

// BootURL omits the boot URL when netboot is disabled.
func (DisabledNetboot) BootURL(Info, *dhcp.Netboot, string) (string, error) {
	return "", nil
}

func httpBootURL(ipxeHTTPBinServer *url.URL, mac net.HardwareAddr, format constant.MACFormat, binary string) string {
	if ipxeHTTPBinServer == nil || binary == "" {
		return ""
	}

	paths := []string{binary}
	if mac != nil {
		paths = append([]string{dhcp.FormatMACAddr(mac, format)}, paths...)
	}

	return ipxeHTTPBinServer.JoinPath(paths...).String()
}

func tftpBootURL(ipxeTFTPBinServer netip.AddrPort, mac net.HardwareAddr, format constant.MACFormat, binary string) string {
	if !ipxeTFTPBinServer.IsValid() || binary == "" {
		return ""
	}

	t := url.URL{
		Scheme: "tftp",
		Host:   ipxeTFTPBinServer.String(),
	}
	paths := []string{binary}
	if mac != nil {
		paths = append([]string{dhcp.FormatMACAddr(mac, format)}, paths...)
	}

	return t.JoinPath(paths...).String()
}

func (n *Netboot) scriptURL(mac net.HardwareAddr) *url.URL {
	scriptURL := n.ipxeScriptURL
	if !n.injectMacAddress || mac == nil {
		return &scriptURL
	}

	filename := path.Base(scriptURL.Path)
	scriptURL.Path = path.Join(path.Dir(scriptURL.Path), mac.String(), filename)
	return &scriptURL
}

func (n *Netboot) notAllowedURL(mac net.HardwareAddr) string {
	notAllowed := *n.scriptURL(mac)
	if idx := strings.LastIndex(notAllowed.Path, "/"); idx >= 0 {
		notAllowed.Path = notAllowed.Path[:idx+1] + "netboot-not-allowed"
	} else {
		notAllowed.Path = "netboot-not-allowed"
	}
	notAllowed.RawPath = ""
	notAllowed.RawQuery = ""
	notAllowed.Fragment = ""
	return notAllowed.String()
}
