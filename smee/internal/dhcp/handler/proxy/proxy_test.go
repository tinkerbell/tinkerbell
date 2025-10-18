package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/smee/internal/dhcp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/nettest"
)

var errBackend = errors.New("backend error")

// mockBackend implements BackendReader for testing.
type mockBackend struct {
	allowNetboot bool
	iPXEBinary   string
	err          error
}

func (m *mockBackend) GetByMac(_ context.Context, _ net.HardwareAddr) (*data.DHCP, *data.Netboot, error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	return &data.DHCP{}, &data.Netboot{AllowNetboot: m.allowNetboot, IPXEBinary: m.iPXEBinary}, nil
}

func (m *mockBackend) GetByIP(_ context.Context, _ net.IP) (*data.DHCP, *data.Netboot, error) {
	return nil, nil, errors.New("not implemented")
}

func TestHandle(t *testing.T) {
	lo, _ := net.InterfaceByName("lo")
	ip := netip.MustParseAddr("127.0.0.1")
	binServerTFTP := netip.AddrPortFrom(ip, 69)
	binServerHTTP, _ := url.Parse("http://localhost:8080")
	ipxeScript := func(*dhcpv4.DHCPv4) *url.URL { u, _ := url.Parse("http://localhost/script.ipxe"); return u }

	tests := map[string]struct {
		handler Handler
		pkt     *dhcpv4.DHCPv4
		peer    net.Addr
		md      *dhcp.Metadata
		want    *dhcpv4.DHCPv4
		wantErr bool
	}{
		"nil packet": {
			handler: Handler{Log: logr.Discard(), Backend: &mockBackend{}},
			pkt:     nil,
			peer:    &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68},
			md:      &dhcp.Metadata{IfName: lo.Name, IfIndex: lo.Index},
			want:    nil,
			wantErr: false,
		},
		"not netboot client": {
			handler: Handler{
				Log:     logr.Discard(),
				Backend: &mockBackend{},
				Netboot: Netboot{Enabled: true},
			},
			pkt: &dhcpv4.DHCPv4{
				OpCode:       dhcpv4.OpcodeBootRequest,
				ClientHWAddr: []byte{1, 2, 3, 4, 5, 6},
				Options:      dhcpv4.OptionsFromList(dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover)),
			},
			peer:    &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68},
			md:      &dhcp.Metadata{IfName: lo.Name, IfIndex: lo.Index},
			want:    nil,
			wantErr: false,
		},
		"backend error": {
			handler: Handler{
				Log:     logr.Discard(),
				Backend: &mockBackend{err: errBackend},
				Netboot: Netboot{Enabled: true, IPXEBinServerTFTP: binServerTFTP, IPXEBinServerHTTP: binServerHTTP, IPXEScriptURL: ipxeScript},
				IPAddr:  ip,
			},
			pkt: &dhcpv4.DHCPv4{
				OpCode:       dhcpv4.OpcodeBootRequest,
				ClientHWAddr: []byte{1, 2, 3, 4, 5, 6},
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover),
					dhcpv4.OptClassIdentifier("PXEClient:Arch:00000:UNDI:002001"),
					dhcpv4.OptClientArch(9), // EFI_X86_64
					dhcpv4.OptGeneric(dhcpv4.OptionClientNetworkInterfaceIdentifier, []byte{1, 2, 3, 4}),
					dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
				),
			},
			peer:    &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68},
			md:      &dhcp.Metadata{IfName: lo.Name, IfIndex: lo.Index},
			want:    nil,
			wantErr: true,
		},
		"netboot not allowed": {
			handler: Handler{
				Log:     logr.Discard(),
				Backend: &mockBackend{allowNetboot: false},
				Netboot: Netboot{Enabled: true, IPXEBinServerTFTP: binServerTFTP, IPXEBinServerHTTP: binServerHTTP, IPXEScriptURL: ipxeScript},
				IPAddr:  ip,
			},
			pkt: &dhcpv4.DHCPv4{
				OpCode:       dhcpv4.OpcodeBootRequest,
				ClientHWAddr: []byte{1, 2, 3, 4, 5, 6},
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover),
					dhcpv4.OptClassIdentifier("PXEClient:Arch:00000:UNDI:002001"),
					dhcpv4.OptClientArch(9), // EFI_X86_64
					dhcpv4.OptGeneric(dhcpv4.OptionClientNetworkInterfaceIdentifier, []byte{1, 2, 3, 4}),
					dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
				),
			},
			peer: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68},
			md:   &dhcp.Metadata{IfName: lo.Name, IfIndex: lo.Index},
			want: &dhcpv4.DHCPv4{
				OpCode:         dhcpv4.OpcodeBootReply,
				ClientHWAddr:   []byte{1, 2, 3, 4, 5, 6},
				ClientIPAddr:   []byte{0, 0, 0, 0},
				YourIPAddr:     []byte{0, 0, 0, 0},
				ServerIPAddr:   []byte{127, 0, 0, 1},
				GatewayIPAddr:  []byte{0, 0, 0, 0},
				ServerHostName: "127.0.0.1",
				BootFileName:   fmt.Sprintf("/%s/netboot-not-allowed", net.HardwareAddr([]byte{1, 2, 3, 4, 5, 6})),
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer),
					dhcpv4.OptServerIdentifier(net.IP{127, 0, 0, 1}),
					dhcpv4.OptClassIdentifier("PXEClient"),
					dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
					dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, dhcpv4.Options{6: []byte{8}}.ToBytes()),
				),
			},
			wantErr: false,
		},
		"valid netboot client discover": {
			handler: Handler{
				Log:     logr.Discard(),
				Backend: &mockBackend{allowNetboot: true},
				Netboot: Netboot{Enabled: true, IPXEBinServerTFTP: binServerTFTP, IPXEBinServerHTTP: binServerHTTP, IPXEScriptURL: ipxeScript},
				IPAddr:  ip,
			},
			pkt: &dhcpv4.DHCPv4{
				OpCode:       dhcpv4.OpcodeBootRequest,
				ClientHWAddr: []byte{1, 2, 3, 4, 5, 6},
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptMessageType(dhcpv4.MessageTypeDiscover),
					dhcpv4.OptClassIdentifier("PXEClient:Arch:00000:UNDI:002001"),
					dhcpv4.OptClientArch(9), // EFI_X86_64
					dhcpv4.OptGeneric(dhcpv4.OptionClientNetworkInterfaceIdentifier, []byte{1, 2, 3, 4}),
					dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
					dhcpv4.OptUserClass("Tinkerbell"),
				),
			},
			peer: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68},
			md:   &dhcp.Metadata{IfName: lo.Name, IfIndex: lo.Index},
			want: &dhcpv4.DHCPv4{
				OpCode:         dhcpv4.OpcodeBootReply,
				ClientHWAddr:   []byte{1, 2, 3, 4, 5, 6},
				ClientIPAddr:   []byte{0, 0, 0, 0},
				YourIPAddr:     []byte{0, 0, 0, 0},
				ServerIPAddr:   []byte{127, 0, 0, 1},
				GatewayIPAddr:  []byte{0, 0, 0, 0},
				ServerHostName: "127.0.0.1",
				BootFileName:   "http://localhost/script.ipxe",
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer),
					dhcpv4.OptServerIdentifier(net.IP{127, 0, 0, 1}),
					dhcpv4.OptClassIdentifier("PXEClient"),
					dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
					dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, dhcpv4.Options{6: []byte{8}}.ToBytes()),
				),
			},
			wantErr: false,
		},
		"valid netboot client request": {
			handler: Handler{
				Log:     logr.Discard(),
				Backend: &mockBackend{allowNetboot: true},
				Netboot: Netboot{Enabled: true, IPXEBinServerTFTP: binServerTFTP, IPXEBinServerHTTP: binServerHTTP, IPXEScriptURL: ipxeScript},
				IPAddr:  ip,
			},
			pkt: &dhcpv4.DHCPv4{
				OpCode:       dhcpv4.OpcodeBootRequest,
				ClientHWAddr: []byte{1, 2, 3, 4, 5, 6},
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptMessageType(dhcpv4.MessageTypeRequest),
					dhcpv4.OptClassIdentifier("PXEClient:Arch:00000:UNDI:002001"),
					dhcpv4.OptClientArch(9), // EFI_X86_64
					dhcpv4.OptGeneric(dhcpv4.OptionClientNetworkInterfaceIdentifier, []byte{1, 2, 3, 4}),
					dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
				),
			},
			peer: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68},
			md:   &dhcp.Metadata{IfName: lo.Name, IfIndex: lo.Index},
			want: &dhcpv4.DHCPv4{
				OpCode:         dhcpv4.OpcodeBootReply,
				ClientHWAddr:   []byte{1, 2, 3, 4, 5, 6},
				ClientIPAddr:   []byte{0, 0, 0, 0},
				YourIPAddr:     []byte{0, 0, 0, 0},
				ServerIPAddr:   []byte{127, 0, 0, 1},
				GatewayIPAddr:  []byte{0, 0, 0, 0},
				ServerHostName: "127.0.0.1",
				BootFileName:   "ipxe.efi",
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptMessageType(dhcpv4.MessageTypeAck),
					dhcpv4.OptServerIdentifier(net.IP{127, 0, 0, 1}),
					dhcpv4.OptClassIdentifier("PXEClient"),
					dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
					dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, dhcpv4.Options{6: []byte{8}}.ToBytes()),
				),
			},
			wantErr: false,
		},
		"valid netboot client request custom ipxe binary": {
			handler: Handler{
				Log:     logr.Discard(),
				Backend: &mockBackend{allowNetboot: true, iPXEBinary: "snp-x86_64.efi"},
				Netboot: Netboot{Enabled: true, IPXEBinServerTFTP: binServerTFTP, IPXEBinServerHTTP: binServerHTTP, IPXEScriptURL: ipxeScript},
				IPAddr:  ip,
			},
			pkt: &dhcpv4.DHCPv4{
				OpCode:       dhcpv4.OpcodeBootRequest,
				ClientHWAddr: []byte{1, 2, 3, 4, 5, 6},
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptMessageType(dhcpv4.MessageTypeRequest),
					dhcpv4.OptClassIdentifier("PXEClient:Arch:00000:UNDI:002001"),
					dhcpv4.OptClientArch(9), // EFI_X86_64
					dhcpv4.OptGeneric(dhcpv4.OptionClientNetworkInterfaceIdentifier, []byte{1, 2, 3, 4}),
					dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
				),
			},
			peer: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 68},
			md:   &dhcp.Metadata{IfName: lo.Name, IfIndex: lo.Index},
			want: &dhcpv4.DHCPv4{
				OpCode:         dhcpv4.OpcodeBootReply,
				ClientHWAddr:   []byte{1, 2, 3, 4, 5, 6},
				ClientIPAddr:   []byte{0, 0, 0, 0},
				YourIPAddr:     []byte{0, 0, 0, 0},
				ServerIPAddr:   []byte{127, 0, 0, 1},
				GatewayIPAddr:  []byte{0, 0, 0, 0},
				ServerHostName: "127.0.0.1",
				BootFileName:   "snp-x86_64.efi",
				Options: dhcpv4.OptionsFromList(
					dhcpv4.OptMessageType(dhcpv4.MessageTypeAck),
					dhcpv4.OptServerIdentifier(net.IP{127, 0, 0, 1}),
					dhcpv4.OptClassIdentifier("PXEClient"),
					dhcpv4.OptGeneric(dhcpv4.OptionClientMachineIdentifier, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
					dhcpv4.OptGeneric(dhcpv4.OptionVendorSpecificInformation, dhcpv4.Options{6: []byte{8}}.ToBytes()),
				),
			},
			wantErr: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Create the test server
			conn, err := nettest.NewLocalPacketListener("udp")
			if err != nil {
				t.Fatal("failed to create test server", err)
			}
			defer conn.Close()

			// Create a client to listen for responses
			pc, err := net.ListenPacket("udp4", ":0")
			if err != nil {
				t.Fatal("failed to create test client", err)
			}
			defer pc.Close()

			// Set up the peer based on the test client
			peer := tt.peer
			if p, ok := peer.(*net.UDPAddr); ok && p != nil {
				// Update the port to match our test client
				peer = &net.UDPAddr{IP: p.IP, Port: pc.LocalAddr().(*net.UDPAddr).Port}
			}

			// Set up the connection
			con := ipv4.NewPacketConn(conn)
			if err := con.SetControlMessage(ipv4.FlagInterface, true); err != nil {
				t.Fatal("failed to set control message", err)
			}

			// Send the packet to the handler
			pkt := dhcp.Packet{Peer: peer, Pkt: tt.pkt, Md: tt.md}
			tt.handler.Handle(context.Background(), con, pkt)

			// Get the response (or timeout if none is expected)
			msg, err := clientResponse(pc)

			// Validate the error
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil && tt.want != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// If we expect a response, compare it with what we got
			if tt.want != nil {
				if msg == nil {
					t.Fatal("expected a response, got nil")
				}

				// Compare the DHCP packets
				if diff := cmp.Diff(msg, tt.want, cmpopts.IgnoreUnexported(dhcpv4.DHCPv4{})); diff != "" {
					t.Fatalf("DHCP response doesn't match expected:\n%s", diff)
				}
			} else if msg != nil {
				t.Fatal("expected no response, but got one")
			}
		})
	}
}

