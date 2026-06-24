package dhcpv6

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp"
	"go.opentelemetry.io/otel/attribute"
)

const (
	UnknownArch = iana.Arch(255)

	// ClientEnterpriseNumber identifies Tinkerbell vendor-class responses.
	ClientEnterpriseNumber = 343 // Uses Intel number to work with IPv6 EDK II UEFI
)

var unusableDerivedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("::ffff:0:0/96"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// ErrNoBootURL reports that no usable DHCPv6 boot URL could be determined.
var ErrNoBootURL = errors.New("no boot URL available")

// VendorClass identifies a DHCPv6 vendor class value advertised by a client.
type VendorClass string

// String returns the vendor class value.
func (c VendorClass) String() string {
	return string(c)
}

// Info holds details about the dhcp request. Use NewInfo to populate the struct fields from a dhcp packet.
type Info struct {
	// Pkt is the dhcpv6 packet that was received from the client.
	Pkt dhcpv6.DHCPv6
	// Msg is the inner DHCPv6 message after relay decapsulation.
	Msg *dhcpv6.Message
	// Relay is the outer relay-forward message when the request arrived through a relay.
	Relay *dhcpv6.RelayMessage
	// Peer is the remote address that sent the DHCPv6 packet.
	Peer net.Addr
	// Arch is the architecture of the client. Use NewInfo to automatically populate this field.
	Arch iana.Arch
	// Mac is the mac address of the client. Use NewInfo to automatically populate this field.
	Mac net.HardwareAddr
	// UserClasses is the user class of the client. Use NewInfo to automatically populate this field.
	UserClasses []dhcp.UserClass
	// VendorClasses are the vendor class options from the client request.
	VendorClasses []*dhcpv6.OptVendorClass
	// IPXEBinary is the iPXE binary file to boot. Use NewInfo to automatically populate this field.
	IPXEBinary string
	// MacAddrFormat is the format to use when injecting the MAC address into the iPXE binary URL.
	MacAddrFormat constant.MACFormat
	// ArchMappingOverride allows customization for mapping architectures to iPXE binaries.
	// This is used to override the default ArchToBootFile mapping.
	ArchMappingOverride map[iana.Arch]constant.IPXEBinary
	// AllowedMessageTypes are DHCPv6 message types accepted by NewInfo.
	AllowedMessageTypes map[dhcpv6.MessageType]struct{}

	log logr.Logger
}

type InfoOption func(*Info)

func WithMacAddrFormat(format constant.MACFormat) InfoOption {
	return func(i *Info) {
		i.MacAddrFormat = format
	}
}

func WithArchMappingOverride(mapping map[iana.Arch]constant.IPXEBinary) InfoOption {
	return func(i *Info) {
		i.ArchMappingOverride = mapping
	}
}

func WithAllowedMessageTypes(messageTypes ...dhcpv6.MessageType) InfoOption {
	return func(i *Info) {
		i.AllowedMessageTypes = make(map[dhcpv6.MessageType]struct{}, len(messageTypes))
		for _, messageType := range messageTypes {
			i.AllowedMessageTypes[messageType] = struct{}{}
		}
	}
}

// WithLogger configures the logger used while extracting DHCPv6 packet metadata.
func WithLogger(log logr.Logger) InfoOption {
	return func(i *Info) {
		i.log = log
	}
}

func NewInfo(peer net.Addr, pkt dhcpv6.DHCPv6, opts ...InfoOption) (Info, error) {
	i := Info{
		Pkt:  pkt,
		Peer: peer,
		log:  logr.Discard(),
	}

	if pkt == nil {
		return i, errors.New("dhcpv6 packet is nil")
	}

	for _, opt := range opts {
		opt(&i)
	}

	// By default allow Information-Request
	if len(i.AllowedMessageTypes) == 0 {
		i.AllowedMessageTypes = map[dhcpv6.MessageType]struct{}{
			dhcpv6.MessageTypeInformationRequest: {},
		}
	}

	var err error
	i.Msg, i.Relay, err = parseDHCPv6Request(pkt, i.AllowedMessageTypes)
	if err != nil {
		return i, err
	}

	i.Mac, err = macFromPacket(pkt, peer, i.log)
	if err != nil {
		return i, err
	}

	i.UserClasses = i.UserClassesFrom()
	i.VendorClasses = i.Msg.Options.VendorClasses()

	i.Arch = dhcp.FirstKnownArch(i.Mac, i.Msg.Options.ArchTypes())
	if i.Arch == UnknownArch {
		// Fallback to VendorClass Arch Option 16
		// https://datatracker.ietf.org/doc/html/rfc9915#section-21.16
		for _, v := range i.VendorClasses {
			arch, ok := archFromDHCPv6VendorClass(v.ToBytes())
			if !ok {
				continue // next
			}
			i.Arch = archFromVendorClass(arch)
			if i.Arch != UnknownArch {
				break // found
			}
		}
	}

	if i.IPXEBinary == "" {
		i.IPXEBinary = dhcp.IPXEBinaryForArch(i.Arch, i.ArchMappingOverride)
	}

	return i, nil
}

// UserClassesFrom returns the DHCPv6 user-class options as Smee user classes.
func (i Info) UserClassesFrom() []dhcp.UserClass {
	userClasses := make([]dhcp.UserClass, 0, len(i.Msg.Options.UserClasses()))
	for _, userClass := range i.Msg.Options.UserClasses() {
		userClasses = append(userClasses, dhcp.UserClass(userClass))
	}
	return userClasses
}

// IsBootfileURLOptionRequested reports whether the client requested the bootfile-url option.
func (i Info) IsBootfileURLOptionRequested() bool {
	return i.Msg.IsOptionRequested(dhcpv6.OptionBootfileURL)
}

func (i *Info) hasUserClass(want dhcp.UserClass) bool {
	for _, userClass := range i.UserClasses {
		if string(userClass) == want.String() {
			return true
		}
	}
	return false
}

// HasHTTPClientVendorClass reports whether the client advertises the HTTPClient vendor class.
func (i *Info) HasHTTPClientVendorClass() bool {
	for _, vendorClass := range i.VendorClasses {
		for _, data := range vendorClass.Data {
			if strings.Contains(string(data), dhcp.HTTPClient.String()) {
				return true
			}
		}
	}
	return false
}

// IsHTTPBootClient reports whether the client should receive HTTP boot options.
func (i *Info) IsHTTPBootClient() bool {
	return IsHTTPBootArch(i.Arch) || i.HasHTTPClientVendorClass()
}

// IsHTTPBootArch reports whether arch is one of the IANA HTTP boot architecture values.
func IsHTTPBootArch(arch iana.Arch) bool {
	switch arch {
	case iana.EFI_X86_HTTP,
		iana.EFI_X86_64_HTTP,
		iana.EFI_BC_HTTP,
		iana.EFI_ARM32_HTTP,
		iana.EFI_ARM64_HTTP,
		iana.INTEL_X86PC_HTTP,
		iana.UBOOT_ARM32_HTTP,
		iana.UBOOT_ARM64_HTTP,
		iana.EFI_RISCV32_HTTP,
		iana.EFI_RISCV64_HTTP,
		iana.EFI_RISCV128_HTTP:
		return true
	default:
		return false
	}
}

// BootURLConfig holds the inputs needed to build a DHCPv6 boot file URL.
type BootURLConfig struct {
	HardwareNetboot   *dhcp.Netboot
	IPXEBinServerTFTP netip.AddrPort
	IPXEBinServerHTTP *url.URL
	IPXEScriptURL     func(net.HardwareAddr) *url.URL
	Traceparent       string
}

// BootURL returns the DHCPv6 boot file URL for the request.
func (i *Info) BootURL(config BootURLConfig) (string, error) {
	// Netboot is not allowed
	if config.HardwareNetboot != nil && !config.HardwareNetboot.AllowNetboot {
		return i.notAllowedBootURL(config.IPXEScriptURL)
	}

	// Break the iPXE boot loop with iPXE script
	if i.hasUserClass(dhcp.Tinkerbell) || i.hasUserClass(dhcp.IPXE) {
		if config.HardwareNetboot != nil && config.HardwareNetboot.IPXEScriptURL != nil {
			return config.HardwareNetboot.IPXEScriptURL.String(), nil
		}

		if config.IPXEScriptURL == nil {
			return "", fmt.Errorf("%w: missing iPXE script URL builder", ErrNoBootURL)
		}

		scriptURL := config.IPXEScriptURL(i.Mac)
		if scriptURL == nil {
			return "", fmt.Errorf("%w: iPXE script URL builder returned nil", ErrNoBootURL)
		}

		return scriptURL.String(), nil
	}

	binary := i.IPXEBinary
	if config.HardwareNetboot != nil && config.HardwareNetboot.IPXEBinary != "" {
		binary = config.HardwareNetboot.IPXEBinary
	}

	if binary == "" {
		return "", fmt.Errorf("%w: missing iPXE binary", ErrNoBootURL)
	}

	if config.Traceparent != "" {
		binary = fmt.Sprintf("%s-%v", binary, config.Traceparent)
	}

	if i.IsHTTPBootClient() {
		bootURL := dhcp.HTTPBootURL(config.IPXEBinServerHTTP, i.Mac, i.MacAddrFormat, binary)
		if bootURL == "" {
			return "", fmt.Errorf("%w: missing HTTP iPXE binary server", ErrNoBootURL)
		}
		return bootURL, nil
	}

	// The default is to return TFTP link
	bootTFTPUrl := dhcp.TFTPBootURL(config.IPXEBinServerTFTP, i.Mac, i.MacAddrFormat, binary)
	if bootTFTPUrl == "" {
		return "", fmt.Errorf("%w: missing TFTP iPXE binary server", ErrNoBootURL)
	}
	// Based upon RFC 5970 and UEFI 2.6, bootfile-url format can be
	// tftp://[SERVER_ADDRESS]/BOOTFILE_NAME or tftp://domain_name/BOOTFILE_NAME
	// As an example where the BOOTFILE_NAME is the EFI loader and
	// SERVER_ADDRESS is the ASCII encoding of an IPV6 address.
	//
	// If the port is default, removing it
	return strings.Replace(bootTFTPUrl, "]:69/", "]/", 1), nil
}

func (i *Info) notAllowedBootURL(smeeIPXEScriptURL func(net.HardwareAddr) *url.URL) (string, error) {
	if smeeIPXEScriptURL == nil {
		return "", fmt.Errorf("%w: missing iPXE script URL builder for not-allowed URL", ErrNoBootURL)
	}

	u := smeeIPXEScriptURL(i.Mac)
	if u == nil {
		return "", fmt.Errorf("%w: iPXE script URL builder returned nil for not-allowed URL", ErrNoBootURL)
	}

	notAllowed := *u
	path := notAllowed.Path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[:idx+1] + "netboot-not-allowed"
	} else {
		path = "netboot-not-allowed"
	}
	notAllowed.Path = path
	notAllowed.RawPath = ""
	notAllowed.RawQuery = ""
	notAllowed.Fragment = ""
	return notAllowed.String(), nil
}

