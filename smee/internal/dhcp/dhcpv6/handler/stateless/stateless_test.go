package stateless

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/constant"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	tbotel "github.com/tinkerbell/tinkerbell/pkg/otel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type recordingPacketConn struct {
	writes [][]byte
	peers  []net.Addr
}

func (r *recordingPacketConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, nil }
func (r *recordingPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	buf := make([]byte, len(b))
	copy(buf, b)
	r.writes = append(r.writes, buf)
	r.peers = append(r.peers, addr)
	return len(b), nil
}
func (r *recordingPacketConn) Close() error                     { return nil }
func (r *recordingPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (r *recordingPacketConn) SetDeadline(time.Time) error      { return nil }
func (r *recordingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (r *recordingPacketConn) SetWriteDeadline(time.Time) error { return nil }

type mockBackend struct {
	hardware map[string]*tinkerbell.Hardware
	err      error
}

type hardwareNotFoundError struct{}

func (hardwareNotFoundError) Error() string  { return "not found" }
func (hardwareNotFoundError) NotFound() bool { return true }

func (m *mockBackend) FilterHardware(_ context.Context, opts data.HardwareFilter) (*tinkerbell.Hardware, error) {
	if m.err != nil {
		return nil, m.err
	}
	hw, ok := m.hardware[opts.ByMACAddress]
	if !ok {
		return nil, hardwareNotFoundError{}
	}
	return hw, nil
}

func TestHandleIgnoresUnsupportedMessageTypes(t *testing.T) {
	handler := newHandler(t, map[string]*tinkerbell.Hardware{})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, net.HardwareAddr{0, 1, 2, 3, 4, 5}, dhcpv6.MessageTypeSolicit, nil, iana.EFI_X86_64)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleDirectInformationRequestUsesExtractedMAC(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", []string{"2001:db8::53"}, "example.com"),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)
	requestStatelessConfig(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
	if got := reply.Options.InformationRefreshTime(0); got != defaultInformationRefreshTime {
		t.Fatalf("unexpected information refresh time: %v", got)
	}
	if got := reply.Options.VendorClass(clientEnterpriseNumber); len(got) != 0 {
		t.Fatalf("unexpected vendor class: %q", got)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::53")}, reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"example.com"}, reply.Options.DomainSearchList().Labels); diff != "" {
		t.Fatalf("unexpected domain search diff (-want +got):\n%s", diff)
	}
}

func TestHandleIgnoresInformationRequestAddressedToAnotherServer(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", []string{"2001:db8::53"}, "example.com"),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	dhcpv6.WithServerID(&dhcpv6.DUIDLL{
		HWType:        iana.HWTypeEthernet,
		LinkLayerAddr: net.HardwareAddr{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01},
	})(msg)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleDefaultInformationRefreshTimeDoesNotMutateHandler(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	handler.InformationRefreshTime = 0
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)
	requestStatelessConfig(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if handler.InformationRefreshTime != 0 {
		t.Fatalf("handler InformationRefreshTime was mutated: %v", handler.InformationRefreshTime)
	}
	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.InformationRefreshTime(0); got != defaultInformationRefreshTime {
		t.Fatalf("unexpected information refresh time: %v", got)
	}
}

func TestHandleInformationRefreshTimeBounds(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	tests := []struct {
		name       string
		configured time.Duration
		want       time.Duration
	}{
		{
			name:       "negative uses default",
			configured: -time.Second,
			want:       defaultInformationRefreshTime,
		},
		{
			name:       "custom positive value is preserved",
			configured: 30 * time.Minute,
			want:       30 * time.Minute,
		},
		{
			name:       "overflowing value is clamped",
			configured: maxInformationRefreshTime + time.Second,
			want:       maxInformationRefreshTime,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
			})
			handler.InformationRefreshTime = tc.configured
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
			requestBootURL(msg)

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessageReply(t, conn)
			if got := reply.Options.InformationRefreshTime(0); got != tc.want {
				t.Fatalf("unexpected information refresh time: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestHandleFiltersIPv4ServersFromDHCPv6Options(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, true, "", "", []string{"192.0.2.53", "2001:db8::53"}, "example.com")
	hardware.Spec.Interfaces[0].DHCP.TimeServers = []string{"192.0.2.123", "2001:db8::123"}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardware,
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)
	requestOptions(msg, dhcpv6.OptionDNSRecursiveNameServer, dhcpv6.OptionNTPServer)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::53")}, reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::123")}, reply.Options.NTPServers()); diff != "" {
		t.Fatalf("unexpected NTP diff (-want +got):\n%s", diff)
	}
}

