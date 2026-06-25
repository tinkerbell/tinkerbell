package dhcpv6

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp"
	"go.opentelemetry.io/otel/attribute"
)

func TestMacFromPacketDirectFallsBackToPeerEUI64(t *testing.T) {
	clientMAC := net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeInformationRequest)
	peer := &net.UDPAddr{
		IP:   eui64LinkLocal(clientMAC),
		Port: dhcpv6.DefaultClientPort,
	}

	got, err := macFromPacket(msg, peer, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != clientMAC.String() {
		t.Fatalf("unexpected MAC: got %s want %s", got, clientMAC)
	}
}

func TestMacFromPacketDirectDoesNotFallBackToNonLinkLocalPeerEUI64(t *testing.T) {
	clientMAC := net.HardwareAddr{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeInformationRequest)
	peer := &net.UDPAddr{
		IP:   eui64Global(clientMAC),
		Port: dhcpv6.DefaultClientPort,
	}

	got, err := macFromPacket(msg, peer, logr.Discard())
	if err == nil {
		t.Fatalf("expected no reliable direct client identity, got MAC %s", got)
	}
	if got != nil {
		t.Fatalf("expected no MAC, got %s", got)
	}
}

func TestMacFromPacketRelayDoesNotFallBackToUDPPeerEUI64(t *testing.T) {
	relayMAC := net.HardwareAddr{0x00, 0xaa, 0xbb, 0xcc, 0xdd, 0xee}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeInformationRequest)
	relay, err := dhcpv6.EncapsulateRelay(
		msg,
		dhcpv6.MessageTypeRelayForward,
		net.ParseIP("2001:db8::1"),
		net.ParseIP("fe80::abcd"),
	)
	if err != nil {
		t.Fatal(err)
	}
	peer := &net.UDPAddr{
		IP:   eui64LinkLocal(relayMAC),
		Port: dhcpv6.DefaultServerPort,
	}

	got, err := macFromPacket(relay, peer, logr.Discard())
	if err == nil {
		t.Fatalf("expected no reliable relay client identity, got MAC %s", got)
	}
	if got != nil {
		t.Fatalf("expected no MAC, got %s", got)
	}
}

