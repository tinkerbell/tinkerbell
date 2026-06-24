package smee

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/insomniacslk/dhcp/dhcpv6"
	reservationv6 "github.com/tinkerbell/tinkerbell/smee/internal/dhcp/dhcpv6/handler/reservation"
	statelessv6 "github.com/tinkerbell/tinkerbell/smee/internal/dhcp/dhcpv6/handler/stateless"
	"github.com/tinkerbell/tinkerbell/smee/internal/metric"
)

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
	cfg := NewConfig(Config{}, netip.Addr{})

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
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
	cfg.DHCPv6.Mode = DHCPv6ModeAutoStateless
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	handler, err := cfg.dhcpv6Handler(logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h, ok := handler.(*statelessv6.Handler)
	if !ok {
		t.Fatalf("unexpected handler type: %T", handler)
	}
	if !h.AutoStatelessEnabled {
		t.Fatal("expected auto-stateless to be enabled")
	}
	if !h.Netboot.Enabled {
		t.Fatal("expected DHCPv6 netboot options to be enabled")
	}
	wantDUID, err := dhcpv6ServerDUID(cfg.DHCPv6.ServerDUID, cfg.TinkServer.AddrPortV6)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantDUID, h.ServerID); diff != "" {
		t.Fatalf("unexpected DHCPv6 server DUID diff (-want +got):\n%s", diff)
	}
}

func TestDHCPv6HandlerNetbootDisabled(t *testing.T) {
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
	cfg.DHCPv6.EnableNetbootOptions = false
	cfg.DHCPv6.TFTPIP = netip.Addr{}
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = ""
	cfg.DHCPv6.IPXEHTTPScript.URL = nil
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	handler, err := cfg.dhcpv6Handler(logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h, ok := handler.(*statelessv6.Handler)
	if !ok {
		t.Fatalf("unexpected handler type: %T", handler)
	}
	if h.Netboot.Enabled {
		t.Fatal("expected DHCPv6 netboot options to be disabled")
	}
	if h.Netboot.IPXEBinServerTFTP.IsValid() {
		t.Fatalf("expected no DHCPv6 TFTP server when netboot is disabled, got %s", h.Netboot.IPXEBinServerTFTP)
	}
	if h.Netboot.IPXEBinServerHTTP != nil {
		t.Fatalf("expected no DHCPv6 HTTP binary URL when netboot is disabled, got %s", h.Netboot.IPXEBinServerHTTP)
	}
	if h.Netboot.IPXEScriptURL != nil {
		t.Fatal("expected no DHCPv6 iPXE script URL builder when netboot is disabled")
	}
}

func TestDHCPv6HandlerNetbootEnabledRequiresTFTPAddress(t *testing.T) {
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
	cfg.DHCPv6.EnableNetbootOptions = true
	cfg.DHCPv6.TFTPIP = netip.Addr{}
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"

	if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
		t.Fatal("expected invalid DHCPv6 TFTP address to fail when netboot is enabled")
	}
}

func TestDHCPv6HandlerReservation(t *testing.T) {
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
	cfg.DHCPv6.Mode = DHCPv6ModeReservation
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"
	cfg.TinkServer.AddrPortV6 = "[2001:db8::20]:42113"

	handler, err := cfg.dhcpv6Handler(logr.Discard())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h, ok := handler.(*reservationv6.Handler)
	if !ok {
		t.Fatalf("unexpected handler type: %T", handler)
	}
	if h.Derived {
		t.Fatal("expected reservation handler to have derived addressing disabled")
	}
	if !h.Netboot.Enabled {
		t.Fatal("expected DHCPv6 netboot options to be enabled")
	}
	wantDUID, err := dhcpv6ServerDUID(cfg.DHCPv6.ServerDUID, cfg.TinkServer.AddrPortV6)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(wantDUID, h.ServerID); diff != "" {
		t.Fatalf("unexpected DHCPv6 server DUID diff (-want +got):\n%s", diff)
	}
}

