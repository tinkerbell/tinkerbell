package reservation

import (
	"context"
	"errors"
	"fmt"
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

func TestHandleSolicitReturnsAdvertiseWithReservation(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, "2001:db8::100", "", true)
	hardware.Spec.Interfaces[0].DHCP.LeaseTime = 7200
	hardware.Spec.Interfaces[0].DHCP.NameServers = []string{"192.0.2.53", "2001:db8::53"}
	hardware.Spec.Interfaces[0].DHCP.TimeServers = []string{"192.0.2.123", "2001:db8::123"}
	hardware.Spec.Interfaces[0].DHCP.DomainName = "example.com"
	handler := newHandler(t, map[string]*tinkerbell.Hardware{mac.String(): hardware})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})
	dhcpv6.WithIAID([4]byte{9, 8, 7, 6})(msg)
	dhcpv6.WithRequestedOptions(
		dhcpv6.OptionDNSRecursiveNameServer,
		dhcpv6.OptionDomainSearchList,
		dhcpv6.OptionNTPServer,
	)(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeAdvertise)
	requireReservation(t, reply, [4]byte{9, 8, 7, 6}, net.ParseIP("2001:db8::100"), time.Hour, 2*time.Hour)
	requireIANATimers(t, reply, time.Hour, 96*time.Minute)
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

func TestHandleSolicitOmitsUnrequestedStatelessOptions(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, "2001:db8::100", "", true)
	hardware.Spec.Interfaces[0].DHCP.NameServers = []string{"2001:db8::53"}
	hardware.Spec.Interfaces[0].DHCP.TimeServers = []string{"2001:db8::123"}
	hardware.Spec.Interfaces[0].DHCP.DomainName = "example.com"
	handler := newHandler(t, map[string]*tinkerbell.Hardware{mac.String(): hardware})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	dhcpv6.WithIANA()(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeAdvertise)
	if got := reply.Options.DNS(); len(got) != 0 {
		t.Fatalf("expected no DNS servers without ORO, got %v", got)
	}
	if got := reply.Options.NTPServers(); len(got) != 0 {
		t.Fatalf("expected no NTP servers without ORO, got %v", got)
	}
	if got := reply.Options.DomainSearchList(); got != nil {
		t.Fatalf("expected no domain search without ORO, got %v", got.Labels)
	}
}

func TestHandleSolicitReturnsReservationForOneIANA(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{1, 1, 1, 1})
	addIANA(t, msg, [4]byte{2, 2, 2, 2})

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeAdvertise)
	requireReservationForIAID(t, reply, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))
	requireIANAStatusForIAID(t, reply, [4]byte{2, 2, 2, 2}, iana.StatusNoAddrsAvail)
}

func TestHandleSolicitClampsTinyLeaseTimes(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	for _, leaseTime := range []int64{1, 2, 3} {
		t.Run(fmt.Sprintf("%ds", leaseTime), func(t *testing.T) {
			hardware := hardwareForMAC(mac, "2001:db8::100", "", true)
			hardware.Spec.Interfaces[0].DHCP.LeaseTime = leaseTime
			handler := newHandler(t, map[string]*tinkerbell.Hardware{mac.String(): hardware})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
			dhcpv6.WithIANA()(msg)
			dhcpv6.WithIAID([4]byte{9, 8, 7, 6})(msg)

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeAdvertise)
			requireReservation(t, reply, [4]byte{9, 8, 7, 6}, net.ParseIP("2001:db8::100"), 30*time.Second, time.Minute)
			requireIANATimers(t, reply, 30*time.Second, 48*time.Second)
		})
	}
}

func TestHandleRequestRenewRebindReturnReply(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	tests := []dhcpv6.MessageType{
		dhcpv6.MessageTypeRequest,
		dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRebind,
	}

	for _, messageType := range tests {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			addIANAAddresses(msg, net.ParseIP("2001:db8::100"))
			if requiresServerID(messageType) {
				withHandlerServerID(handler)(msg)
			}

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireReservation(t, reply, [4]byte{9, 8, 7, 6}, net.ParseIP("2001:db8::100"), 84*time.Hour, 168*time.Hour)
		})
	}
}

func TestHandleRebindDoesNotRequireServerID(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeRebind)
	addIANAAddresses(msg, net.ParseIP("2001:db8::100"))

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
	requireReservation(t, reply, [4]byte{9, 8, 7, 6}, net.ParseIP("2001:db8::100"), 84*time.Hour, 168*time.Hour)
}

func TestHandleRequestReturnsReservationWithoutRequestedAddress(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeRequest)
	addIANA(t, msg, [4]byte{9, 8, 7, 6})
	withHandlerServerID(handler)(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
	requireReservation(t, reply, [4]byte{9, 8, 7, 6}, net.ParseIP("2001:db8::100"), 84*time.Hour, 168*time.Hour)
}