// ApplyBootOptions adds DHCPv6 netboot reply options for a resolved boot URL.
func ApplyBootOptions(reply dhcpv6.DHCPv6, i Info, bootURL string) {
	if bootURL == "" {
		return
	}

	// Set Option OPT_BOOTFILE_URL (59)
	dhcpv6.WithOption(dhcpv6.OptBootFileURL(bootURL))(reply)

	// If the client used this option in the request, the server SHOULD
	// include this option to inform the client about the pre-boot
	// environments that are supported by the boot file. The list MUST only
	// contain architecture types that have initially been queried by the
	// client. The items MUST also be listed in order of descending
	// priority.
	// https://datatracker.ietf.org/doc/html/rfc5970#section-3.3
	// Option OPTION_CLIENT_ARCH_TYPE (61)
	if ClientArchSelected(i.Msg.Options.ArchTypes(), i.Arch) {
		dhcpv6.WithOption(dhcpv6.OptClientArchType(i.Arch))(reply)
	}

	// HTTP boot clients expect the server to identify itself as HTTPClient.
	// Do not identify TFTP/PXE boot replies as PXEClient: EDK II treats that
	// as a BINL boot-service offer and probes UDP/4011 before TFTP.
	if i.IsHTTPBootClient() {
		dhcpv6.WithOption(VendorClassOption(dhcp.HTTPClient))(reply)
	}
}