func TestDHCPv6HandlerDerived(t *testing.T) {
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
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

	h, ok := handler.(*reservationv6.Handler)
	if !ok {
		t.Fatalf("unexpected handler type: %T", handler)
	}
	if !h.Derived {
		t.Fatal("expected derived handler to have derived addressing enabled")
	}
	if h.DerivedDirectAddressPool != cfg.DHCPv6.DerivedDirectAddressPool {
		t.Fatalf("unexpected derived direct address pool: got %s want %s", h.DerivedDirectAddressPool, cfg.DHCPv6.DerivedDirectAddressPool)
	}
	if h.DerivedRelayAddressPrefix != cfg.DHCPv6.DerivedRelayAddressPrefix {
		t.Fatalf("unexpected derived relay address prefix: got %d want %d", h.DerivedRelayAddressPrefix, cfg.DHCPv6.DerivedRelayAddressPrefix)
	}
}

func TestDHCPv6HandlerRejectsIPv4DerivedDirectPool(t *testing.T) {
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
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
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
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
			cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
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
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
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
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
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
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
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
			cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
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
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
	cfg.DHCPv6.ServerDUID = "not-hex"
	cfg.DHCPv6.TFTPIP = netip.MustParseAddr("2001:db8::10")
	cfg.DHCPv6.IPXEHTTPBinaryURL.Host = "boot.example"
	cfg.DHCPv6.IPXEHTTPScript.URL.Host = "boot.example"

	if _, err := cfg.dhcpv6Handler(logr.Discard()); err == nil {
		t.Fatal("expected invalid DHCPv6 server DUID to fail")
	}
}

func TestDHCPv6ServerDUID(t *testing.T) {
	expectedHash := sha256.Sum256([]byte(DHCPv6ServerDUIDHashPrefix + "2001:db8::20"))
	var expectedUUID [16]byte
	copy(expectedUUID[:], expectedHash[:16])
	expected := &dhcpv6.DUIDUUID{UUID: expectedUUID}

	tests := map[string]struct {
		addrPort string
		want     dhcpv6.DUID
	}{
		"ipv6 grpc endpoint":       {addrPort: "[2001:db8::20]:42113", want: expected},
		"same ipv6 different port": {addrPort: "[2001:db8::20]:12345", want: expected},
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
			if diff := cmp.Diff(fallbackDHCPv6ServerDUID, got); diff != "" {
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
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
	cfg.DHCP.Mode = DHCPModeReservation
	cfg.DHCPv6.Enabled = true
	cfg.DHCPv6.Mode = DHCPv6ModeAutoStateless
	cfg.DHCPv6.SyslogIP = netip.MustParseAddr("2001:db8::10")
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
	} else if !strings.Contains(body, "set syslog6 2001:db8::10") {
		t.Fatalf("expected DHCPv6 syslog IP in static iPXE response body, got:\n%s", body)
	}
}

func TestScriptHandlerIgnoresAutoModesWhenDHCPDisabled(t *testing.T) {
	cfg := NewConfig(Config{}, netip.MustParseAddr("192.0.2.1"))
	cfg.Backend = nil
	cfg.DHCP.Enabled = false
	cfg.DHCP.Mode = DHCPModeAutoProxy
	cfg.DHCPv6.Enabled = false
	cfg.DHCPv6.Mode = DHCPv6ModeAutoStateless

	handler := cfg.ScriptHandler(logr.Discard())
	if handler == nil {
		t.Fatal("expected script handler")
	}

	req := httptest.NewRequest(http.MethodGet, "/ipxe/script/auto6.ipxe", nil)
	req.RemoteAddr = "[2001:db8::1]:12345"
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusNotFound)
	}
	if body := resp.Body.String(); body != "" {
		t.Fatalf("expected empty response body, got:\n%s", body)
	}
}
