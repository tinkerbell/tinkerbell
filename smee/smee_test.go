package smee

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/smee/internal/metric"
)

type dhcpv6TestBackend struct {
	hardware *tinkerbell.Hardware
	err      error
}

func (b dhcpv6TestBackend) FilterHardware(context.Context, data.HardwareFilter) (*tinkerbell.Hardware, error) {
	return b.hardware, b.err
}

type dhcpv6HardwareNotFoundError struct{}

func (dhcpv6HardwareNotFoundError) Error() string  { return "hardware not found" }
func (dhcpv6HardwareNotFoundError) NotFound() bool { return true }

type dhcpv6RecordingPacketConn struct {
	writes [][]byte
}

func (c *dhcpv6RecordingPacketConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, nil }
func (c *dhcpv6RecordingPacketConn) WriteTo(payload []byte, _ net.Addr) (int, error) {
	c.writes = append(c.writes, append([]byte(nil), payload...))
	return len(payload), nil
}
func (c *dhcpv6RecordingPacketConn) Close() error                     { return nil }
func (c *dhcpv6RecordingPacketConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *dhcpv6RecordingPacketConn) SetDeadline(time.Time) error      { return nil }
func (c *dhcpv6RecordingPacketConn) SetReadDeadline(time.Time) error  { return nil }
func (c *dhcpv6RecordingPacketConn) SetWriteDeadline(time.Time) error { return nil }

func newDHCPv6HandlerTestConfig() *Config {
	config := NewConfig(Config{})
	config.Backend = dhcpv6TestBackend{}
	return config
}

func dhcpv6MessageWithMAC(t *testing.T, mac net.HardwareAddr, messageType dhcpv6.MessageType) *dhcpv6.Message {
	t.Helper()
	message, err := dhcpv6.NewMessage(dhcpv6.WithClientID(&dhcpv6.DUIDLL{
		HWType:        iana.HWTypeEthernet,
		LinkLayerAddr: mac,
	}))
	if err != nil {
		t.Fatal(err)
	}
	message.MessageType = messageType
	dhcpv6.WithArchType(iana.EFI_X86_64)(message)
	return message
}

func dhcpv6HardwareForMAC(mac net.HardwareAddr, ip, netmask string) *tinkerbell.Hardware {
	allowNetboot := true
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
					Netboot: &tinkerbell.Netboot{AllowPXE: &allowNetboot},
				},
			},
		},
	}
}

func requireDHCPv6Message(t *testing.T, conn *dhcpv6RecordingPacketConn, messageType dhcpv6.MessageType) *dhcpv6.Message {
	t.Helper()
	if len(conn.writes) != 1 {
		t.Fatalf("expected one DHCPv6 response, got %d", len(conn.writes))
	}
	packet, err := dhcpv6.FromBytes(conn.writes[0])
	if err != nil {
		t.Fatal(err)
	}
	message, ok := packet.(*dhcpv6.Message)
	if !ok {
		t.Fatalf("expected DHCPv6 message response, got %T", packet)
	}
	if message.Type() != messageType {
		t.Fatalf("unexpected DHCPv6 response type: got %s want %s", message.Type(), messageType)
	}
	return message
}

func requireDHCPv6RelayMessage(t *testing.T, conn *dhcpv6RecordingPacketConn, messageType dhcpv6.MessageType) *dhcpv6.Message {
	t.Helper()
	if len(conn.writes) != 1 {
		t.Fatalf("expected one DHCPv6 response, got %d", len(conn.writes))
	}
	packet, err := dhcpv6.FromBytes(conn.writes[0])
	if err != nil {
		t.Fatal(err)
	}
	relay, ok := packet.(*dhcpv6.RelayMessage)
	if !ok {
		t.Fatalf("expected DHCPv6 relay response, got %T", packet)
	}
	if relay.Type() != dhcpv6.MessageTypeRelayReply {
		t.Fatalf("unexpected DHCPv6 relay response type: got %s want %s", relay.Type(), dhcpv6.MessageTypeRelayReply)
	}
	message, err := relay.GetInnerMessage()
	if err != nil {
		t.Fatal(err)
	}
	if message.Type() != messageType {
		t.Fatalf("unexpected inner DHCPv6 response type: got %s want %s", message.Type(), messageType)
	}
	return message
}

func dhcpv6AddressForIAID(t *testing.T, message *dhcpv6.Message, iaid [4]byte) (netip.Addr, bool) {
	t.Helper()
	for _, ianaOption := range message.Options.IANA() {
		if ianaOption.IaId != iaid {
			continue
		}
		addressOption := ianaOption.Options.OneAddress()
		if addressOption == nil {
			return netip.Addr{}, false
		}
		address, ok := netip.AddrFromSlice(addressOption.IPv6Addr)
		if !ok {
			t.Fatalf("invalid DHCPv6 IA address: %s", addressOption.IPv6Addr)
		}
		return address, true
	}
	t.Fatalf("expected DHCPv6 response to contain IAID %#v", iaid)
	return netip.Addr{}, false
}