// ApplyRequestedStatelessOptions adds requested DNS, domain-search, and NTP options.
func ApplyRequestedStatelessOptions(reply dhcpv6.DHCPv6, i Info, d *dhcp.DHCP) {
	if d == nil {
		return
	}

	if dnsServers := IPv6Only(d.NameServers); i.Msg.IsOptionRequested(dhcpv6.OptionDNSRecursiveNameServer) && len(dnsServers) > 0 {
		dhcpv6.WithDNS(dnsServers...)(reply)
	}
	if i.Msg.IsOptionRequested(dhcpv6.OptionDomainSearchList) && len(d.DomainSearch) > 0 {
		dhcpv6.WithDomainSearchList(d.DomainSearch...)(reply)
	}
	if ntpServers := IPv6Only(d.NTPServers); i.Msg.IsOptionRequested(dhcpv6.OptionNTPServer) && len(ntpServers) > 0 {
		reply.AddOption(NTPServerOption(ntpServers))
	}
}

// WriteReply sends reply directly, or encapsulated as a relay reply when needed.
func WriteReply(conn net.PacketConn, peer net.Addr, relay *dhcpv6.RelayMessage, reply *dhcpv6.Message) (dhcpv6.DHCPv6, error) {
	response := dhcpv6.DHCPv6(reply)
	if relay != nil {
		var err error
		response, err = dhcpv6.NewRelayReplFromRelayForw(relay, reply)
		if err != nil {
			return nil, err
		}
	}

	_, err := conn.WriteTo(response.ToBytes(), peer)
	return response, err
}