func TestHandleDirectInformationRequestWithoutBootURLRequestOmitsBootURL(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", []string{"2001:db8::53"}, "example.com"),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestOptions(msg, dhcpv6.OptionDNSRecursiveNameServer)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "" {
		t.Fatalf("expected no boot file URL, got %q", got)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::53")}, reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS diff (-want +got):\n%s", diff)
	}
}

func TestHandleOmitsUnrequestedStatelessConfigOptions(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, true, "", "", []string{"2001:db8::53"}, "example.com")
	hardware.Spec.Interfaces[0].DHCP.TimeServers = []string{"2001:db8::123"}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardware,
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got == "" {
		t.Fatal("expected boot file URL")
	}
	if diff := cmp.Diff([]net.IP(nil), reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS diff (-want +got):\n%s", diff)
	}
	if labels := reply.Options.DomainSearchList(); labels != nil && len(labels.Labels) != 0 {
		t.Fatalf("unexpected domain search list: %#v", labels.Labels)
	}
	if diff := cmp.Diff([]net.IP(nil), reply.Options.NTPServers()); diff != "" {
		t.Fatalf("unexpected NTP diff (-want +got):\n%s", diff)
	}
}

func TestHandleNetbootDisabledOnlyOmitsBootURL(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, true, "", "", []string{"2001:db8::53"}, "example.com")
	hardware.Spec.Interfaces[0].DHCP.TimeServers = []string{"2001:db8::123"}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardware,
	})
	handler.Netboot.Enabled = false
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)
	requestStatelessConfig(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "" {
		t.Fatalf("expected no boot file URL, got %q", got)
	}
	if got := reply.Options.InformationRefreshTime(0); got != defaultInformationRefreshTime {
		t.Fatalf("unexpected information refresh time: %v", got)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::53")}, reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::123")}, reply.Options.NTPServers()); diff != "" {
		t.Fatalf("unexpected NTP diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"example.com"}, reply.Options.DomainSearchList().Labels); diff != "" {
		t.Fatalf("unexpected domain search diff (-want +got):\n%s", diff)
	}
}

func TestHandleDirectInformationRequestFallsBackToPeerEUI64(t *testing.T) {
	mac := net.HardwareAddr{0, 17, 34, 51, 68, 85}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg, err := dhcpv6.NewMessage(dhcpv6.WithClientID(&dhcpv6.DUIDEN{
		EnterpriseNumber:     1,
		EnterpriseIdentifier: []byte("test"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	msg.MessageType = dhcpv6.MessageTypeInformationRequest
	dhcpv6.WithArchType(iana.EFI_X86_64)(msg)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::211:22ff:fe33:4455"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "tftp://192.0.2.1:69/00:11:22:33:44:55/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
}

func TestHandleRelayInformationRequestFallsBackToDUIDMAC(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)
	relay := relayRequest(t, msg, nil)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	reply := requireSingleRelayReply(t, conn)
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	if inner.Options.BootFileURL() != "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", inner.Options.BootFileURL())
	}
}

func TestHandleRelayInformationRequestFallsBackToPeerEUI64(t *testing.T) {
	mac := net.HardwareAddr{0x08, 0x00, 0x27, 0x9e, 0xf5, 0x3a}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	handler.Netboot.IPXEScriptURL = func(mac net.HardwareAddr) *url.URL {
		u, _ := url.Parse("http://[fd8a:3f4b:7c91:1::201]:7080/ipxe/script/" + mac.String() + "/auto6.ipxe")
		return u
	}
	conn := &recordingPacketConn{}
	msg, err := dhcpv6.NewMessage(dhcpv6.WithClientID(&dhcpv6.DUIDUUID{}))
	if err != nil {
		t.Fatal(err)
	}
	msg.MessageType = dhcpv6.MessageTypeInformationRequest
	dhcpv6.WithUserClass([]byte("Tinkerbell"))(msg)
	dhcpv6.WithArchType(iana.EFI_ARM64)(msg)
	requestBootURL(msg)
	relay, err := dhcpv6.EncapsulateRelay(msg, dhcpv6.MessageTypeRelayForward, net.ParseIP("fd8a:3f4b:7c91:2::1"), net.ParseIP("fe80::a00:27ff:fe9e:f53a"))
	if err != nil {
		t.Fatal(err)
	}

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fd8a:3f4b:7c91:1::1"), Port: dhcpv6.DefaultServerPort}, relay)

	reply := requireSingleRelayReply(t, conn)
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	if got := inner.Options.BootFileURL(); got != "http://[fd8a:3f4b:7c91:1::201]:7080/ipxe/script/08:00:27:9e:f5:3a/auto6.ipxe" {
		t.Fatalf("unexpected script URL: %s", got)
	}
}

func TestHandleRelayInformationRequestReplyWrapped(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)
	relay := relayRequest(t, msg, mac)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	reply := requireSingleRelayReply(t, conn)
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	if inner.Options.BootFileURL() != "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", inner.Options.BootFileURL())
	}
	if got := inner.Options.VendorClass(clientEnterpriseNumber); len(got) != 0 {
		t.Fatalf("unexpected vendor class: %q", got)
	}
}