func TestHandleStatefulMessageWithoutIANAIgnored(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	pool := netip.MustParsePrefix("2001:db8:abcd::/64")
	tests := []struct {
		name        string
		messageType dhcpv6.MessageType
		hardware    *tinkerbell.Hardware
		configure   func(*Handler)
	}{
		{
			name:        "reservation solicit",
			messageType: dhcpv6.MessageTypeSolicit,
			hardware:    hardwareForMAC(mac, "2001:db8::100", "", true),
		},
		{
			name:        "reservation request",
			messageType: dhcpv6.MessageTypeRequest,
			hardware:    hardwareForMAC(mac, "2001:db8::100", "", true),
		},
		{
			name:        "reservation renew",
			messageType: dhcpv6.MessageTypeRenew,
			hardware:    hardwareForMAC(mac, "2001:db8::100", "", true),
		},
		{
			name:        "reservation rebind",
			messageType: dhcpv6.MessageTypeRebind,
			hardware:    hardwareForMAC(mac, "2001:db8::100", "", true),
		},
		{
			name:        "derived solicit",
			messageType: dhcpv6.MessageTypeSolicit,
			hardware:    hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
			configure: func(handler *Handler) {
				handler.Derived = true
				handler.DerivedDirectAddressPool = pool
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): test.hardware,
			})
			if test.configure != nil {
				test.configure(handler)
			}
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, test.messageType)
			if requiresServerID(test.messageType) {
				withHandlerServerID(handler)(msg)
			}

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestHandleRenewRebindReturnNoBindingWithoutRequestedAddress(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	tests := []dhcpv6.MessageType{
		dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRebind,
	}

	for _, messageType := range tests {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			dhcpv6.WithIANA()(msg)
			if requiresServerID(messageType) {
				withHandlerServerID(handler)(msg)
			}

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireIANAStatus(t, reply, iana.StatusNoBinding)
		})
	}
}

func TestHandleRequestRenewRebindReturnStatusForMismatchedAddress(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	tests := []struct {
		messageType dhcpv6.MessageType
		wantStatus  iana.StatusCode
	}{
		{messageType: dhcpv6.MessageTypeRequest, wantStatus: iana.StatusNoAddrsAvail},
		{messageType: dhcpv6.MessageTypeRenew, wantStatus: iana.StatusNoBinding},
		{messageType: dhcpv6.MessageTypeRebind, wantStatus: iana.StatusNoBinding},
	}

	for _, test := range tests {
		t.Run(test.messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, test.messageType)
			addIANAAddresses(msg, net.ParseIP("2001:db8::200"))
			if requiresServerID(test.messageType) {
				withHandlerServerID(handler)(msg)
			}

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireIANAStatus(t, reply, test.wantStatus)
		})
	}
}

func TestHandleRequestRenewRebindReturnStatusForMixedAddressSet(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	tests := []struct {
		messageType dhcpv6.MessageType
		wantStatus  iana.StatusCode
	}{
		{messageType: dhcpv6.MessageTypeRequest, wantStatus: iana.StatusNoAddrsAvail},
		{messageType: dhcpv6.MessageTypeRenew, wantStatus: iana.StatusNoBinding},
		{messageType: dhcpv6.MessageTypeRebind, wantStatus: iana.StatusNoBinding},
	}

	for _, test := range tests {
		t.Run(test.messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, test.messageType)
			addIANAAddresses(msg, net.ParseIP("2001:db8::100"), net.ParseIP("2001:db8::200"))
			if requiresServerID(test.messageType) {
				withHandlerServerID(handler)(msg)
			}

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireIANAStatus(t, reply, test.wantStatus)
		})
	}
}

func TestHandleRequestReturnsPerIANAResults(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeRequest)
	withHandlerServerID(handler)(msg)
	addIANA(t, msg, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))
	addIANA(t, msg, [4]byte{2, 2, 2, 2}, net.ParseIP("2001:db8::200"))

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
	requireReservationForIAID(t, reply, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))
	requireIANAStatusForIAID(t, reply, [4]byte{2, 2, 2, 2}, iana.StatusNoAddrsAvail)
}

func TestHandleRequestReturnsOneReservationForDuplicateAcceptableIANAs(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeRequest)
	withHandlerServerID(handler)(msg)
	addIANA(t, msg, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))
	addIANA(t, msg, [4]byte{2, 2, 2, 2}, net.ParseIP("2001:db8::100"))

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
	requireReservationForIAID(t, reply, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))
	requireIANAStatusForIAID(t, reply, [4]byte{2, 2, 2, 2}, iana.StatusNoAddrsAvail)
}

func TestHandleRenewRebindReturnOneReservationForDuplicateMatchingIANAs(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	for _, messageType := range []dhcpv6.MessageType{dhcpv6.MessageTypeRenew, dhcpv6.MessageTypeRebind} {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			withHandlerServerID(handler)(msg)
			addIANA(t, msg, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))
			addIANA(t, msg, [4]byte{2, 2, 2, 2}, net.ParseIP("2001:db8::100"))

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireReservationForIAID(t, reply, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))
			requireIANAStatusForIAID(t, reply, [4]byte{2, 2, 2, 2}, iana.StatusNoBinding)
		})
	}
}