// EncodeToAttributes takes a DHCPv6 packet and returns opentelemetry key/value attributes.
func EncodeToAttributes(d dhcpv6.DHCPv6, namespace string, includeIAAddr bool) []attribute.KeyValue {
	if d == nil {
		return nil
	}

	attrs := []attribute.KeyValue{
		attribute.String(fmt.Sprintf("DHCP.%s.Header.messageType", namespace), d.Type().String()),
		attribute.Bool(fmt.Sprintf("DHCP.%s.Header.isRelay", namespace), d.IsRelay()),
	}

	if xid, err := dhcpv6.GetTransactionID(d); err == nil {
		attrs = append(attrs, attribute.String(fmt.Sprintf("DHCP.%s.Header.transactionID", namespace), xid.String()))
	}

	msg, err := d.GetInnerMessage()
	if err != nil || msg == nil {
		return attrs
	}
	if msg.Type() != d.Type() {
		attrs = append(attrs, attribute.String(fmt.Sprintf("DHCP.%s.Header.innerMessageType", namespace), msg.Type().String()))
	}
	if includeIAAddr {
		if ia := msg.Options.OneIANA(); ia != nil {
			if addr := ia.Options.OneAddress(); addr != nil {
				attrs = append(attrs, attribute.String(fmt.Sprintf("DHCP.%s.Opt5.IAAddr", namespace), addr.IPv6Addr.String()))
			}
		}
	}
	if bootURL := msg.Options.BootFileURL(); bootURL != "" {
		attrs = append(attrs, attribute.String(fmt.Sprintf("DHCP.%s.Opt59.BootFileURL", namespace), bootURL))
	}
	if dnsServers := IPStrings(msg.Options.DNS()); len(dnsServers) > 0 {
		attrs = append(attrs, attribute.StringSlice(fmt.Sprintf("DHCP.%s.Opt23.NameServers", namespace), dnsServers))
	}
	if domains := msg.Options.DomainSearchList(); domains != nil && len(domains.Labels) > 0 {
		attrs = append(attrs, attribute.String(fmt.Sprintf("DHCP.%s.Opt24.DomainSearch", namespace), strings.Join(domains.Labels, ",")))
	}
	if ntpServers := IPStrings(msg.Options.NTPServers()); len(ntpServers) > 0 {
		attrs = append(attrs, attribute.StringSlice(fmt.Sprintf("DHCP.%s.Opt56.NTPServers", namespace), ntpServers))
	}

	return attrs
}