func TestHandleRelayInformationRequestWithoutClientIdentityIgnored(t *testing.T) {
	mac := net.HardwareAddr{0, 17, 34, 51, 68, 85}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeInformationRequest)
	requestBootURL(msg)
	relay := relayRequest(t, msg, nil)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleNestedRelayInformationRequestUsesInnermostRelayMAC(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	outerMAC := net.HardwareAddr{0, 1, 2, 3, 4, 6}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeInformationRequest)
	requestBootURL(msg)
	innerRelay := relayRequest(t, msg, mac)
	relay, err := dhcpv6.EncapsulateRelay(innerRelay, dhcpv6.MessageTypeRelayForward, net.ParseIP("2001:db8::2"), net.ParseIP("fe80::beef"))
	if err != nil {
		t.Fatal(err)
	}
	relay.AddOption(dhcpv6.OptClientLinkLayerAddress(iana.HWTypeEthernet, outerMAC))

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	reply := requireSingleRelayReply(t, conn)
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	if inner.Options.BootFileURL() != "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", inner.Options.BootFileURL())
	}
}

func TestHandleIgnoresUnknownHardware(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleDisallowedNetbootStillRepliesWithStatelessConfig(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, false, "", "", []string{"2001:db8::53"}, "example.com")
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardware,
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)
	requestOptions(msg, dhcpv6.OptionDNSRecursiveNameServer, dhcpv6.OptionDomainSearchList)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got, want := reply.Options.BootFileURL(), "http://boot.example/ipxe/script/00:01:02:03:04:05/netboot-not-allowed"; got != want {
		t.Fatalf("unexpected boot file URL: got %q want %q", got, want)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::53")}, reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"example.com"}, reply.Options.DomainSearchList().Labels); diff != "" {
		t.Fatalf("unexpected domain search diff (-want +got):\n%s", diff)
	}
}