func TestHandleIgnoresMessagesMissingRequiredServerID(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	tests := []dhcpv6.MessageType{
		dhcpv6.MessageTypeRequest,
		dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRelease,
		dhcpv6.MessageTypeDecline,
	}

	for _, messageType := range tests {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			dhcpv6.WithIANA(dhcpv6.OptIAAddress{
				IPv6Addr: net.ParseIP("2001:db8::100"),
			})(msg)

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestHandleIgnoresMessagesAddressedToAnotherServer(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	tests := []dhcpv6.MessageType{
		dhcpv6.MessageTypeRequest,
		dhcpv6.MessageTypeRenew,
		dhcpv6.MessageTypeRelease,
		dhcpv6.MessageTypeDecline,
	}

	for _, messageType := range tests {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			dhcpv6.WithServerID(&dhcpv6.DUIDLL{
				HWType:        iana.HWTypeEthernet,
				LinkLayerAddr: net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0x00},
			})(msg)
			if isReleaseOrDecline(messageType) {
				dhcpv6.WithIANA(dhcpv6.OptIAAddress{
					IPv6Addr: net.ParseIP("2001:db8::100"),
				})(msg)
			}

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestHandleReservationIgnoresHardwareWithNoIPv6(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	tests := map[string]*tinkerbell.Hardware{
		"missing IP": func() *tinkerbell.Hardware {
			hw := hardwareForMAC(mac, "2001:db8::100", "", true)
			hw.Spec.Interfaces[0].DHCP.IP = nil
			return hw
		}(),
		"IPv4 reservation":             hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
		"IPv4-mapped IPv6 reservation": hardwareForMAC(mac, "::ffff:192.0.2.100", "", true),
		"unspecified IPv6 reservation": hardwareForMAC(mac, "::", "", true),
		"loopback IPv6 reservation":    hardwareForMAC(mac, "::1", "", true),
		"link-local IPv6 reservation":  hardwareForMAC(mac, "fe80::100", "", true),
		"multicast IPv6 reservation":   hardwareForMAC(mac, "ff02::1", "", true),
	}

	for name, hardware := range tests {
		t.Run(name, func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{mac.String(): hardware})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
			addIANA(t, msg, [4]byte{2, 3, 4, 5})

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestHandleDerivedUsesDirectPoolWhenReservationHasNoIPv6(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	pool := netip.MustParsePrefix("2001:db8:abcd::/64")
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
	})
	handler.Derived = true
	handler.DerivedDirectAddressPool = pool
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeAdvertise)
	requireReservation(t, reply, [4]byte{2, 3, 4, 5}, net.IP(derivedAddress(pool, mac).AsSlice()), 84*time.Hour, 168*time.Hour)
}

func TestHandleDerivedIgnoresDirectRequestWithoutPool(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
	})
	handler.Derived = true
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleDerivedIgnoresUnusableDirectPools(t *testing.T) {
	for _, tc := range []struct {
		name string
		pool netip.Prefix
	}{
		{name: "zero prefix", pool: netip.MustParsePrefix("::/0")},
		{name: "link local", pool: netip.MustParsePrefix("fe80::/64")},
		{name: "multicast", pool: netip.MustParsePrefix("ff00::/8")},
		{name: "IPv4 mapped", pool: netip.MustParsePrefix("::ffff:0:0/96")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
			})
			handler.Derived = true
			handler.DerivedDirectAddressPool = tc.pool
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
			addIANA(t, msg, [4]byte{2, 3, 4, 5})

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestDerivedAddressAvoidsSubnetRouterAnycast(t *testing.T) {
	pool := netip.MustParsePrefix("2001:db8:abcd::/64")
	var zeroHash [32]byte

	got := derivedAddressFromHash(pool, zeroHash)

	if got == pool.Masked().Addr() {
		t.Fatalf("derived address used subnet-router anycast address: %s", got)
	}
	if !pool.Contains(got) {
		t.Fatalf("derived address left pool: got %s pool %s", got, pool)
	}
	if want := netip.MustParseAddr("2001:db8:abcd::1"); got != want {
		t.Fatalf("unexpected derived address: got %s want %s", got, want)
	}
}

func TestHandleDerivedUsesHardwareIPv6BeforeDerivedPool(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	handler.Derived = true
	handler.DerivedDirectAddressPool = netip.MustParsePrefix("2001:db8:abcd::/64")
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeAdvertise)
	requireReservation(t, reply, [4]byte{2, 3, 4, 5}, net.ParseIP("2001:db8::100"), 84*time.Hour, 168*time.Hour)
}

func TestHandleRapidCommitSolicitReturnsReply(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})
	dhcpv6.WithRapidCommit(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
	if reply.GetOneOption(dhcpv6.OptionRapidCommit) == nil {
		t.Fatal("expected rapid commit option")
	}
	requireReservation(t, reply, [4]byte{2, 3, 4, 5}, net.ParseIP("2001:db8::100"), 84*time.Hour, 168*time.Hour)
}

func TestHandleInformationRequestDelegatesToStateless(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
	if got := reply.Options.BootFileURL(); got != "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
	if got := reply.Options.VendorClass(clientEnterpriseNumber); len(got) != 0 {
		t.Fatalf("unexpected vendor class: %q", got)
	}
	if reply.Options.OneIANA() != nil {
		t.Fatal("information-request reply should not include IA_NA")
	}
}