// VendorClassOption returns a DHCPv6 vendor-class option for a client type.
func VendorClassOption(ct dhcp.ClientType) *dhcpv6.OptVendorClass {
	return &dhcpv6.OptVendorClass{
		EnterpriseNumber: ClientEnterpriseNumber,
		Data:             [][]byte{[]byte(ct)},
	}
}

// ClientArchSelected reports whether selected was present in the client request.
func ClientArchSelected(archs iana.Archs, selected iana.Arch) bool {
	return slices.Contains(archs, selected)
}

// IPv6Only returns only IPv6 addresses, excluding IPv4 and invalid values.
func IPv6Only(servers []net.IP) []net.IP {
	var result []net.IP
	for _, server := range servers {
		if server == nil || server.To4() != nil || server.To16() == nil {
			continue
		}
		result = append(result, server)
	}
	return result
}

// IPStrings converts non-nil IP addresses to strings.
func IPStrings(servers []net.IP) []string {
	result := make([]string, 0, len(servers))
	for _, server := range servers {
		if server == nil {
			continue
		}
		result = append(result, server.String())
	}
	return result
}

// NTPServerOption builds a DHCPv6 NTP server option from server addresses.
func NTPServerOption(servers []net.IP) *dhcpv6.OptNTPServer {
	opt := &dhcpv6.OptNTPServer{}
	for _, server := range servers {
		addr := dhcpv6.NTPSuboptionSrvAddr(server)
		opt.Suboptions.Add(&addr)
	}
	return opt
}

// HardwareNotFound reports whether err satisfies the backend not-found contract.
func HardwareNotFound(err error) bool {
	type notFound interface {
		NotFound() bool
	}
	var nfe notFound
	return errors.As(err, &nfe) && nfe.NotFound()
}

func parseDHCPv6Request(packet dhcpv6.DHCPv6, allowed map[dhcpv6.MessageType]struct{}) (*dhcpv6.Message, *dhcpv6.RelayMessage, error) {
	switch pkt := packet.(type) {
	case *dhcpv6.Message:
		if _, ok := allowed[pkt.Type()]; !ok {
			return nil, nil, errors.New("unsupported message type")
		}
		return pkt, nil, nil
	case *dhcpv6.RelayMessage:
		if pkt.Type() != dhcpv6.MessageTypeRelayForward {
			return nil, nil, errors.New("unsupported relay type")
		}
		inner, err := pkt.GetInnerMessage()
		if err != nil {
			return nil, nil, err
		}
		if _, ok := allowed[inner.Type()]; !ok {
			return nil, nil, errors.New("unsupported relay inner message type")
		}
		return inner, pkt, nil
	default:
		return nil, nil, errors.New("unsupported packet type")
	}
}