func TestHandleIgnoresDisabledDHCPHardware(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, true, "", "", nil, "")
	hardware.Spec.Interfaces[0].DisableDHCP = true

	tests := map[string]*Handler{
		"stateless":      newHandler(t, map[string]*tinkerbell.Hardware{mac.String(): hardware}),
		"auto-stateless": newAutoHandler(t, map[string]*tinkerbell.Hardware{mac.String(): hardware}),
	}

	for name, handler := range tests {
		t.Run(name, func(t *testing.T) {
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
			requestBootURL(msg)

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestHandleAutoStatelessUnknownHardwareReturnsDefaultBinary(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
	if got := reply.Options.VendorClass(clientEnterpriseNumber); len(got) != 0 {
		t.Fatalf("unexpected vendor class: %q", got)
	}
	if diff := cmp.Diff([]net.IP(nil), reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS diff (-want +got):\n%s", diff)
	}
	if labels := reply.Options.DomainSearchList(); labels != nil && len(labels.Labels) != 0 {
		t.Fatalf("unexpected domain search list: %#v", labels.Labels)
	}
}

func TestHandleAutoStatelessIgnoresBackendLookupErrors(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandlerWithBackend(t, &mockBackend{err: errors.New("backend unavailable")}, true)
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleAutoStatelessIgnoresUnusableKnownHardware(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, true, "", "", nil, "")
	hardware.Spec.Interfaces[0].Netboot = nil
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardware,
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleAutoStatelessAppendsTraceparentToBootURL(t *testing.T) {
	const traceparent = "00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01"
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{})
	handler.OTELEnabled = true
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)
	ctx := contextWithTraceparent(t, traceparent)

	handler.Handle(ctx, conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got, want := reply.Options.BootFileURL(), "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi-"+traceparent; got != want {
		t.Fatalf("unexpected boot file URL: got %q want %q", got, want)
	}
}

func TestHandleAutoStatelessHTTPArchReturnsHTTPBinary(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64_HTTP)
	msg.AddOption(&dhcpv6.OptVendorClass{
		EnterpriseNumber: clientEnterpriseNumber,
		Data:             [][]byte{[]byte("PXEClient:Arch:00016:UNDI:003001")},
	})
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "http://boot.example/ipxe/binary/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
	if diff := cmp.Diff([][]byte{[]byte("HTTPClient")}, reply.Options.VendorClass(clientEnterpriseNumber)); diff != "" {
		t.Fatalf("unexpected vendor class diff (-want +got):\n%s", diff)
	}
}

func TestHandleNetbootReplyEchoesSelectedClientArch(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg := messageWithoutArch(t, mac, dhcpv6.MessageTypeInformationRequest, nil)
	msg.AddOption(dhcpv6.OptClientArchType(iana.Arch(65535), iana.EFI_X86_64, iana.EFI_ARM64))
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
	if diff := cmp.Diff(iana.Archs{iana.EFI_X86_64}, reply.Options.ArchTypes()); diff != "" {
		t.Fatalf("unexpected arch types diff (-want +got):\n%s", diff)
	}
}

func TestHandleAutoStatelessHTTPVendorClassFallbackReturnsHTTPBinary(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	msg.AddOption(&dhcpv6.OptVendorClass{
		EnterpriseNumber: clientEnterpriseNumber,
		Data:             [][]byte{[]byte("PXEClient:HTTPClient:Arch:00016:UNDI:003001")},
	})
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "http://boot.example/ipxe/binary/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
}

func TestHandleAutoStatelessHTTPVendorClassArchFallbackWithoutArchOptionReturnsHTTPBinary(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{})
	conn := &recordingPacketConn{}
	msg := messageWithoutArch(t, mac, dhcpv6.MessageTypeInformationRequest, nil)
	msg.AddOption(&dhcpv6.OptVendorClass{
		EnterpriseNumber: clientEnterpriseNumber,
		Data:             [][]byte{[]byte("HTTPClient:Arch:00016:UNDI:003001")},
	})
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "http://boot.example/ipxe/binary/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
}

func TestHandleAutoStatelessUnknownHardwareReturnsStaticScript(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, []byte("Tinkerbell"), iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "http://boot.example/ipxe/script/00:01:02:03:04:05/auto6.ipxe" {
		t.Fatalf("unexpected script URL: %s", got)
	}
}

func TestHandleAutoStatelessIPv6ScriptURLKeepsSchemeAndHost(t *testing.T) {
	mac := net.HardwareAddr{0x08, 0x00, 0x27, 0x9e, 0xf5, 0x3a}
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{})
	handler.Netboot.IPXEScriptURL = func(mac net.HardwareAddr) *url.URL {
		u, _ := url.Parse("http://[fd8a:3f4b:7c91:1::201]:7080/ipxe/script/" + mac.String() + "/auto6.ipxe")
		return u
	}
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, []byte("iPXE"), iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "http://[fd8a:3f4b:7c91:1::201]:7080/ipxe/script/08:00:27:9e:f5:3a/auto6.ipxe" {
		t.Fatalf("unexpected script URL: %s", got)
	}
}

func TestHandleAutoStatelessBootHintsWithoutOROOmitBootURLForUnknownHardware(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	tests := map[string]*dhcpv6.Message{
		"http client vendor class": messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64),
		"tinkerbell user class":    messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, []byte("Tinkerbell"), iana.EFI_X86_64),
		"ipxe user class":          messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, []byte("iPXE"), iana.EFI_X86_64),
	}
	tests["http client vendor class"].AddOption(&dhcpv6.OptVendorClass{
		EnterpriseNumber: clientEnterpriseNumber,
		Data:             [][]byte{[]byte("PXEClient:HTTPClient:Arch:00016:UNDI:003001")},
	})

	for name, msg := range tests {
		t.Run(name, func(t *testing.T) {
			handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{})
			conn := &recordingPacketConn{}

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessageReply(t, conn)
			if got := reply.Options.BootFileURL(); got != "" {
				t.Fatalf("expected no boot file URL, got %q", got)
			}
		})
	}
}