func TestHandleInformationRequestRepliesWithStatelessConfigWhenNetbootDisallowed(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	hardware := hardwareForMAC(mac, "2001:db8::100", "", false)
	hardware.Spec.Interfaces[0].DHCP.NameServers = []string{"2001:db8::53"}
	hardware.Spec.Interfaces[0].DHCP.DomainName = "example.com"
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardware,
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest)
	dhcpv6.WithRequestedOptions(dhcpv6.OptionDNSRecursiveNameServer, dhcpv6.OptionDomainSearchList)(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
	if got := reply.Options.BootFileURL(); got != "" {
		t.Fatalf("expected no boot file URL, got %q", got)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::53")}, reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"example.com"}, reply.Options.DomainSearchList().Labels); diff != "" {
		t.Fatalf("unexpected domain search diff (-want +got):\n%s", diff)
	}
}

func TestHandleInformationRequestIgnoresIPv4Reservation(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleInformationRequestAddressedToAnotherServerNotDelegated(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest)
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

func TestHandleReservationPXEBootURLDoesNotSendPXEClientVendorClass(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{9, 8, 7, 6})
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeAdvertise)
	if got := reply.Options.BootFileURL(); got != "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi" {
		t.Fatalf("unexpected boot file URL: %s", got)
	}
	if diff := cmp.Diff(iana.Archs{iana.EFI_X86_64}, reply.Options.ArchTypes()); diff != "" {
		t.Fatalf("unexpected client arch types diff (-want +got):\n%s", diff)
	}
	if got := reply.Options.VendorClass(clientEnterpriseNumber); len(got) != 0 {
		t.Fatalf("unexpected vendor class: %q", got)
	}
}

func TestHandleReservationDisallowedNetbootReturnsValidNotAllowedBootURL(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", false),
	})
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{9, 8, 7, 6})
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeAdvertise)
	if got, want := reply.Options.BootFileURL(), "http://boot.example/ipxe/script/00:01:02:03:04:05/netboot-not-allowed"; got != want {
		t.Fatalf("unexpected boot file URL: got %q want %q", got, want)
	}
	requireReservation(t, reply, [4]byte{9, 8, 7, 6}, net.ParseIP("2001:db8::100"), 84*time.Hour, 168*time.Hour)
}

func TestHandleReservationDoesNotReplyWhenRequestedBootURLUnavailable(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithoutArch(t, mac, dhcpv6.MessageTypeSolicit)
	dhcpv6.WithIANA()(msg)
	requestBootURL(msg)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleReservationAppendsTraceparentToBootURL(t *testing.T) {
	const traceparent = "00-23b1e307bb35484f535a1f772c06910e-d887dc3912240434-01"
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	handler.OTELEnabled = true
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	dhcpv6.WithIANA()(msg)
	requestBootURL(msg)
	ctx := contextWithTraceparent(t, traceparent)

	handler.Handle(ctx, conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeAdvertise)
	if got, want := reply.Options.BootFileURL(), "tftp://192.0.2.1:69/00:01:02:03:04:05/ipxe.efi-"+traceparent; got != want {
		t.Fatalf("unexpected boot file URL: got %q want %q", got, want)
	}
}

func TestHandleIgnoresUnavailableReservations(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	disabled := hardwareForMAC(mac, "2001:db8::100", "", true)
	disabled.Spec.Interfaces[0].DisableDHCP = true

	tests := map[string]*mockBackend{
		"unknown hardware": {hardware: map[string]*tinkerbell.Hardware{}},
		"disabled DHCP": {hardware: map[string]*tinkerbell.Hardware{
			mac.String(): disabled,
		}},
		"IPv4 reservation": {hardware: map[string]*tinkerbell.Hardware{
			mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
		}},
	}

	for name, backend := range tests {
		t.Run(name, func(t *testing.T) {
			handler := newHandlerWithBackend(t, backend)
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
			addIANA(t, msg, [4]byte{2, 3, 4, 5})

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestHandleReleaseAndDeclineReturnReply(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	for _, messageType := range []dhcpv6.MessageType{dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeDecline} {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			withHandlerServerID(handler)(msg)
			dhcpv6.WithIANA(dhcpv6.OptIAAddress{
				IPv6Addr: net.ParseIP("2001:db8::100"),
			})(msg)

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireSuccessStatus(t, reply)
			requireIANAStatus(t, reply, iana.StatusSuccess)
		})
	}
}

func TestHandleReleaseAndDeclineReturnNoBinding(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	for _, messageType := range []dhcpv6.MessageType{dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeDecline} {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			withHandlerServerID(handler)(msg)
			dhcpv6.WithIANA(dhcpv6.OptIAAddress{
				IPv6Addr: net.ParseIP("2001:db8::200"),
			})(msg)

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireSuccessStatus(t, reply)
			requireIANAStatus(t, reply, iana.StatusNoBinding)
		})
	}
}

func TestHandleReleaseAndDeclineReturnPerIANAStatus(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	for _, messageType := range []dhcpv6.MessageType{dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeDecline} {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			withHandlerServerID(handler)(msg)
			addIANA(t, msg, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::200"))
			addIANA(t, msg, [4]byte{2, 2, 2, 2}, net.ParseIP("2001:db8::100"))

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireSuccessStatus(t, reply)
			requireIANAStatusForIAID(t, reply, [4]byte{1, 1, 1, 1}, iana.StatusNoBinding)
			requireIANAStatusForIAID(t, reply, [4]byte{2, 2, 2, 2}, iana.StatusSuccess)
		})
	}
}