func macFromPacket(packet dhcpv6.DHCPv6, peer net.Addr, log logr.Logger) (net.HardwareAddr, error) {
	if xid, err := dhcpv6.GetTransactionID(packet); err == nil {
		log = log.WithValues("xid", xid.String())
	} else {
		log.V(1).Info("DHCPv6 MAC extraction could not determine transaction ID", "error", err)
	}

	mac, err := extractMAC(packet, log)
	if mac != nil {
		return mac, nil
	}

	if packet.IsRelay() {
		log.V(1).Info("DHCPv6 MAC extraction failed: no reliable relay client identity", "error", err)
		return nil, err
	}

	udpPeer, ok := peer.(*net.UDPAddr)
	if !ok || udpPeer.IP == nil {
		log.V(1).Info("DHCPv6 MAC extraction peer fallback unavailable", "peer", peer, "error", err)
		return nil, errors.New("DHCPv6 MAC extraction peer fallback unavailable")
	}
	if !udpPeer.IP.IsLinkLocalUnicast() {
		log.V(1).Info("DHCPv6 MAC extraction peer fallback requires link-local address", "peerIP", udpPeer.IP.String(), "error", err)
		return nil, errors.New("DHCPv6 MAC extraction peer fallback requires link-local address")
	}

	log.V(1).Info("DHCPv6 MAC extraction falling back to peer EUI-64 address", "peerIP", udpPeer.IP.String(), "error", err)
	mac, err = dhcpv6.GetMacAddressFromEUI64(udpPeer.IP)
	if err != nil {
		log.V(1).Info("DHCPv6 MAC extraction failed from peer EUI-64 address", "peerIP", udpPeer.IP.String(), "error", err)
		return nil, err
	}
	if mac != nil {
		log.V(1).Info("DHCPv6 MAC extracted from peer EUI-64 address", "mac", mac.String(), "peerIP", udpPeer.IP.String())
		return mac, nil
	}

	log.V(1).Info("DHCPv6 MAC extraction failed: no reliable client identity", "peerIP", udpPeer.IP.String())
	return nil, errors.New("no reliable client identity")
}

// ExtractMAC looks into the inner most PeerAddr field in the RelayInfo header
// which contains the EUI-64 address of the client making the request, populated
// by the dhcp relay, it is possible to extract the mac address from that IP.
// If that fails, it looks for the MAC addressed embededded in the DUID.
// Note that this only works with type DuidLL and DuidLLT.
// If a mac address cannot be found an error will be returned.
//
// This is a copy of github.com/insomniacslk/dhcp/dhcpv6.ExtractMAC but with additional
// logging for debug.
func extractMAC(packet dhcpv6.DHCPv6, log logr.Logger) (net.HardwareAddr, error) {
	msg := packet
	if packet.IsRelay() {
		log.V(1).Info("DHCPv6 MAC extraction inspecting relay packet")
		inner, err := dhcpv6.DecapsulateRelayIndex(packet, -1)
		if err != nil {
			log.V(1).Info("DHCPv6 MAC extraction failed to decapsulate relay packet", "error", err)
			return nil, err
		}
		relay, ok := inner.(*dhcpv6.RelayMessage)
		if !ok {
			err := fmt.Errorf("expected relay message, got %T", inner)
			log.V(1).Info("DHCPv6 MAC extraction failed to inspect relay packet", "error", err)
			return nil, err
		}
		if _, mac := relay.Options.ClientLinkLayerAddress(); mac != nil {
			log.V(1).Info("DHCPv6 MAC extracted from relay client link-layer address option (79)", "mac", mac.String())
			return mac, nil
		}
		log.V(1).Info("DHCPv6 relay client link-layer address option (79) not present")
		if mac, err := dhcpv6.GetMacAddressFromEUI64(relay.PeerAddr); err == nil {
			log.V(1).Info("DHCPv6 MAC extracted from relay peer EUI-64 address", "mac", mac.String(), "peerAddr", relay.PeerAddr.String())
			return mac, nil
		}
		log.V(1).Info("DHCPv6 MAC extraction failed from relay peer EUI-64 address", "peerAddr", relay.PeerAddr.String(), "error", err)

		msg, err = relay.GetInnerMessage()
		if err != nil {
			log.V(1).Info("DHCPv6 MAC extraction failed to read relay inner message", "error", err)
			return nil, err
		}
	}
	message, ok := msg.(*dhcpv6.Message)
	if !ok {
		err := fmt.Errorf("expected DHCPv6 message, got %T", msg)
		log.V(1).Info("DHCPv6 MAC extraction failed to inspect inner message", "error", err)
		return nil, err
	}
	duid := message.Options.ClientID()
	if duid == nil {
		log.V(1).Info("DHCPv6 MAC extraction failed: client ID not found")
		return nil, fmt.Errorf("client ID not found in packet")
	}
	switch d := duid.(type) {
	case *dhcpv6.DUIDLL:
		if d.LinkLayerAddr != nil {
			log.V(1).Info("DHCPv6 MAC extracted from DUID-LL client ID", "mac", d.LinkLayerAddr.String())
			return d.LinkLayerAddr, nil
		}
		log.V(1).Info("DHCPv6 DUID-LL client ID has no link-layer address")
	case *dhcpv6.DUIDLLT:
		if d.LinkLayerAddr != nil {
			log.V(1).Info("DHCPv6 MAC extracted from DUID-LLT client ID", "mac", d.LinkLayerAddr.String())
			return d.LinkLayerAddr, nil
		}
		log.V(1).Info("DHCPv6 DUID-LLT client ID has no link-layer address")
	default:
		log.V(1).Info("DHCPv6 client ID type does not contain a MAC address", "duidType", fmt.Sprintf("%T", duid))
	}
	log.V(1).Info("DHCPv6 MAC extraction failed")
	return nil, fmt.Errorf("failed to extract MAC")
}