func TestHandleAutoStatelessKnownDisallowedNetbootRepliesWithoutBootURLWhenNotRequested(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, false, "", "", []string{"2001:db8::53"}, "example.com")
	hardware.Spec.Interfaces[0].DHCP.TimeServers = []string{"2001:db8::123"}
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardware,
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	msg.AddOption(&dhcpv6.OptVendorClass{
		EnterpriseNumber: clientEnterpriseNumber,
		Data:             [][]byte{[]byte("PXEClient:Arch:00016:UNDI:003001")},
	})
	requestOptions(msg, dhcpv6.OptionDNSRecursiveNameServer, dhcpv6.OptionNTPServer)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "" {
		t.Fatalf("expected no boot file URL, got %q", got)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::53")}, reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::123")}, reply.Options.NTPServers()); diff != "" {
		t.Fatalf("unexpected NTP diff (-want +got):\n%s", diff)
	}
}

func TestHandleAutoStatelessUnknownNormalInformationRequestOmitsBootURL(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newAutoHandler(t, map[string]*tinkerbell.Hardware{})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "" {
		t.Fatalf("expected no boot file URL, got %q", got)
	}
}

func TestHandleTinkerbellUserClassReturnsScriptURL(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, []byte("Tinkerbell"), iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "http://boot.example/ipxe/script/00:01:02:03:04:05/auto6.ipxe" {
		t.Fatalf("unexpected script URL: %s", got)
	}
}

func TestHandleTinkerbellUserClassReturnsCustomScriptURLUnchanged(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "https://boot.example/custom/auto.ipxe", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, []byte("Tinkerbell"), iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "https://boot.example/custom/auto.ipxe" {
		t.Fatalf("unexpected script URL: %s", got)
	}
}

func TestHandleIPXEUserClassReturnsScriptURL(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "", "", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, []byte("iPXE"), iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "http://boot.example/ipxe/script/00:01:02:03:04:05/auto6.ipxe" {
		t.Fatalf("unexpected script URL: %s", got)
	}
}

func TestHandleUsesCustomIPXEBinaryOverride(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, true, "snp-x86_64.efi", "", nil, ""),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "tftp://192.0.2.1:69/00:01:02:03:04:05/snp-x86_64.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
}

func TestHandleRequestArchTakesPrecedenceOverHardwareArch(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMACWithArch(mac, true, "", "", nil, "", "aarch64"),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_X86_64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
}

func TestHandleCustomIPXEBinaryOverrideTakesPrecedenceOverHardwareArch(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMACWithArch(mac, true, "snp-x86_64.efi", "", nil, "", "aarch64"),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest, nil, iana.EFI_ARM64)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessageReply(t, conn)
	if got := reply.Options.BootFileURL(); got != "tftp://192.0.2.1:69/00:01:02:03:04:05/snp-x86_64.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
}

func newHandler(t *testing.T, hardware map[string]*tinkerbell.Hardware) *Handler {
	t.Helper()
	return newHandlerWithBackend(t, &mockBackend{hardware: hardware}, false)
}

func newAutoHandler(t *testing.T, hardware map[string]*tinkerbell.Hardware) *Handler {
	t.Helper()
	return newHandlerWithBackend(t, &mockBackend{hardware: hardware}, true)
}

func newHandlerWithBackend(t *testing.T, backend *mockBackend, auto bool) *Handler {
	t.Helper()
	httpBinaryURL, err := url.Parse("http://boot.example/ipxe/binary")
	if err != nil {
		t.Fatal(err)
	}
	return &Handler{
		Backend: backend,
		ServerID: &dhcpv6.DUIDLL{
			HWType:        iana.HWTypeEthernet,
			LinkLayerAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		},
		Netboot: Netboot{
			IPXEBinServerTFTP: netip.MustParseAddrPort("192.0.2.1:69"),
			IPXEBinServerHTTP: httpBinaryURL,
			IPXEScriptURL: func(mac net.HardwareAddr) *url.URL {
				scriptURL := "http://boot.example/ipxe/script/auto6.ipxe"
				if mac != nil {
					scriptURL = "http://boot.example/ipxe/script/" + mac.String() + "/auto6.ipxe"
				}
				u, _ := url.Parse(scriptURL)
				return u
			},
			Enabled:             true,
			InjectMacAddrFormat: constant.MacAddrFormatColon,
		},
		AutoStatelessEnabled: auto,
	}
}

func messageWithMAC(t *testing.T, mac net.HardwareAddr, messageType dhcpv6.MessageType, userClass []byte, arch iana.Arch) *dhcpv6.Message {
	t.Helper()
	msg := messageWithoutArch(t, mac, messageType, userClass)
	dhcpv6.WithArchType(arch)(msg)
	return msg
}