func TestHandleReleaseAndDeclineReturnNoBindingForMixedAddressSet(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	for _, messageType := range []dhcpv6.MessageType{dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeDecline} {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			withHandlerServerID(handler)(msg)
			addIANAAddresses(msg, net.ParseIP("2001:db8::100"), net.ParseIP("2001:db8::200"))

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireSuccessStatus(t, reply)
			requireIANAStatus(t, reply, iana.StatusNoBinding)
		})
	}
}

func TestHandleReleaseAndDeclineReturnPerIANANoBindingWithoutReservation(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	for _, messageType := range []dhcpv6.MessageType{dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeDecline} {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandler(t, map[string]*tinkerbell.Hardware{})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			withHandlerServerID(handler)(msg)
			addIANA(t, msg, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))
			addIANA(t, msg, [4]byte{2, 2, 2, 2}, net.ParseIP("2001:db8::200"))

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
			requireIANAStatusForIAID(t, reply, [4]byte{1, 1, 1, 1}, iana.StatusNoBinding)
			requireIANAStatusForIAID(t, reply, [4]byte{2, 2, 2, 2}, iana.StatusNoBinding)
		})
	}
}

func TestHandleReleaseAndDeclineIgnoreBackendErrors(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	for _, messageType := range []dhcpv6.MessageType{dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeDecline} {
		t.Run(messageType.String(), func(t *testing.T) {
			handler := newHandlerWithBackend(t, &mockBackend{err: errors.New("backend unavailable")})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			withHandlerServerID(handler)(msg)
			addIANA(t, msg, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestHandleReleaseAndDeclineIgnoreUnusableHardware(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	for _, messageType := range []dhcpv6.MessageType{dhcpv6.MessageTypeRelease, dhcpv6.MessageTypeDecline} {
		t.Run(messageType.String(), func(t *testing.T) {
			hardware := hardwareForMAC(mac, "2001:db8::100", "", true)
			hardware.Spec.Interfaces[0].Netboot = nil
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardware,
			})
			conn := &recordingPacketConn{}
			msg := messageWithMAC(t, mac, messageType)
			withHandlerServerID(handler)(msg)
			addIANA(t, msg, [4]byte{1, 1, 1, 1}, net.ParseIP("2001:db8::100"))

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestHandleReleaseFromTcpdumpReturnsReply(t *testing.T) {
	mac := net.HardwareAddr{0x08, 0x00, 0x27, 0x9e, 0xf5, 0x3a}
	serverID := &dhcpv6.DUIDEN{
		EnterpriseNumber: 123,
		EnterpriseIdentifier: []byte{
			0x8b, 0x95, 0x26, 0x11, 0xa2, 0x7a, 0x87, 0xa3,
			0x92, 0x43, 0xff, 0x17, 0xad, 0x2a, 0xe2, 0xfc,
		},
	}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "fd8a:3f4b:7c91:1::99", "", true),
	})
	handler.ServerID = serverID
	conn := &recordingPacketConn{}
	msg, err := dhcpv6.NewMessage(
		dhcpv6.WithClientID(&dhcpv6.DUIDLLT{
			HWType:        iana.HWTypeEthernet,
			Time:          821816327,
			LinkLayerAddr: mac,
		}),
		dhcpv6.WithServerID(serverID),
	)
	if err != nil {
		t.Fatal(err)
	}
	msg.MessageType = dhcpv6.MessageTypeRelease
	msg.AddOption(dhcpv6.OptElapsedTime(19 * time.Second))
	msg.AddOption(&dhcpv6.OptIANA{
		IaId: [4]byte{0xff, 0x77, 0xbb, 0xfd},
		T1:   (1<<32 - 1) * time.Second,
		T2:   (1<<32 - 1) * time.Second,
		Options: dhcpv6.IdentityOptions{Options: []dhcpv6.Option{
			&dhcpv6.OptIAAddress{
				IPv6Addr:          net.ParseIP("fd8a:3f4b:7c91:1::99"),
				PreferredLifetime: 12 * time.Hour,
				ValidLifetime:     24 * time.Hour,
			},
		}},
	})

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::a00:27ff:fe9e:f53a"), Port: dhcpv6.DefaultClientPort}, msg)

	reply := requireSingleMessage(t, conn, dhcpv6.MessageTypeReply)
	if !reply.Options.ServerID().Equal(serverID) {
		t.Fatalf("unexpected server ID: got %s want %s", reply.Options.ServerID(), serverID)
	}
	requireIANAStatus(t, reply, iana.StatusSuccess)
	if got := reply.Options.OneIANA().IaId; got != [4]byte{0xff, 0x77, 0xbb, 0xfd} {
		t.Fatalf("unexpected IAID: got %#v", got)
	}
}

func TestHandleRelaySolicitReplyWrapped(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})
	relay := relayRequest(t, msg, mac)
	peer := &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}

	handler.Handle(context.Background(), conn, peer, relay)

	reply := requireSingleRelayReply(t, conn)
	if conn.peers[0] != peer {
		t.Fatalf("unexpected peer: got %v want %v", conn.peers[0], peer)
	}
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	if inner.Type() != dhcpv6.MessageTypeAdvertise {
		t.Fatalf("unexpected inner message type: %s", inner.Type())
	}
	requireReservation(t, inner, [4]byte{2, 3, 4, 5}, net.ParseIP("2001:db8::100"), 84*time.Hour, 168*time.Hour)
}

func TestHandleRelayRequestReplyWrapped(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeRequest)
	addIANAAddresses(msg, net.ParseIP("2001:db8::100"))
	withHandlerServerID(handler)(msg)
	relay := relayRequest(t, msg, mac)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	reply := requireSingleRelayReply(t, conn)
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	if inner.Type() != dhcpv6.MessageTypeReply {
		t.Fatalf("unexpected inner message type: %s", inner.Type())
	}
	requireReservation(t, inner, [4]byte{9, 8, 7, 6}, net.ParseIP("2001:db8::100"), 84*time.Hour, 168*time.Hour)
}

