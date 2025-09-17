package dhcp

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
)

const (
	PXEClient  ClientType = "PXEClient"
	HTTPClient ClientType = "HTTPClient"
)

// known user-class types. must correspond to DHCP option 77 - User-Class
// https://www.rfc-editor.org/rfc/rfc3004.html
const (
	// If the client has had iPXE burned into its ROM (or is a VM
	// that uses iPXE as the PXE "ROM"), special handling is
	// needed because in this mode the client is using iPXE native
	// drivers and chainloading to a UNDI stack won't work.
	IPXE UserClass = "iPXE"
	// If the client identifies as "Tinkerbell", we've already
	// chainloaded this client to the full-featured copy of iPXE
	// we supply. We have to distinguish this case so we don't
	// loop on the chainload step.
	Tinkerbell UserClass = "Tinkerbell"
)

// UserClass is DHCP option 77 (https://www.rfc-editor.org/rfc/rfc3004.html).
type UserClass string

// ClientType is from DHCP option 60. Normally only PXEClient or HTTPClient.
type ClientType string

// Packet holds the data that is passed to a DHCP handler.
type Packet struct {
	// Peer is the address of the client that sent the DHCP message.
	Peer net.Addr
	// Pkt is the DHCP message.
	Pkt *dhcpv4.DHCPv4
	// Md is the metadata that was passed to the DHCP server.
	Md *Metadata
}

// Metadata holds metadata about the DHCP packet that was received.
type Metadata struct {
	// IfName is the name of the interface that the DHCP message was received on.
	IfName string
	// IfIndex is the index of the interface that the DHCP message was received on.
	IfIndex int
}

// ArchToBootFile maps supported hardware PXE architectures types to iPXE binary files.
func ArchToBootFile() map[iana.Arch]constant.IPXEBinary {
	return map[iana.Arch]constant.IPXEBinary{
		iana.INTEL_X86PC:       constant.IPXEBinaryUndionlyKPXE,
		iana.NEC_PC98:          constant.IPXEBinaryUndionlyKPXE,
		iana.EFI_ITANIUM:       constant.IPXEBinaryUndionlyKPXE,
		iana.DEC_ALPHA:         constant.IPXEBinaryUndionlyKPXE,
		iana.ARC_X86:           constant.IPXEBinaryUndionlyKPXE,
		iana.INTEL_LEAN_CLIENT: constant.IPXEBinaryUndionlyKPXE,
		iana.EFI_IA32:          constant.IPXEBinaryIPXEEFI,
		iana.EFI_X86_64:        constant.IPXEBinaryIPXEEFI,
		iana.EFI_XSCALE:        constant.IPXEBinaryIPXEEFI,
		iana.EFI_BC:            constant.IPXEBinaryIPXEEFI,
		iana.EFI_ARM32:         constant.IPXEBinarySNPARM64,
		iana.EFI_ARM64:         constant.IPXEBinarySNPARM64,
		iana.EFI_X86_HTTP:      constant.IPXEBinaryIPXEEFI,
		iana.EFI_X86_64_HTTP:   constant.IPXEBinaryIPXEEFI,
		iana.EFI_ARM32_HTTP:    constant.IPXEBinarySNPARM64,
		iana.EFI_ARM64_HTTP:    constant.IPXEBinarySNPARM64,
		iana.INTEL_X86PC_HTTP:  constant.IPXEBinaryUndionlyKPXE,
		iana.Arch(41):          constant.IPXEBinarySNPARM64, // arm rpiboot (0x29): https://www.iana.org/assignments/dhcpv6-parameters/dhcpv6-parameters.xhtml#processor-architecture
	}
}

// ErrUnknownArch is used when the PXE client request is from an unknown architecture.
var ErrUnknownArch = fmt.Errorf("could not determine client architecture from option 93")