// archFromDHCPv6VendorClass extracts the PXE architecture code from DHCPv6
// Vendor Class data.
//
// Expected vendor class example:
//
//	PXEClient:Arch:00007:UNDI:003016
//
// Returns:
//   - arch code, e.g. 7
//   - true if an arch value was found and parsed
func archFromDHCPv6VendorClass(vendorClassData []byte) (int, bool) {
	s := string(vendorClassData)

	const marker = "Arch:"
	idx := strings.Index(s, marker)
	if idx == -1 {
		return 0, false
	}

	start := idx + len(marker)
	end := strings.IndexByte(s[start:], ':')
	if end == -1 {
		end = len(s)
	} else {
		end = start + end
	}

	archStr := strings.TrimSpace(s[start:end])
	if archStr == "" {
		return 0, false
	}

	arch, err := strconv.Atoi(archStr)
	if err != nil {
		return 0, false
	}

	return arch, true
}

var dhcpv6VendorClassArchToIANA = map[int]iana.Arch{
	0:  iana.INTEL_X86PC,
	2:  iana.EFI_ITANIUM,
	6:  iana.EFI_IA32,
	7:  iana.EFI_X86_64,
	9:  iana.EFI_BC,
	10: iana.EFI_ARM32,
	11: iana.EFI_ARM64,
	15: iana.EFI_X86_HTTP,
	16: iana.EFI_X86_64_HTTP,
	17: iana.EFI_BC_HTTP,
	18: iana.EFI_ARM32_HTTP,
	19: iana.EFI_ARM64_HTTP,
	20: iana.INTEL_X86PC_HTTP,
	23: iana.UBOOT_ARM32_HTTP,
	24: iana.UBOOT_ARM64_HTTP,
	26: iana.EFI_RISCV32_HTTP,
	28: iana.EFI_RISCV64_HTTP,
	30: iana.EFI_RISCV128_HTTP,
}

// IANAArchFromPXEArch converts the integer parsed from
// PXEClient:Arch:00007:UNDI:003016 into iana.Arch.
func archFromVendorClass(arch int) iana.Arch {
	v, ok := dhcpv6VendorClassArchToIANA[arch]
	if !ok {
		return UnknownArch
	}
	return v
}

func prefixesOverlap(a, b netip.Prefix) bool {
	return a.Contains(b.Addr()) || b.Contains(a.Addr())
}

// UsableDerivedPrefix reports whether prefix can safely produce derived unicast addresses.
func UsableDerivedPrefix(prefix netip.Prefix) bool {
	if !prefix.IsValid() || !prefix.Addr().Is6() || prefix.Addr().Is4In6() || prefix.Bits() < 1 || prefix.Bits() > 64 {
		return false
	}

	prefix = prefix.Masked()
	for _, unusable := range unusableDerivedPrefixes {
		if prefixesOverlap(prefix, unusable) {
			return false
		}
	}

	return true
}