func TestHandleDerivedRelayUsesLinkAddressPrefix(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	linkAddr := netip.MustParseAddr("2001:db8:abcd:1234::1")
	pool := netip.PrefixFrom(linkAddr, 64)
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
	})
	handler.Derived = true
	handler.DerivedRelayAddressPrefix = 64
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})
	relay := relayRequestWithLinkAddress(t, msg, mac, linkAddr)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	reply := requireSingleRelayReply(t, conn)
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	requireReservation(t, inner, [4]byte{2, 3, 4, 5}, net.IP(derivedAddress(pool, mac).AsSlice()), 84*time.Hour, 168*time.Hour)
}

func TestHandleDerivedRelayHonorsCustomPrefix(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	linkAddr := netip.MustParseAddr("2001:db8:abcd:1234::1")
	pool := netip.PrefixFrom(linkAddr, 56)
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
	})
	handler.Derived = true
	handler.DerivedRelayAddressPrefix = 56
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})
	relay := relayRequestWithLinkAddress(t, msg, mac, linkAddr)

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	reply := requireSingleRelayReply(t, conn)
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	requireReservation(t, inner, [4]byte{2, 3, 4, 5}, net.IP(derivedAddress(pool, mac).AsSlice()), 84*time.Hour, 168*time.Hour)
}

func TestHandleDerivedRelayIgnoresNarrowPrefix(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
	})
	handler.Derived = true
	handler.DerivedRelayAddressPrefix = 65
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})
	relay := relayRequestWithLinkAddress(t, msg, mac, netip.MustParseAddr("2001:db8:abcd:1234::1"))

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleDerivedRelayIgnoresZeroPrefix(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
	})
	handler.Derived = true
	handler.DerivedRelayAddressPrefix = 0
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})
	relay := relayRequestWithLinkAddress(t, msg, mac, netip.MustParseAddr("2001:db8:abcd:1234::1"))

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleDerivedRelayIgnoresUnusableLinkAddress(t *testing.T) {
	for _, tc := range []struct {
		name     string
		linkAddr netip.Addr
	}{
		{name: "unspecified", linkAddr: netip.IPv6Unspecified()},
		{name: "link local", linkAddr: netip.MustParseAddr("fe80::1")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
			handler := newHandler(t, map[string]*tinkerbell.Hardware{
				mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
			})
			handler.Derived = true
			handler.DerivedRelayAddressPrefix = 64
			conn := &recordingPacketConn{}
			msg := messageWithDUIDEN(t, dhcpv6.MessageTypeSolicit)
			addIANA(t, msg, [4]byte{2, 3, 4, 5})
			relay := relayRequestWithLinkAddress(t, msg, mac, tc.linkAddr)

			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

			if len(conn.writes) != 0 {
				t.Fatalf("expected no reply, got %d", len(conn.writes))
			}
		})
	}
}

func TestHandleDerivedIgnoresUnknownHardware(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{})
	handler.Derived = true
	handler.DerivedDirectAddressPool = netip.MustParsePrefix("2001:db8:abcd::/64")
	conn := &recordingPacketConn{}
	msg := messageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, msg)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply, got %d", len(conn.writes))
	}
}