func TestArchFromVendorClassHTTPBootArchitectures(t *testing.T) {
	tests := map[int]iana.Arch{
		7:  iana.EFI_X86_64,
		9:  iana.EFI_BC,
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

	for arch, want := range tests {
		t.Run(want.String(), func(t *testing.T) {
			if got := archFromVendorClass(arch); got != want {
				t.Fatalf("archFromVendorClass(%d) = %s, want %s", arch, got, want)
			}
		})
	}
}

func TestBootURLDisallowedNetbootNotAllowedURL(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	info := Info{Mac: mac}
	disallowedNetboot := &dhcp.Netboot{AllowNetboot: false}

	tests := map[string]struct {
		scriptURL func(net.HardwareAddr) *url.URL
		want      string
		wantErr   bool
	}{
		"nil builder": {
			scriptURL: nil,
			wantErr:   true,
		},
		"builder returns nil": {
			scriptURL: func(net.HardwareAddr) *url.URL {
				return nil
			},
			wantErr: true,
		},
		"injected MAC path": {
			scriptURL: scriptURLBuilder(t, "http://boot.example/ipxe/script/"+mac.String()+"/auto6.ipxe"),
			want:      "http://boot.example/ipxe/script/00:01:02:03:04:05/netboot-not-allowed",
		},
		"non injected MAC path": {
			scriptURL: scriptURLBuilder(t, "https://boot.example/custom/auto6.ipxe"),
			want:      "https://boot.example/custom/netboot-not-allowed",
		},
		"query and fragment are stripped": {
			scriptURL: scriptURLBuilder(t, "https://boot.example/custom/auto6.ipxe?token=secret#section"),
			want:      "https://boot.example/custom/netboot-not-allowed",
		},
		"trailing slash path": {
			scriptURL: scriptURLBuilder(t, "http://boot.example/ipxe/script/"+mac.String()+"/"),
			want:      "http://boot.example/ipxe/script/00:01:02:03:04:05/netboot-not-allowed",
		},
		"IPv6 host": {
			scriptURL: scriptURLBuilder(t, "http://[fd8a:3f4b:7c91:1::201]:7080/ipxe/script/"+mac.String()+"/auto6.ipxe"),
			want:      "http://[fd8a:3f4b:7c91:1::201]:7080/ipxe/script/00:01:02:03:04:05/netboot-not-allowed",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := info.BootURL(BootURLConfig{
				HardwareNetboot: disallowedNetboot,
				IPXEScriptURL:   tt.scriptURL,
			})
			if tt.wantErr {
				if !errors.Is(err, ErrNoBootURL) {
					t.Fatalf("expected ErrNoBootURL, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("unexpected boot URL: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestBootURLReturnsErrNoBootURL(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	httpBinaryURL := mustParseURL(t, "http://boot.example/ipxe/binary")
	tftpServer := netip.MustParseAddrPort("192.0.2.1:69")

	tests := map[string]struct {
		info   Info
		config BootURLConfig
	}{
		"missing iPXE binary": {
			info: Info{Mac: mac},
			config: BootURLConfig{
				IPXEBinServerTFTP: tftpServer,
			},
		},
		"missing default script URL builder": {
			info: Info{
				Mac:         mac,
				UserClasses: []dhcp.UserClass{dhcp.IPXE},
			},
		},
		"nil generated script URL": {
			info: Info{
				Mac:         mac,
				UserClasses: []dhcp.UserClass{dhcp.IPXE},
			},
			config: BootURLConfig{
				IPXEScriptURL: func(net.HardwareAddr) *url.URL {
					return nil
				},
			},
		},
		"missing HTTP binary server": {
			info: Info{
				Mac:        mac,
				Arch:       iana.EFI_X86_64_HTTP,
				IPXEBinary: "ipxe.efi",
			},
		},
		"invalid TFTP binary server": {
			info: Info{
				Mac:        mac,
				Arch:       iana.EFI_X86_64,
				IPXEBinary: "ipxe.efi",
			},
		},
		"hardware binary override with missing HTTP binary server": {
			info: Info{
				Mac:  mac,
				Arch: iana.EFI_X86_64_HTTP,
			},
			config: BootURLConfig{
				HardwareNetboot: &dhcp.Netboot{AllowNetboot: true, IPXEBinary: "custom.efi"},
			},
		},
		"hardware binary override with invalid TFTP binary server": {
			info: Info{
				Mac:  mac,
				Arch: iana.EFI_X86_64,
			},
			config: BootURLConfig{
				HardwareNetboot: &dhcp.Netboot{AllowNetboot: true, IPXEBinary: "custom.efi"},
			},
		},
		"missing binary checked before valid HTTP server": {
			info: Info{
				Mac:  mac,
				Arch: iana.EFI_X86_64_HTTP,
			},
			config: BootURLConfig{
				IPXEBinServerHTTP: httpBinaryURL,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tt.info.BootURL(tt.config)
			if got != "" {
				t.Fatalf("expected empty boot URL, got %q", got)
			}
			if !errors.Is(err, ErrNoBootURL) {
				t.Fatalf("expected ErrNoBootURL, got %v", err)
			}
		})
	}
}

func TestBootURLDisallowedNetbootDoesNotMutateScriptURL(t *testing.T) {
	info := Info{Mac: net.HardwareAddr{0, 1, 2, 3, 4, 5}}
	u := mustParseURL(t, "https://boot.example/custom/auto6.ipxe?token=secret#section")

	got, err := info.BootURL(BootURLConfig{
		HardwareNetboot: &dhcp.Netboot{AllowNetboot: false},
		IPXEScriptURL: func(net.HardwareAddr) *url.URL {
			return u
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://boot.example/custom/netboot-not-allowed"; got != want {
		t.Fatalf("unexpected boot URL: got %q want %q", got, want)
	}
	if want := "https://boot.example/custom/auto6.ipxe?token=secret#section"; u.String() != want {
		t.Fatalf("script URL was mutated: got %q want %q", u.String(), want)
	}
}

func TestApplyBootOptionsAddsHTTPBootOptions(t *testing.T) {
	request := messageWithDUIDEN(t, dhcpv6.MessageTypeInformationRequest)
	request.AddOption(dhcpv6.OptClientArchType(iana.EFI_X86_64_HTTP))
	reply := &dhcpv6.Message{}

	ApplyBootOptions(reply, Info{Msg: request, Arch: iana.EFI_X86_64_HTTP}, "http://boot.example/ipxe.efi")

	if got := reply.Options.BootFileURL(); got != "http://boot.example/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
	if got := reply.Options.ArchTypes(); len(got) != 1 || got[0] != iana.EFI_X86_64_HTTP {
		t.Fatalf("unexpected arch types: %v", got)
	}
	if got := reply.Options.VendorClass(ClientEnterpriseNumber); len(got) != 1 || string(got[0]) != dhcp.HTTPClient.String() {
		t.Fatalf("unexpected vendor class: %q", got)
	}
}

func TestApplyRequestedStatelessOptionsFiltersIPv6(t *testing.T) {
	request := messageWithDUIDEN(t, dhcpv6.MessageTypeInformationRequest)
	dhcpv6.WithRequestedOptions(
		dhcpv6.OptionDNSRecursiveNameServer,
		dhcpv6.OptionDomainSearchList,
		dhcpv6.OptionNTPServer,
	)(request)
	reply := &dhcpv6.Message{}

	ApplyRequestedStatelessOptions(reply, Info{Msg: request}, &dhcp.DHCP{
		NameServers:  []net.IP{net.ParseIP("192.0.2.53"), net.ParseIP("2001:db8::53")},
		DomainSearch: []string{"example.com"},
		NTPServers:   []net.IP{net.ParseIP("192.0.2.123"), net.ParseIP("2001:db8::123")},
	})

	if got := reply.Options.DNS(); len(got) != 1 || !got[0].Equal(net.ParseIP("2001:db8::53")) {
		t.Fatalf("unexpected DNS servers: %v", got)
	}
	if got := reply.Options.DomainSearchList(); got == nil || len(got.Labels) != 1 || got.Labels[0] != "example.com" {
		t.Fatalf("unexpected domain search list: %v", got)
	}
	if got := reply.Options.NTPServers(); len(got) != 1 || !got[0].Equal(net.ParseIP("2001:db8::123")) {
		t.Fatalf("unexpected NTP servers: %v", got)
	}
}

func TestEncodeToAttributesIncludesIAAddressWhenRequested(t *testing.T) {
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeReply)
	dhcpv6.WithIANA(dhcpv6.OptIAAddress{IPv6Addr: net.ParseIP("2001:db8::100")})(msg)

	attrs := EncodeToAttributes(msg, "reply", true)
	if !hasStringAttribute(attrs, "DHCP.reply.Opt5.IAAddr", "2001:db8::100") {
		t.Fatalf("expected IA address attribute in %v", attrs)
	}

	attrs = EncodeToAttributes(msg, "reply", false)
	if hasAttribute(attrs, "DHCP.reply.Opt5.IAAddr") {
		t.Fatalf("unexpected IA address attribute in %v", attrs)
	}
}

func TestWriteReplyWrapsRelayResponses(t *testing.T) {
	request := messageWithDUIDEN(t, dhcpv6.MessageTypeInformationRequest)
	relay, err := dhcpv6.EncapsulateRelay(request, dhcpv6.MessageTypeRelayForward, net.ParseIP("2001:db8::1"), net.ParseIP("fe80::abcd"))
	if err != nil {
		t.Fatal(err)
	}
	reply := &dhcpv6.Message{
		MessageType:   dhcpv6.MessageTypeReply,
		TransactionID: request.TransactionID,
	}
	conn := &recordingPacketConn{}

	response, err := WriteReply(conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay, reply)
	if err != nil {
		t.Fatal(err)
	}
	if len(conn.writes) != 1 {
		t.Fatalf("expected one write, got %d", len(conn.writes))
	}
	if response.Type() != dhcpv6.MessageTypeRelayReply {
		t.Fatalf("unexpected response type: %s", response.Type())
	}
	packet, err := dhcpv6.FromBytes(conn.writes[0])
	if err != nil {
		t.Fatal(err)
	}
	if packet.Type() != dhcpv6.MessageTypeRelayReply {
		t.Fatalf("unexpected wire response type: %s", packet.Type())
	}
}

func messageWithDUIDEN(t *testing.T, messageType dhcpv6.MessageType) *dhcpv6.Message {
	t.Helper()
	msg, err := dhcpv6.NewMessage(dhcpv6.WithClientID(&dhcpv6.DUIDEN{
		EnterpriseNumber:     1,
		EnterpriseIdentifier: []byte("test"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	msg.MessageType = messageType
	return msg
}

type recordingPacketConn struct {
	writes [][]byte
}

func (r *recordingPacketConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, nil }
func (r *recordingPacketConn) WriteTo(b []byte, _ net.Addr) (int, error) {
	r.writes = append(r.writes, append([]byte(nil), b...))
	return len(b), nil
}
func (r *recordingPacketConn) Close() error                     { return nil }
func (r *recordingPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (r *recordingPacketConn) SetDeadline(time.Time) error      { return nil }
func (r *recordingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (r *recordingPacketConn) SetWriteDeadline(time.Time) error { return nil }

func hasAttribute(attrs []attribute.KeyValue, key string) bool {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return true
		}
	}
	return false
}

func hasStringAttribute(attrs []attribute.KeyValue, key, value string) bool {
	for _, attr := range attrs {
		if string(attr.Key) == key && attr.Value.AsString() == value {
			return true
		}
	}
	return false
}

func scriptURLBuilder(t *testing.T, raw string) func(net.HardwareAddr) *url.URL {
	t.Helper()
	u := mustParseURL(t, raw)
	return func(net.HardwareAddr) *url.URL {
		return u
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func eui64LinkLocal(mac net.HardwareAddr) net.IP {
	return net.IP{
		0xfe, 0x80, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		mac[0] ^ 0x02, mac[1], mac[2], 0xff,
		0xfe, mac[3], mac[4], mac[5],
	}
}

func eui64Global(mac net.HardwareAddr) net.IP {
	return net.IP{
		0x20, 0x01, 0x0d, 0xb8,
		0x00, 0x00, 0x00, 0x01,
		mac[0] ^ 0x02, mac[1], mac[2], 0xff,
		0xfe, mac[3], mac[4], mac[5],
	}
}