// TestConfig_syslogHost verifies that a configured SyslogFQDN takes precedence over the DHCP
// syslog IP, and that the IP is used as a fallback when no FQDN is set. Covers #533.
func TestConfig_syslogHost(t *testing.T) {
	tests := []struct {
		name       string
		syslogFQDN string
		syslogIP   netip.Addr
		want       string
	}{
		{
			name:       "FQDN set overrides IP",
			syslogFQDN: "syslog.example.com",
			syslogIP:   netip.MustParseAddr("192.168.1.100"),
			want:       "syslog.example.com",
		},
		{
			name:       "empty FQDN falls back to IP",
			syslogFQDN: "",
			syslogIP:   netip.MustParseAddr("192.168.1.100"),
			want:       "192.168.1.100",
		},
		{
			name:       "FQDN with subdomain preserved",
			syslogFQDN: "logs.reboot.example.net",
			syslogIP:   netip.MustParseAddr("10.0.0.1"),
			want:       "logs.reboot.example.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			c.IPXE.HTTPScriptServer.SyslogFQDN = tt.syslogFQDN
			c.IPXE.HTTPScriptServer.SyslogFQDNV6 = "ipv6-only.example.com"
			c.DHCP.SyslogIP = tt.syslogIP

			if got := c.syslogHost(); got != tt.want {
				t.Errorf("syslogHost() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfig_syslogHostV6(t *testing.T) {
	tests := []struct {
		name         string
		syslogFQDNV6 string
		syslogIP     netip.Addr
		want         string
	}{
		{
			name:         "FQDN set overrides IPv6 address",
			syslogFQDNV6: "syslog-v6.example.com",
			syslogIP:     netip.MustParseAddr("2001:db8::100"),
			want:         "syslog-v6.example.com",
		},
		{
			name:     "empty FQDN falls back to IPv6 address",
			syslogIP: netip.MustParseAddr("2001:db8::100"),
			want:     "2001:db8::100",
		},
		{
			name: "unset address does not render an invalid IP",
		},
		{
			name:     "unspecified IPv6 address is not advertised",
			syslogIP: netip.IPv6Unspecified(),
		},
		{
			name:     "IPv4 address is not advertised to IPv6 scripts",
			syslogIP: netip.MustParseAddr("192.0.2.100"),
		},
		{
			name:     "IPv4-mapped address is not advertised to IPv6 scripts",
			syslogIP: netip.MustParseAddr("::ffff:192.0.2.100"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{}
			c.IPXE.HTTPScriptServer.SyslogFQDNV6 = tt.syslogFQDNV6
			c.IPXE.HTTPScriptServer.SyslogFQDN = "ipv4-only.example.com"
			c.DHCPv6.SyslogIP = tt.syslogIP

			if got := c.syslogHostV6(); got != tt.want {
				t.Errorf("syslogHostV6() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunSyslogServer(t *testing.T) {
	// Grab a free UDP port, then release it so the receiver can bind to it.
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("reserve udp port: %v", err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runSyslogServer(ctx, logr.Discard(), addr)
	}()

	// Give the receiver a moment to bind, then cancel to trigger a clean stop.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("runSyslogServer() returned error on clean shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runSyslogServer() did not return after context cancellation")
	}
}

func TestRunSyslogServer_startError(t *testing.T) {
	// An unparseable bind address makes StartReceiver fail, so runSyslogServer
	// should surface the error rather than block.
	err := runSyslogServer(context.Background(), logr.Discard(), "not-a-valid-address")
	if err == nil {
		t.Error("runSyslogServer() expected error for invalid bind address, got nil")
	}
}

func TestDHCPv6BindInterfaces(t *testing.T) {
	tests := []struct {
		name          string
		bindInterface string
		want          []string
	}{
		{
			name:          "empty",
			bindInterface: "",
			want:          []string{""},
		},
		{
			name:          "single interface",
			bindInterface: "macvlan0",
			want:          []string{"macvlan0"},
		},
		{
			name:          "multiple interfaces",
			bindInterface: "macvlan0,eth0",
			want:          []string{"macvlan0", "eth0"},
		},
		{
			name:          "trims whitespace",
			bindInterface: " macvlan0, eth0 ",
			want:          []string{"macvlan0", "eth0"},
		},
		{
			name:          "skips empty interfaces",
			bindInterface: "macvlan0,,eth0",
			want:          []string{"macvlan0", "eth0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, dhcpv6BindInterfaces(tt.bindInterface)); diff != "" {
				t.Fatalf("dhcpv6BindInterfaces() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewConfigDHCPv6Defaults(t *testing.T) {
	cfg := NewConfig(Config{})

	if cfg.DHCPv6.Enabled {
		t.Fatal("DHCPv6 should be disabled by default")
	}
	if cfg.DHCPv6.Mode != DHCPv6ModeStateless {
		t.Fatalf("unexpected DHCPv6 mode: got %q want %q", cfg.DHCPv6.Mode, DHCPv6ModeStateless)
	}
	if !cfg.DHCPv6.EnableNetbootOptions {
		t.Fatal("DHCPv6 netboot options should be enabled by default")
	}
	if got, want := cfg.DHCPv6.BindAddr, netip.IPv6Unspecified(); got != want {
		t.Fatalf("unexpected DHCPv6 bind addr: got %q want %q", got, want)
	}
	if cfg.DHCPv6.IPXEHTTPBinaryURL == cfg.DHCP.IPXEHTTPBinaryURL {
		t.Fatal("DHCPv6 should have its own iPXE HTTP binary URL")
	}
	if cfg.DHCPv6.IPXEHTTPScript.URL == cfg.DHCP.IPXEHTTPScript.URL {
		t.Fatal("DHCPv6 should have its own iPXE HTTP script URL")
	}
	if cfg.IPXE.HTTPScriptServer.OSIEURLv6 == nil {
		t.Fatal("OSIEURLv6 should be initialized by default")
	}
	if cfg.IPXE.HTTPScriptServer.OSIEURLv6 == cfg.IPXE.HTTPScriptServer.OSIEURL {
		t.Fatal("OSIEURLv6 should be independent from OSIEURL")
	}
	if cfg.Syslog.BindAddr.IsValid() {
		t.Fatalf("Syslog bind address should be unset by default: got %q", cfg.Syslog.BindAddr)
	}
	if cfg.TFTP.BindAddr.IsValid() {
		t.Fatalf("TFTP bind address should be unset by default: got %q", cfg.TFTP.BindAddr)
	}
}

func TestNewConfigServiceBindAddresses(t *testing.T) {
	bindAddr := netip.MustParseAddr("192.0.2.1")
	cfg := NewConfig(Config{
		Syslog: Syslog{BindAddr: bindAddr},
		TFTP:   TFTP{BindAddr: bindAddr},
	})

	if got := cfg.Syslog.BindAddr; got != bindAddr {
		t.Fatalf("unexpected Syslog bind address: got %q want %q", got, bindAddr)
	}
	if got := cfg.TFTP.BindAddr; got != bindAddr {
		t.Fatalf("unexpected TFTP bind address: got %q want %q", got, bindAddr)
	}
}

func TestNewConfigDHCPv6DNSDefaults(t *testing.T) {
	wantNameServers := []netip.Addr{
		netip.MustParseAddr("192.0.2.53"),
		netip.MustParseAddr("2001:db8::53"),
	}
	wantDomainSearch := []string{"example.com", "lab.example.com"}
	cfg := NewConfig(Config{
		DHCPv6: DHCPv6{
			DefaultNameServers:      wantNameServers,
			DefaultDomainSearchList: wantDomainSearch,
		},
	})

	if diff := cmp.Diff(wantNameServers, cfg.DHCPv6.DefaultNameServers, cmpopts.EquateComparable(netip.Addr{})); diff != "" {
		t.Fatalf("unexpected default nameservers (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantDomainSearch, cfg.DHCPv6.DefaultDomainSearchList); diff != "" {
		t.Fatalf("unexpected default domain search list (-want +got):\n%s", diff)
	}
}

func TestDHCPv6ModeSet(t *testing.T) {
	var mode DHCPv6Mode
	if err := mode.Set("stateless"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != DHCPv6ModeStateless {
		t.Fatalf("unexpected mode: got %q want %q", mode, DHCPv6ModeStateless)
	}
	if err := mode.Set("auto-stateless"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != DHCPv6ModeAutoStateless {
		t.Fatalf("unexpected mode: got %q want %q", mode, DHCPv6ModeAutoStateless)
	}
	if err := mode.Set("AUTO-STATELESS"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != DHCPv6ModeAutoStateless {
		t.Fatalf("unexpected mode: got %q want %q", mode, DHCPv6ModeAutoStateless)
	}
	if err := mode.Set("reservation"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != DHCPv6ModeReservation {
		t.Fatalf("unexpected mode: got %q want %q", mode, DHCPv6ModeReservation)
	}
	if err := mode.Set("derived"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != DHCPv6ModeDerived {
		t.Fatalf("unexpected mode: got %q want %q", mode, DHCPv6ModeDerived)
	}
	if err := mode.Set("invalid"); err == nil {
		t.Fatal("expected invalid DHCPv6 mode to fail")
	} else if !strings.Contains(err.Error(), string(DHCPv6ModeDerived)) || !strings.Contains(err.Error(), string(DHCPv6ModeReservation)) {
		t.Fatalf("expected invalid mode error to list reservation and derived, got: %v", err)
	}
}

func TestNoServicesEnabledIncludesDHCPv6(t *testing.T) {
	cfg := Config{
		DHCPv6: DHCPv6{Enabled: true},
	}

	if cfg.noServicesEnabled() {
		t.Fatal("DHCPv6 should count as an enabled service")
	}
}

func TestDHCPv6HandlerAutoStatelessEnabled(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	cfg := newDHCPv6HandlerTestConfig()
	cfg.Backend = dhcpv6TestBackend{err: dhcpv6HardwareNotFoundError{}}
	cfg.DHCPv6.DefaultNameServers = []netip.Addr{
		netip.MustParseAddr("192.0.2.53"),
		netip.MustParseAddr("2001:db8::53"),
	}
	cfg.DHCPv6.DefaultDomainSearchList = []string{"example.com"}
	cfg.DHCPv6.Mode = DHCPv6ModeAutoStateless
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	handler, err := cfg.dhcpv6Handler(logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	request := dhcpv6MessageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest)
	dhcpv6.WithRequestedOptions(
		dhcpv6.OptionBootfileURL,
		dhcpv6.OptionDNSRecursiveNameServer,
		dhcpv6.OptionDomainSearchList,
	)(request)
	conn := &dhcpv6RecordingPacketConn{}
	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, request)

	reply := requireDHCPv6Message(t, conn, dhcpv6.MessageTypeReply)
	if got, want := reply.Options.BootFileURL(), "tftp://[2001:db8::10]/00:01:02:03:04:05/ipxe.efi"; got != want {
		t.Fatalf("unexpected boot file URL: got %q want %q", got, want)
	}
	if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::53")}, reply.Options.DNS()); diff != "" {
		t.Fatalf("unexpected DNS servers (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"example.com"}, reply.Options.DomainSearchList().Labels); diff != "" {
		t.Fatalf("unexpected domain search list (-want +got):\n%s", diff)
	}
	wantServerID, err := dhcpv6ServerDUID(cfg.DHCPv6.ServerDUID, cfg.TinkServer.AddrPortV6)
	if err != nil {
		t.Fatal(err)
	}
	if got := reply.Options.ServerID(); got == nil || !got.Equal(wantServerID) {
		t.Fatalf("unexpected server ID: got %v want %v", got, wantServerID)
	}
}

func TestDHCPv6HandlerNetbootDisabled(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	cfg := newDHCPv6HandlerTestConfig()
	cfg.Backend = dhcpv6TestBackend{hardware: dhcpv6HardwareForMAC(mac, "2001:db8::100", "")}
	cfg.DHCPv6.EnableNetbootOptions = false
	cfg.DHCPv6.TFTPIP = netip.Addr{}
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = ""
	cfg.DHCPv6.IPXEHTTPScript.URL = nil
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	handler, err := cfg.dhcpv6Handler(logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	request := dhcpv6MessageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest)
	dhcpv6.WithRequestedOptions(dhcpv6.OptionBootfileURL)(request)
	conn := &dhcpv6RecordingPacketConn{}
	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, request)

	reply := requireDHCPv6Message(t, conn, dhcpv6.MessageTypeReply)
	if got := reply.Options.BootFileURL(); got != "" {
		t.Fatalf("expected disabled netboot to omit the boot file URL, got %q", got)
	}
}

func TestDHCPv6HandlerNetbootEnabledRejectsUnusableConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "missing TFTP address",
			mutate: func(config *Config) {
				config.DHCPv6.TFTPIP = netip.Addr{}
			},
		},
		{
			name: "unspecified TFTP address",
			mutate: func(config *Config) {
				config.DHCPv6.TFTPIP = netip.IPv6Unspecified()
			},
		},
		{
			name: "HTTP binary URL without host",
			mutate: func(config *Config) {
				config.DHCPv6.IPXEHTTPBinaryURL.Host = ""
			},
		},
		{
			name: "HTTP script URL without host",
			mutate: func(config *Config) {
				config.DHCPv6.IPXEHTTPScript.URL.Host = ""
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := newDHCPv6HandlerTestConfig()
			cfg.DHCPv6.EnableNetbootOptions = true
			cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
			cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
			cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
			test.mutate(cfg)

			if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
				t.Fatal("expected unusable DHCPv6 netboot configuration to fail at startup")
			}
		})
	}
}

func TestDHCPv6HandlerReservation(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	cfg := newDHCPv6HandlerTestConfig()
	cfg.Backend = dhcpv6TestBackend{hardware: dhcpv6HardwareForMAC(mac, "2001:db8::100", "")}
	cfg.DHCPv6.Mode = DHCPv6ModeReservation
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	handler, err := cfg.dhcpv6Handler(logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	request := dhcpv6MessageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	request.AddOption(&dhcpv6.OptIANA{IaId: [4]byte{1, 2, 3, 4}})
	request.AddOption(&dhcpv6.OptIANA{IaId: [4]byte{5, 6, 7, 8}})
	conn := &dhcpv6RecordingPacketConn{}
	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, request)

	reply := requireDHCPv6Message(t, conn, dhcpv6.MessageTypeAdvertise)
	address, ok := dhcpv6AddressForIAID(t, reply, [4]byte{1, 2, 3, 4})
	if !ok {
		t.Fatal("expected reservation response to contain an IA address for the first IAID")
	}
	if want := netip.MustParseAddr("2001:db8::100"); address != want {
		t.Fatalf("unexpected reservation address: got %s want %s", address, want)
	}
	if address, ok := dhcpv6AddressForIAID(t, reply, [4]byte{5, 6, 7, 8}); ok {
		t.Fatalf("reservation mode unexpectedly assigned a second address: %s", address)
	}
}

func TestDHCPv6HandlerReservationDoesNotDeriveAddress(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	cfg := newDHCPv6HandlerTestConfig()
	cfg.Backend = dhcpv6TestBackend{hardware: dhcpv6HardwareForMAC(mac, "192.0.2.100", "255.255.255.0")}
	cfg.DHCPv6.Mode = DHCPv6ModeReservation
	// A usable pool would allow a reply if derived mode were accidentally enabled.
	cfg.DHCPv6.DerivedDirectAddressPool = netip.MustParsePrefix("2001:db8:abcd::/64")
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"

	handler, err := cfg.dhcpv6Handler(logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	request := dhcpv6MessageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
	request.AddOption(&dhcpv6.OptIANA{IaId: [4]byte{1, 2, 3, 4}})
	conn := &dhcpv6RecordingPacketConn{}
	handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, request)

	if len(conn.writes) != 0 {
		t.Fatalf("expected no reply without an IPv6 reservation, got %d", len(conn.writes))
	}
}

func TestDHCPv6HandlerInformationRequestWithoutIPv6Reservation(t *testing.T) {
	for _, mode := range []DHCPv6Mode{DHCPv6ModeReservation, DHCPv6ModeDerived} {
		t.Run(string(mode), func(t *testing.T) {
			mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
			cfg := newDHCPv6HandlerTestConfig()
			cfg.Backend = dhcpv6TestBackend{hardware: dhcpv6HardwareForMAC(mac, "192.0.2.100", "255.255.255.0")}
			cfg.DHCPv6.DefaultNameServers = []netip.Addr{
				netip.MustParseAddr("192.0.2.53"),
				netip.MustParseAddr("2001:db8::53"),
			}
			cfg.DHCPv6.DefaultDomainSearchList = []string{"example.com"}
			cfg.DHCPv6.Mode = mode
			cfg.DHCPv6.DerivedDirectAddressPool = netip.Prefix{}
			cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
			cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
			cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
			cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

			handler, err := cfg.dhcpv6Handler(logr.Discard())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			request := dhcpv6MessageWithMAC(t, mac, dhcpv6.MessageTypeInformationRequest)
			dhcpv6.WithRequestedOptions(
				dhcpv6.OptionBootfileURL,
				dhcpv6.OptionDNSRecursiveNameServer,
				dhcpv6.OptionDomainSearchList,
			)(request)
			conn := &dhcpv6RecordingPacketConn{}
			handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, request)

			reply := requireDHCPv6Message(t, conn, dhcpv6.MessageTypeReply)
			if got := reply.Options.IANA(); len(got) != 0 {
				t.Fatalf("information-request reply must not contain IA_NA options, got %v", got)
			}
			if got, want := reply.Options.BootFileURL(), "tftp://[2001:db8::10]/00:01:02:03:04:05/ipxe.efi"; got != want {
				t.Fatalf("unexpected boot file URL: got %q want %q", got, want)
			}
			if diff := cmp.Diff([]net.IP{net.ParseIP("2001:db8::53")}, reply.Options.DNS()); diff != "" {
				t.Fatalf("unexpected DNS servers (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{"example.com"}, reply.Options.DomainSearchList().Labels); diff != "" {
				t.Fatalf("unexpected domain search list (-want +got):\n%s", diff)
			}
			wantServerID, err := dhcpv6ServerDUID(cfg.DHCPv6.ServerDUID, cfg.TinkServer.AddrPortV6)
			if err != nil {
				t.Fatal(err)
			}
			if got := reply.Options.ServerID(); got == nil || !got.Equal(wantServerID) {
				t.Fatalf("unexpected server ID: got %v want %v", got, wantServerID)
			}
		})
	}
}

func TestDHCPv6HandlerDerived(t *testing.T) {
	mac := net.HardwareAddr{0, 1, 2, 3, 4, 5}
	cfg := newDHCPv6HandlerTestConfig()
	cfg.Backend = dhcpv6TestBackend{hardware: dhcpv6HardwareForMAC(mac, "192.0.2.100", "255.255.255.0")}
	cfg.DHCPv6.Mode = DHCPv6ModeDerived
	cfg.DHCPv6.DerivedDirectAddressPool = netip.MustParsePrefix("2001:db8:abcd::/64")
	cfg.DHCPv6.DerivedRelayAddressPrefix = 56
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	handler, err := cfg.dhcpv6Handler(logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("direct address pool", func(t *testing.T) {
		request := dhcpv6MessageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
		request.AddOption(&dhcpv6.OptIANA{IaId: [4]byte{1, 2, 3, 4}})
		conn := &dhcpv6RecordingPacketConn{}
		handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: dhcpv6.DefaultClientPort}, request)

		reply := requireDHCPv6Message(t, conn, dhcpv6.MessageTypeAdvertise)
		address, ok := dhcpv6AddressForIAID(t, reply, [4]byte{1, 2, 3, 4})
		if !ok {
			t.Fatal("expected derived response to contain an IA address")
		}
		if !cfg.DHCPv6.DerivedDirectAddressPool.Contains(address) {
			t.Fatalf("derived address %s is outside configured pool %s", address, cfg.DHCPv6.DerivedDirectAddressPool)
		}
	})

	t.Run("relay address prefix", func(t *testing.T) {
		request := dhcpv6MessageWithMAC(t, mac, dhcpv6.MessageTypeSolicit)
		request.AddOption(&dhcpv6.OptIANA{IaId: [4]byte{5, 6, 7, 8}})
		linkAddress := netip.MustParseAddr("2001:db8:1234:5678::1")
		relay, err := dhcpv6.EncapsulateRelay(
			request,
			dhcpv6.MessageTypeRelayForward,
			net.IP(linkAddress.AsSlice()),
			net.ParseIP("fe80::abcd"),
		)
		if err != nil {
			t.Fatal(err)
		}
		relay.AddOption(dhcpv6.OptClientLinkLayerAddress(iana.HWTypeEthernet, mac))
		conn := &dhcpv6RecordingPacketConn{}
		handler.Handle(context.Background(), conn, &net.UDPAddr{IP: net.ParseIP("fe80::abcd"), Port: dhcpv6.DefaultServerPort}, relay)

		reply := requireDHCPv6RelayMessage(t, conn, dhcpv6.MessageTypeAdvertise)
		address, ok := dhcpv6AddressForIAID(t, reply, [4]byte{5, 6, 7, 8})
		if !ok {
			t.Fatal("expected derived relay response to contain an IA address")
		}
		want := netip.MustParseAddr("2001:db8:1234:560c:e02a:b9c:ced9:5cd0")
		if address != want {
			t.Fatalf("unexpected address derived with /56 relay prefix: got %s want %s", address, want)
		}
	})
}

func TestDHCPv6HandlerRejectsIPv4DerivedDirectPool(t *testing.T) {
	cfg := newDHCPv6HandlerTestConfig()
	cfg.DHCPv6.Mode = DHCPv6ModeDerived
	cfg.DHCPv6.DerivedDirectAddressPool = netip.MustParsePrefix("192.0.2.0/24")
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
		t.Fatal("expected IPv4 derived direct address pool to fail")
	}
}

func TestDHCPv6HandlerRejectsNarrowDerivedDirectPool(t *testing.T) {
	cfg := newDHCPv6HandlerTestConfig()
	cfg.DHCPv6.Mode = DHCPv6ModeDerived
	cfg.DHCPv6.DerivedDirectAddressPool = netip.MustParsePrefix("2001:db8:abcd::/65")
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
		t.Fatal("expected narrow derived direct address pool to fail")
	}
}

func TestDHCPv6HandlerRejectsUnusableDerivedDirectPool(t *testing.T) {
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
			cfg := newDHCPv6HandlerTestConfig()
			cfg.DHCPv6.Mode = DHCPv6ModeDerived
			cfg.DHCPv6.DerivedDirectAddressPool = tc.pool
			cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
			cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
			cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
			cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

			if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
				t.Fatal("expected unusable derived direct address pool to fail")
			}
		})
	}
}

func TestDHCPv6HandlerRejectsInvalidDerivedRelayPrefix(t *testing.T) {
	cfg := newDHCPv6HandlerTestConfig()
	cfg.DHCPv6.Mode = DHCPv6ModeDerived
	cfg.DHCPv6.DerivedRelayAddressPrefix = 129
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
		t.Fatal("expected invalid derived relay prefix to fail")
	}
}

func TestDHCPv6HandlerRejectsNarrowDerivedRelayPrefix(t *testing.T) {
	cfg := newDHCPv6HandlerTestConfig()
	cfg.DHCPv6.Mode = DHCPv6ModeDerived
	cfg.DHCPv6.DerivedRelayAddressPrefix = 65
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
		t.Fatal("expected narrow derived relay prefix to fail")
	}
}

func TestDHCPv6HandlerRejectsZeroDerivedRelayPrefix(t *testing.T) {
	cfg := newDHCPv6HandlerTestConfig()
	cfg.DHCPv6.Mode = DHCPv6ModeDerived
	cfg.DHCPv6.DerivedRelayAddressPrefix = 0
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
		t.Fatal("expected zero derived relay prefix to fail")
	}
}

func TestDHCPv6HandlerIgnoresDerivedValidationOutsideDerivedMode(t *testing.T) {
	for _, mode := range []DHCPv6Mode{
		DHCPv6ModeStateless,
		DHCPv6ModeAutoStateless,
		DHCPv6ModeReservation,
	} {
		t.Run(string(mode), func(t *testing.T) {
			cfg := newDHCPv6HandlerTestConfig()
			cfg.DHCPv6.Mode = mode
			cfg.DHCPv6.DerivedDirectAddressPool = netip.MustParsePrefix("::/0")
			cfg.DHCPv6.DerivedRelayAddressPrefix = 0
			cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
			cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
			cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
			cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

			if _, err := cfg.dhcpv6Handler(logr.Discard()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDHCPv6HandlerRejectsInvalidServerDUID(t *testing.T) {
	cfg := newDHCPv6HandlerTestConfig()
	cfg.DHCPv6.ServerDUID = "not-hex"
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"

	if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
		t.Fatal("expected invalid DHCPv6 server DUID to fail")
	}
}

func TestDHCPv6ServerDUID(t *testing.T) {
	// Fixed UUIDv5 vectors also pin the namespace, name format, version, and variant.
	expected := &dhcpv6.DUIDUUID{UUID: uuid.MustParse("7e73e512-ca05-5bf6-84b6-9a237eccfa06")}

	tests := map[string]struct {
		addrPort string
		want     dhcpv6.DUID
	}{
		"ipv6 grpc endpoint":       {addrPort: "[2001:db8::20]:42113", want: expected},
		"same ipv6 different port": {addrPort: "[2001:db8::20]:12345", want: expected},
		"same ipv6 expanded":       {addrPort: "[2001:0db8:0000:0000:0000:0000:0000:0020]:42113", want: expected},
		"different ipv6":           {addrPort: "[2001:db8::21]:42113", want: &dhcpv6.DUIDUUID{UUID: uuid.MustParse("ed3b50d2-4734-52d9-bf6a-f900f86290db")}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := dhcpv6ServerDUID("", tt.addrPort)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("unexpected DUID diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDHCPv6ServerDUIDFallbackForInvalidAddress(t *testing.T) {
	// The existing fallback already carries UUIDv4 and RFC 4122 variant bits.
	expected := &dhcpv6.DUIDUUID{UUID: uuid.MustParse("8d5f4b6a-7c2e-490f-9a38-369d4c612f10")}
	tests := map[string]string{
		"ipv4":             "192.0.2.20:42113",
		"missing port":     "2001:db8::20",
		"invalid endpoint": "not-an-addr-port",
		"empty endpoint":   "",
	}

	for name, addrPort := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := dhcpv6ServerDUID("", addrPort)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(expected, got); diff != "" {
				t.Fatalf("unexpected fallback DUID diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDHCPv6ServerDUIDConfigured(t *testing.T) {
	want := &dhcpv6.DUIDUUID{UUID: [16]byte{0x12, 0x34, 0x56, 0x78, 0x12, 0x34, 0x56, 0x78, 0x90, 0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78}}
	tests := map[string]string{
		"colon separated": "00:04:12:34:56:78:12:34:56:78:90:ab:cd:ef:12:34:56:78",
		"dash separated":  "00-04-12-34-56-78-12-34-56-78-90-ab-cd-ef-12-34-56-78",
		"plain hex":       "0004123456781234567890abcdef12345678",
	}

	for name, configured := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := dhcpv6ServerDUID(configured, "[2001:db8::20]:42113")
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("unexpected DUID diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDHCPv6ServerDUIDConfiguredInvalid(t *testing.T) {
	tests := []string{
		"0",
		"not-hex",
		"00:04:01",
	}

	for _, configured := range tests {
		t.Run(configured, func(t *testing.T) {
			if _, err := dhcpv6ServerDUID(configured, "[2001:db8::20]:42113"); err == nil {
				t.Fatal("expected invalid configured DUID to fail")
			}
		})
	}
}

func TestScriptHandlerUsesStaticIPXEForDHCPv6AutoStateless(t *testing.T) {
	metric.Init()
	cfg := NewConfig(Config{})
	cfg.DHCP.Mode = DHCPModeReservation
	cfg.DHCPv6.Enabled = true
	cfg.DHCPv6.Mode = DHCPv6ModeAutoStateless
	cfg.DHCPv6.SyslogIP = netip.MustParseAddr("2001:db8::10")
	cfg.IPXE.HTTPScriptServer.SyslogFQDNV6 = "2001:db8::20"
	cfg.IPXE.HTTPScriptServer.OSIEURL.Scheme = "http"
	cfg.IPXE.HTTPScriptServer.OSIEURL.Host = "osie.example"

	handler := cfg.ScriptHandler(logr.Discard())
	if handler == nil {
		t.Fatal("expected script handler")
	}

	req := httptest.NewRequest(http.MethodGet, "/ipxe/script/auto6.ipxe", nil)
	req.RemoteAddr = "[2001:db8::1]:12345"
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if body := resp.Body.String(); body == "" {
		t.Fatal("expected static iPXE response body")
	} else if !strings.Contains(body, "set syslog-address:ipv6 2001:db8::20") {
		t.Fatalf("expected configured syslog override in static IPv6 iPXE response body, got:\n%s", body)
	} else if strings.Contains(body, "set syslog-address:ipv6 2001:db8::10") {
		t.Fatalf("expected configured syslog override to take precedence over DHCPv6 syslog IP, got:\n%s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/ipxe/script/auto.ipxe", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("DHCPv6 auto-stateless enabled IPv4 static fallback: got status %d want %d", resp.Code, http.StatusNotFound)
	}
}

func TestScriptHandlerPreservesIPv4AutoProxyFallbackWhenDHCPDisabled(t *testing.T) {
	metric.JobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "test_ipv4_fallback_jobs_duration_seconds"}, []string{"from", "op"})
	metric.JobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_ipv4_fallback_jobs_total"}, []string{"from", "op"})
	metric.JobsInProgress = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_ipv4_fallback_jobs_in_progress"}, []string{"from", "op"})
	cfg := NewConfig(Config{})
	cfg.Backend = nil
	cfg.DHCP.Enabled = false
	cfg.DHCP.Mode = DHCPModeAutoProxy
	cfg.DHCP.SyslogIP = netip.MustParseAddr("192.0.2.10")
	cfg.DHCPv6.Enabled = false
	cfg.DHCPv6.Mode = DHCPv6ModeAutoStateless

	handler := cfg.ScriptHandler(logr.Discard())
	if handler == nil {
		t.Fatal("expected script handler")
	}

	tests := map[string]struct {
		script        string
		remoteAddr    string
		wantStatus    int
		wantBody      string
		wantEmptyBody bool
	}{
		"IPv4 fallback remains enabled": {
			script:     "auto.ipxe",
			remoteAddr: "192.0.2.1:12345",
			wantStatus: http.StatusOK,
			wantBody:   "set syslog-address:ipv4 192.0.2.10",
		},
		"IPv6 fallback remains disabled": {
			script:        "auto6.ipxe",
			remoteAddr:    "[2001:db8::1]:12345",
			wantStatus:    http.StatusNotFound,
			wantEmptyBody: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ipxe/script/"+tt.script, nil)
			req.RemoteAddr = tt.remoteAddr
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			if resp.Code != tt.wantStatus {
				t.Fatalf("unexpected status code: got %d want %d", resp.Code, tt.wantStatus)
			}
			body := resp.Body.String()
			if tt.wantEmptyBody && body != "" {
				t.Fatalf("expected empty response body, got:\n%s", body)
			}
			if tt.wantBody != "" && !strings.Contains(body, tt.wantBody) {
				t.Fatalf("expected response body to contain %q, got:\n%s", tt.wantBody, body)
			}
		})
	}
}