func messageWithoutArch(t *testing.T, mac net.HardwareAddr, messageType dhcpv6.MessageType, userClass []byte) *dhcpv6.Message {
	t.Helper()
	msg, err := dhcpv6.NewMessage(dhcpv6.WithClientID(&dhcpv6.DUIDLL{
		HWType:        iana.HWTypeEthernet,
		LinkLayerAddr: mac,
	}))
	if err != nil {
		t.Fatal(err)
	}
	msg.MessageType = messageType
	if userClass != nil {
		dhcpv6.WithUserClass(userClass)(msg)
	}
	return msg
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
	dhcpv6.WithArchType(iana.EFI_X86_64)(msg)
	return msg
}

func requestBootURL(msg *dhcpv6.Message) {
	requestOptions(msg, dhcpv6.OptionBootfileURL)
}

func requestStatelessConfig(msg *dhcpv6.Message) {
	requestOptions(msg, dhcpv6.OptionDNSRecursiveNameServer, dhcpv6.OptionDomainSearchList, dhcpv6.OptionNTPServer)
}

func requestOptions(msg *dhcpv6.Message, options ...dhcpv6.OptionCode) {
	dhcpv6.WithRequestedOptions(options...)(msg)
}

func contextWithTraceparent(t *testing.T, traceparent string) context.Context {
	t.Helper()
	oldPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTextMapPropagator(oldPropagator)
	})
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tbotel.ContextWithTraceparentString(context.Background(), traceparent)
}

func relayRequest(t *testing.T, msg *dhcpv6.Message, mac net.HardwareAddr) *dhcpv6.RelayMessage {
	t.Helper()
	relay, err := dhcpv6.EncapsulateRelay(msg, dhcpv6.MessageTypeRelayForward, net.ParseIP("2001:db8::1"), net.ParseIP("fe80::abcd"))
	if err != nil {
		t.Fatal(err)
	}
	if mac != nil {
		relay.AddOption(dhcpv6.OptClientLinkLayerAddress(iana.HWTypeEthernet, mac))
	}
	return relay
}

func requireSingleMessageReply(t *testing.T, conn *recordingPacketConn) *dhcpv6.Message {
	t.Helper()
	if len(conn.writes) != 1 {
		t.Fatalf("expected one reply, got %d", len(conn.writes))
	}
	packet, err := dhcpv6.FromBytes(conn.writes[0])
	if err != nil {
		t.Fatal(err)
	}
	msg, ok := packet.(*dhcpv6.Message)
	if !ok {
		t.Fatalf("expected message reply, got %T", packet)
	}
	if msg.Type() != dhcpv6.MessageTypeReply {
		t.Fatalf("unexpected message type: %s", msg.Type())
	}
	return msg
}

func requireSingleRelayReply(t *testing.T, conn *recordingPacketConn) *dhcpv6.RelayMessage {
	t.Helper()
	if len(conn.writes) != 1 {
		t.Fatalf("expected one reply, got %d", len(conn.writes))
	}
	packet, err := dhcpv6.FromBytes(conn.writes[0])
	if err != nil {
		t.Fatal(err)
	}
	reply, ok := packet.(*dhcpv6.RelayMessage)
	if !ok {
		t.Fatalf("expected relay reply, got %T", packet)
	}
	if reply.Type() != dhcpv6.MessageTypeRelayReply {
		t.Fatalf("unexpected relay reply type: %s", reply.Type())
	}
	return reply
}

func hardwareForMAC(mac net.HardwareAddr, allowNetboot bool, binary, scriptURL string, nameServers []string, domainName string) *tinkerbell.Hardware {
	return hardwareForMACWithArch(mac, allowNetboot, binary, scriptURL, nameServers, domainName, "")
}

func hardwareForMACWithArch(mac net.HardwareAddr, allowNetboot bool, binary, scriptURL string, nameServers []string, domainName, arch string) *tinkerbell.Hardware {
	return &tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{
			Interfaces: []tinkerbell.Interface{
				{
					DHCP: &tinkerbell.DHCP{
						MAC:         mac.String(),
						NameServers: nameServers,
						DomainName:  domainName,
						Arch:        arch,
					},
					Netboot: &tinkerbell.Netboot{
						AllowPXE: &allowNetboot,
						IPXE: &tinkerbell.IPXE{
							Binary: binary,
							URL:    scriptURL,
						},
					},
				},
			},
		},
	}
}