// clientResponse attempts to read a DHCP response from the given connection.
func clientResponse(pc net.PacketConn) (*dhcpv4.DHCPv4, error) {
	buf := make([]byte, 1024)
	if err := pc.SetReadDeadline(time.Now().Add(time.Millisecond * 100)); err != nil {
		return nil, err
	}
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		return nil, err
	}
	msg, err := dhcpv4.FromBytes(buf[:n])
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func TestSetMessageType(t *testing.T) {
	tests := map[string]struct {
		msgType dhcpv4.MessageType
		want    dhcpv4.MessageType
		wantErr bool
	}{
		"discover": {msgType: dhcpv4.MessageTypeDiscover, want: dhcpv4.MessageTypeOffer},
		"request":  {msgType: dhcpv4.MessageTypeRequest, want: dhcpv4.MessageTypeAck},
		"other":    {msgType: dhcpv4.MessageTypeRelease, wantErr: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			reply := &dhcpv4.DHCPv4{}
			err := setMessageType(reply, tt.msgType)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantErr && reply.MessageType() != tt.want {
				t.Fatalf("got %v, want %v", reply.MessageType(), tt.want)
			}
		})
	}
}

func TestReplyDestination(t *testing.T) {
	peer := &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 68}
	giaddr := net.IPv4(5, 6, 7, 8)
	tests := map[string]struct {
		giaddr net.IP
		want   net.Addr
	}{
		"giaddr set":   {giaddr: giaddr, want: &net.UDPAddr{IP: giaddr, Port: dhcpv4.ServerPort}},
		"giaddr unset": {giaddr: net.IPv4zero, want: peer},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := replyDestination(peer, tt.giaddr)
			if udp, ok := got.(*net.UDPAddr); ok {
				if !udp.IP.Equal(tt.want.(*net.UDPAddr).IP) || udp.Port != tt.want.(*net.UDPAddr).Port {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			} else if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeToAttributes(t *testing.T) {
	h := &Handler{Log: logr.Discard()}
	pkt := &dhcpv4.DHCPv4{BootFileName: "test.efi"}
	kvs := h.encodeToAttributes(pkt, "test")
	if len(kvs) == 0 {
		t.Fatal("expected attributes, got none")
	}
}

func TestIgnorePacketError(t *testing.T) {
	err := IgnorePacketError{
		PacketType: dhcpv4.MessageTypeRelease,
		Details:    "test details",
	}

	expected := "Ignoring packet: message type RELEASE: details test details"
	if err.Error() != expected {
		t.Errorf("expected error string %q, got %q", expected, err.Error())
	}
}