// Info holds details about the dhcp request. Use NewInfo to populate the struct fields from a dhcp packet.
type Info struct {
	// Pkt is the dhcp packet that was received from the client.
	Pkt *dhcpv4.DHCPv4
	// Arch is the architecture of the client. Use NewInfo to automatically populate this field.
	Arch iana.Arch
	// Mac is the mac address of the client. Use NewInfo to automatically populate this field.
	Mac net.HardwareAddr
	// UserClass is the user class of the client. Use NewInfo to automatically populate this field.
	UserClass UserClass
	// ClientType is the client type of the client. Use NewInfo to automatically populate this field.
	ClientType ClientType
	// IsNetbootClient returns nil if the client is a valid netboot client.	Otherwise it returns an error.
	// Use NewInfo to automatically populate this field.
	IsNetbootClient error
	// IPXEBinary is the iPXE binary file to boot. Use NewInfo to automatically populate this field.
	IPXEBinary string
	// MacAddrFormat is the format to use when injecting the MAC address into the iPXE binary URL.
	MacAddrFormat constant.MACFormat
	// ArchMappingOverride allows customization for mapping architectures to iPXE binaries.
	// This is used to override the default ArchToBootFile mapping.
	ArchMappingOverride map[iana.Arch]constant.IPXEBinary
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

func WithIPXEBinary(binary string) InfoOption {
	return func(i *Info) {
		i.IPXEBinary = binary
	}
}

func NewInfo(pkt *dhcpv4.DHCPv4, opts ...InfoOption) Info {
	i := Info{Pkt: pkt}
	for _, opt := range opts {
		opt(&i)
	}
	if pkt != nil {
		i.Arch = Arch(pkt)
		i.Mac = pkt.ClientHWAddr
		i.UserClass = i.UserClassFrom()
		i.ClientType = i.ClientTypeFrom()
		i.IsNetbootClient = IsNetbootClient(pkt)
		if i.IPXEBinary == "" {
			i.IPXEBinary = i.IPXEBinaryFrom()
		}
	}

	return i
}

// isRaspberryPI checks if the mac address is from a Raspberry PI by matching prefixes against OUI registrations of the Raspberry Pi Trading Ltd.
// https://www.netify.ai/resources/macs/brands/raspberry-pi
// https://udger.com/resources/mac-address-vendor-detail?name=raspberry_pi_foundation
// https://macaddress.io/statistics/company/27594
func isRaspberryPI(mac net.HardwareAddr) bool {
	prefixes := [][]byte{
		{0xb8, 0x27, 0xeb}, // B8:27:EB
		{0xdc, 0xa6, 0x32}, // DC:A6:32
		{0xe4, 0x5f, 0x01}, // E4:5F:01
		{0x28, 0xcd, 0xc1}, // 28:CD:C1
		{0xd8, 0x3a, 0xdd}, // D8:3A:DD
	}
	for _, prefix := range prefixes {
		if bytes.HasPrefix(mac, prefix) {
			return true
		}
	}

	return false
}

// Arch returns the Arch of the client pulled from DHCP option 93.
func Arch(d *dhcpv4.DHCPv4) iana.Arch {
	// if the mac address is from a Raspberry PI, use the Raspberry PI architecture.
	// Some Raspberry PI's (Raspberry PI 5) report an option 93 of 0.
	// This translates to iana.INTEL_X86PC and causes us to map to undionly.kpxe.
	if isRaspberryPI(d.ClientHWAddr) {
		return iana.Arch(41)
	}

	// get option 93 ; arch
	fwt := d.ClientArch()
	if len(fwt) == 0 {
		return iana.Arch(255) // unknown arch
	}
	var archKnown bool
	var a iana.Arch
	for _, elem := range fwt {
		if !strings.Contains(elem.String(), "unknown") {
			archKnown = true
			// Basic architecture identification, based purely on
			// the PXE architecture option.
			// https://www.iana.org/assignments/dhcpv6-parameters/dhcpv6-parameters.xhtml#processor-architecture
			a = elem
			break
		}
	}
	if !archKnown {
		return iana.Arch(255) // unknown arch
	}

	return a
}

func (i Info) IPXEBinaryFrom() string {
	dst := ArchToBootFile()
	src := i.ArchMappingOverride
	maps.Copy(dst, src)
	bin, found := dst[i.Arch]
	if !found {
		return ""
	}

	return bin.String()
}

// String function for clientType.
func (c ClientType) String() string {
	return string(c)
}

// String function for UserClass.
func (u UserClass) String() string {
	return string(u)
}

func (i Info) UserClassFrom() UserClass {
	var u UserClass
	if i.Pkt != nil {
		if val := i.Pkt.Options.Get(dhcpv4.OptionUserClassInformation); val != nil {
			u = UserClass(string(val))
		}
	}

	return u
}

func (i Info) ClientTypeFrom() ClientType {
	var c ClientType
	if i.Pkt != nil {
		if val := i.Pkt.Options.Get(dhcpv4.OptionClassIdentifier); val != nil {
			if strings.HasPrefix(string(val), HTTPClient.String()) {
				c = HTTPClient
			} else {
				c = PXEClient
			}
		}
	}

	return c
}

// IsNetbootClient returns nil if the client is a valid netboot client.	Otherwise it returns an error.
//
// A valid netboot client will have the following in its DHCP request:
// 1. is a DHCP discovery/request message type.
// 2. option 93 is set.
// 3. option 94 is set.
// 4. option 97 is correct length.
// 5. option 60 is set with this format: "PXEClient:Arch:xxxxx:UNDI:yyyzzz" or "HTTPClient:Arch:xxxxx:UNDI:yyyzzz".
//
// See: http://www.pix.net/software/pxeboot/archive/pxespec.pdf
//
// See: https://www.rfc-editor.org/rfc/rfc4578.html
func IsNetbootClient(pkt *dhcpv4.DHCPv4) error {
	var err error
	// only response to DISCOVER and REQUEST packets
	if pkt.MessageType() != dhcpv4.MessageTypeDiscover && pkt.MessageType() != dhcpv4.MessageTypeRequest {
		err = wrapNonNil(err, "message type must be either Discover or Request")
	}
	// option 60 must be set
	if !pkt.Options.Has(dhcpv4.OptionClassIdentifier) {
		err = wrapNonNil(err, "option 60 not set")
	}
	// option 60 must start with PXEClient or HTTPClient
	opt60 := pkt.GetOneOption(dhcpv4.OptionClassIdentifier)
	if !strings.HasPrefix(string(opt60), string(PXEClient)) && !strings.HasPrefix(string(opt60), string(HTTPClient)) {
		err = wrapNonNil(err, "option 60 not PXEClient or HTTPClient")
	}

	// option 93 must be set
	if !pkt.Options.Has(dhcpv4.OptionClientSystemArchitectureType) {
		err = wrapNonNil(err, "option 93 not set")
	}

	// option 94 must be set
	if !pkt.Options.Has(dhcpv4.OptionClientNetworkInterfaceIdentifier) {
		err = wrapNonNil(err, "option 94 not set")
	}

	// option 97 must be have correct length or not be set
	guid := pkt.GetOneOption(dhcpv4.OptionClientMachineIdentifier)
	switch len(guid) {
	case 0:
		// A missing GUID is invalid according to the spec, however
		// there are PXE ROMs in the wild that omit the GUID and still
		// expect to boot. The only thing we do with the GUID is
		// mirror it back to the client if it's there, so we might as
		// well accept these buggy ROMs.
	case 17:
		if guid[0] != 0 {
			err = wrapNonNil(err, "option 97 does not start with 0")
		}
	default:
		err = wrapNonNil(err, "option 97 has invalid length (must be 0 or 17)")
	}

	return err
}

func wrapNonNil(err error, format string) error {
	if err == nil {
		return errors.New(format)
	}

	return fmt.Errorf("%w: %v", err, format)
}

// Bootfile returns the calculated dhcp header: "file" value. see https://datatracker.ietf.org/doc/html/rfc2131#section-2 .
func (i Info) Bootfile(customUC UserClass, ipxeScript, ipxeHTTPBinServer *url.URL, ipxeTFTPBinServer netip.AddrPort) string {
	bootfile := "/no-ipxe-script-defined"

	// If a machine is in an ipxe boot loop, it is likely to be that we aren't matching on IPXE or Tinkerbell userclass (option 77).
	switch { // order matters here.
	case i.UserClass == Tinkerbell, (customUC != "" && i.UserClass == customUC): // this case gets us out of an ipxe boot loop.
		if ipxeScript != nil {
			bootfile = ipxeScript.String()
		}
	case i.ClientType == HTTPClient: // Check the client type from option 60.
		if ipxeHTTPBinServer != nil {
			paths := []string{i.IPXEBinary}
			if i.Mac != nil {
				paths = append([]string{macAddrFormat(i.Mac, i.MacAddrFormat)}, paths...)
			}
			bootfile = ipxeHTTPBinServer.JoinPath(paths...).String()
		}
	case i.UserClass == IPXE: // if the "iPXE" user class is found it means we aren't in our custom version of ipxe, but because of the option 43 we're setting we need to give a full tftp url from which to boot.
		t := url.URL{
			Scheme: "tftp",
			Host:   ipxeTFTPBinServer.String(),
		}
		paths := []string{i.IPXEBinary}
		if i.Mac != nil {
			paths = append([]string{macAddrFormat(i.Mac, i.MacAddrFormat)}, paths...)
		}
		bootfile = t.JoinPath(paths...).String()
	default:
		if i.IPXEBinary != "" {
			bootfile = i.IPXEBinary
		}
	}

	return bootfile
}

func macAddrFormat(mac net.HardwareAddr, f constant.MACFormat) string {
	switch f {
	case constant.MacAddrFormatColon:
		return mac.String()
	case constant.MacAddrFormatDot:
		return dotNotation(mac)
	case constant.MacAddrFormatDash:
		return dashNotation(mac)
	case constant.MacAddrFormatNoDelimiter:
		return noDelimiter(mac)
	case constant.MacAddrFormatEmpty:
		return ""
	default:
		return mac.String() // default is colon delimited
	}
}

// NextServer returns the calculated dhcp header (ServerIPAddr): "siaddr" value. see https://datatracker.ietf.org/doc/html/rfc2131#section-2 .
func (i Info) NextServer(ipxeHTTPBinServer *url.URL, ipxeTFTPBinServer netip.AddrPort, publicIP netip.Addr) net.IP {
	var nextServer net.IP

	// If a machine is in an ipxe boot loop, it is likely to be that we aren't matching on IPXE or Tinkerbell userclass (option 77).
	switch { // order matters here.
	case i.ClientType == HTTPClient: // Check the client type from option 60.
		if ipxeHTTPBinServer != nil {
			// remove port if it exists as net.IP doesn't support ports.
			if ip := net.ParseIP(ipxeHTTPBinServer.Hostname()); ip != nil {
				nextServer = ip
			}
		}
	case i.UserClass == IPXE: // if the "iPXE" user class is found it means we aren't in our custom version of ipxe, but because of the option 43 we're setting we need to give a full tftp url from which to boot.
		nextServer = ipxeTFTPBinServer.Addr().AsSlice()
	default:
		nextServer = ipxeTFTPBinServer.Addr().AsSlice()
	}

	// If the nextServer is nil or unspecified, it might that the ipxeHTTPBinServer has a port or is a DNS name.
	// ipxeHTTPBinServer is perfectly acceptable to have a port or be a DNS name but we can't use that in DHCP in headers
	// fields siaddr or sname and can't be used in DHCP options like option 54 (dhcp server identifier).
	if (nextServer == nil || nextServer.IsUnspecified()) && publicIP.IsValid() && !publicIP.IsUnspecified() {
		nextServer = publicIP.AsSlice()
	}

	return nextServer
}

// AddRPIOpt43 adds the Raspberry PI required option43 sub options to an existing opt 43.
func (i Info) AddRPIOpt43(opts dhcpv4.Options) []byte {
	// these are suboptions of option43. ref: https://datatracker.ietf.org/doc/html/rfc2132#section-8.4
	if isRaspberryPI(i.Mac) {
		// TODO document what these hex strings are and why they are needed.
		// https://www.raspberrypi.org/documentation/computers/raspberry-pi.html#PXE_OPTION43
		// tested with Raspberry Pi 4 using UEFI from here: https://github.com/pftf/RPi4/releases/tag/v1.31
		// all files were served via a tftp server and lived at the top level dir of the tftp server (i.e tftp://server/)
		// "\x00\x00\x11" is equal to NUL(Null), NUL(Null), DC1(Device Control 1)
		opt9, _ := hex.DecodeString("00001152617370626572727920506920426f6f74") // "\x00\x00\x11Raspberry Pi Boot"
		opts[9] = opt9
		// "\x0a\x04\x00" is equal to LF(Line Feed), EOT(End of Transmission), NUL(Null)
		opt10, _ := hex.DecodeString("00505845") // "\x0a\x04\x00PXE"
		opts[10] = opt10
	}

	return opts.ToBytes()
}