func TestHandleDerivedNestedRelayUsesInnermostLinkAddressPrefix(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	innerLinkAddr := netip.MustParseAddr("2001:db8:1111:2222::1")
	outerLinkAddr := netip.MustParseAddr("2001:db8:aaaa:bbbb::1")
	pool := netip.PrefixFrom(innerLinkAddr, 64)
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "192.0.2.100", "255.255.255.0", true),
	})
	handler.Derived = true
	handler.DerivedRelayAddressPrefix = 64
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeSolicit)
	addIANA(t, msg, [4]byte{2, 3, 4, 5})
	innerRelay := relayRequestWithLinkAddress(t, msg, mac, innerLinkAddr)
	relay, err := dhcpv6.EncapsulateRelay(innerRelay, dhcpv6.MessageTypeRelayForward, net.IP(outerLinkAddr.AsSlice()), net.ParseIP("fe80::beef"))
	if err != nil {
		t.Fatal(err)
	}

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	reply := requireSingleRelayReply(t, conn)
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	requireReservation(t, inner, [4]byte{2, 3, 4, 5}, net.IP(derivedAddress(pool, mac).AsSlice()), 84*time.Hour, 168*time.Hour)
}

func TestHandleNestedRelayPreservesRelayChain(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	handler := newHandler(t, map[string]*tinkerbell.Hardware{
		mac.String(): hardwareForMAC(mac, "2001:db8::100", "", true),
	})
	conn := &recordingPacketConn{}
	msg := messageWithDUIDEN(t, dhcpv6.MessageTypeRequest)
	addIANAAddresses(msg, net.ParseIP("2001:db8::100"))
	withHandlerServerID(handler)(msg)
	innerRelay := relayRequest(t, msg, mac)
	relay, err := dhcpv6.EncapsulateRelay(innerRelay, dhcpv6.MessageTypeRelayForward, net.ParseIP("2001:db8::2"), net.ParseIP("fe80::beef"))
	if err != nil {
		t.Fatal(err)
	}

	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

	reply := requireSingleRelayReply(t, conn)
	decap, err := dhcpv6.DecapsulateRelay(reply)
	if err != nil {
		t.Fatal(err)
	}
	if !decap.IsRelay() {
		t.Fatalf("expected nested relay reply, got %T", decap)
	}
	inner, err := reply.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	requireReservation(t, inner, [4]byte{9, 8, 7, 6}, net.ParseIP("2001:db8::100"), 84*time.Hour, 168*time.Hour)
}

func newHandler(t *testing.T, hardware map[string]*tinkerbell.Hardware) *Handler {
	t.Helper()
	return newHandlerWithBackend(t, &mockBackend{hardware: hardware})
}

func newHandlerWithBackend(t *testing.T, backend *mockBackend) *Handler {
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
				u, _ := url.Parse("http://boot.example/ipxe/script/" + mac.String() + "/auto6.ipxe")
				return u
			},
			Enabled:             true,
			InjectMacAddrFormat: constant.MacAddrFormatColon,
		},
	}
}

func messageWithMAC(t *testing.T, mac net.HardwareAddr, messageType dhcpv6.MessageType) *dhcpv6.Message {
	t.Helper()
	msg, err := dhcpv6.NewMessage(dhcpv6.WithClientID(&dhcpv6.DUIDLL{
		HWType:        iana.HWTypeEthernet,
		LinkLayerAddr: mac,
	}))
	if err != nil {
		t.Fatal(err)
	}
	msg.MessageType = messageType
	dhcpv6.WithArchType(iana.EFI_X86_64)(msg)
	return msg
}

func messageWithoutArch(t *testing.T, mac net.HardwareAddr, messageType dhcpv6.MessageType) *dhcpv6.Message {
	t.Helper()
	msg, err := dhcpv6.NewMessage(dhcpv6.WithClientID(&dhcpv6.DUIDLL{
		HWType:        iana.HWTypeEthernet,
		LinkLayerAddr: mac,
	}))
	if err != nil {
		t.Fatal(err)
	}
	msg.MessageType = messageType
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
	dhcpv6.WithRequestedOptions(dhcpv6.OptionBootfileURL)(msg)
}

func withHandlerServerID(handler *Handler) dhcpv6.Modifier {
	return dhcpv6.WithServerID(handler.ServerID)
}

func addIANAAddresses(msg *dhcpv6.Message, addresses ...net.IP) {
	options := make([]dhcpv6.Option, 0, len(addresses))
	for _, address := range addresses {
		options = append(options, &dhcpv6.OptIAAddress{IPv6Addr: address})
	}
	msg.AddOption(&dhcpv6.OptIANA{
		IaId:    [4]byte{9, 8, 7, 6},
		Options: dhcpv6.IdentityOptions{Options: options},
	})
}

func addIANA(t *testing.T, msg *dhcpv6.Message, iaid [4]byte, addresses ...net.IP) {
	t.Helper()
	options := make([]dhcpv6.Option, 0, len(addresses))
	for _, address := range addresses {
		options = append(options, &dhcpv6.OptIAAddress{IPv6Addr: address})
	}
	msg.AddOption(&dhcpv6.OptIANA{
		IaId:    iaid,
		Options: dhcpv6.IdentityOptions{Options: options},
	})
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
	return relayRequestWithLinkAddress(t, msg, mac, netip.MustParseAddr("2001:db8::1"))
}

func relayRequestWithLinkAddress(t *testing.T, msg *dhcpv6.Message, mac net.HardwareAddr, linkAddr netip.Addr) *dhcpv6.RelayMessage {
	t.Helper()
	relay, err := dhcpv6.EncapsulateRelay(msg, dhcpv6.MessageTypeRelayForward, net.IP(linkAddr.AsSlice()), net.ParseIP("fe80::abcd"))
	if err != nil {
		t.Fatal(err)
	}
	if mac != nil {
		relay.AddOption(dhcpv6.OptClientLinkLayerAddress(iana.HWTypeEthernet, mac))
	}
	return relay
}

func requireSingleMessage(t *testing.T, conn *recordingPacketConn, messageType dhcpv6.MessageType) *dhcpv6.Message {
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
	if msg.Type() != messageType {
		t.Fatalf("unexpected message type: got %s want %s", msg.Type(), messageType)
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

func requireReservation(t *testing.T, msg *dhcpv6.Message, iaid [4]byte, ip net.IP, preferred, valid time.Duration) {
	t.Helper()
	ia := msg.Options.OneIANA()
	if ia == nil {
		t.Fatal("expected IA_NA")
	}
	if ia.IaId != iaid {
		t.Fatalf("unexpected IAID: got %#v want %#v", ia.IaId, iaid)
	}
	addr := ia.Options.OneAddress()
	if addr == nil {
		t.Fatal("expected IAAddr")
	}
	if !addr.IPv6Addr.Equal(ip) {
		t.Fatalf("unexpected reservation IP: got %s want %s", addr.IPv6Addr, ip)
	}
	if addr.PreferredLifetime != preferred {
		t.Fatalf("unexpected preferred lifetime: got %v want %v", addr.PreferredLifetime, preferred)
	}
	if addr.ValidLifetime != valid {
		t.Fatalf("unexpected valid lifetime: got %v want %v", addr.ValidLifetime, valid)
	}
}

func requireIANATimers(t *testing.T, msg *dhcpv6.Message, t1, t2 time.Duration) {
	t.Helper()
	ia := msg.Options.OneIANA()
	if ia == nil {
		t.Fatal("expected IA_NA")
	}
	if ia.T1 != t1 {
		t.Fatalf("unexpected T1: got %v want %v", ia.T1, t1)
	}
	if ia.T2 != t2 {
		t.Fatalf("unexpected T2: got %v want %v", ia.T2, t2)
	}
}

func requireReservationForIAID(t *testing.T, msg *dhcpv6.Message, iaid [4]byte, ip net.IP) {
	t.Helper()
	ia := ianaByIAID(t, msg, iaid)
	addr := ia.Options.OneAddress()
	if addr == nil {
		t.Fatal("expected IAAddr")
	}
	if !addr.IPv6Addr.Equal(ip) {
		t.Fatalf("unexpected reservation IP: got %s want %s", addr.IPv6Addr, ip)
	}
}

func requireSuccessStatus(t *testing.T, msg *dhcpv6.Message) {
	t.Helper()
	status := msg.Options.Status()
	if status == nil {
		t.Fatal("expected status code option")
	}
	if status.StatusCode != iana.StatusSuccess {
		t.Fatalf("unexpected status: got %s want %s", status.StatusCode, iana.StatusSuccess)
	}
}

func requireIANAStatus(t *testing.T, msg *dhcpv6.Message, want iana.StatusCode) {
	t.Helper()
	ia := msg.Options.OneIANA()
	if ia == nil {
		t.Fatal("expected IA_NA")
	}
	status := ia.Options.Status()
	if status == nil {
		t.Fatal("expected IA_NA status")
	}
	if status.StatusCode != want {
		t.Fatalf("unexpected IA_NA status: got %s want %s", status.StatusCode, want)
	}
}

func requireIANAStatusForIAID(t *testing.T, msg *dhcpv6.Message, iaid [4]byte, want iana.StatusCode) {
	t.Helper()
	ia := ianaByIAID(t, msg, iaid)
	status := ia.Options.Status()
	if status == nil {
		t.Fatal("expected IA_NA status")
	}
	if status.StatusCode != want {
		t.Fatalf("unexpected IA_NA status: got %s want %s", status.StatusCode, want)
	}
}

func ianaByIAID(t *testing.T, msg *dhcpv6.Message, iaid [4]byte) *dhcpv6.OptIANA {
	t.Helper()
	for _, ia := range msg.Options.IANA() {
		if ia.IaId == iaid {
			return ia
		}
	}
	t.Fatalf("expected IA_NA with IAID %#v", iaid)
	return nil
}

func hardwareForMAC(mac net.HardwareAddr, ip, netmask string, allowNetboot bool) *tinkerbell.Hardware {
	return &tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{
			Interfaces: []tinkerbell.Interface{
				{
					DHCP: &tinkerbell.DHCP{
						MAC: mac.String(),
						IP: &tinkerbell.IP{
							Address: ip,
							Netmask: netmask,
						},
					},
					Netboot: &tinkerbell.Netboot{
						AllowPXE: &allowNetboot,
					},
				},
			},
		},
	}
}
