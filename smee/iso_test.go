package smee

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestISOHandlersEndpointFamily(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/source.iso" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(isoMagicString))
	}))
	defer upstream.Close()
	sourceURL, err := url.Parse(upstream.URL + "/source.iso")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		ipv4       bool
		ipv6       bool
		fqdn       bool
		route      string
		wantSyslog string
		wantGRPC   string
	}{
		{"IPv4 only", true, false, false, ISOURI, "192.0.2.10", "192.0.2.10:42113"},
		{"IPv6 only", false, true, false, ISOURIV6, "2001:db8::10", "[2001:db8::10]:42113"},
		{"dual stack IPv4", true, true, false, ISOURI, "192.0.2.10", "192.0.2.10:42113"},
		{"dual stack IPv6", true, true, false, ISOURIV6, "2001:db8::10", "[2001:db8::10]:42113"},
		{"IPv4 syslog FQDN", true, true, true, ISOURI, "syslog-v4.example.com", "192.0.2.10:42113"},
		{"IPv6 syslog FQDN", true, true, true, ISOURIV6, "syslog-v6.example.com", "[2001:db8::10]:42113"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const mac = "de:ed:be:ef:fe:ed"
			c := Config{
				ISO: ISO{Enabled: true, UpstreamURL: sourceURL},
				Backend: dhcpv6TestBackend{hardware: &tinkerbell.Hardware{
					Spec: tinkerbell.HardwareSpec{
						Interfaces: []tinkerbell.Interface{{
							DHCP:    &tinkerbell.DHCP{MAC: mac},
							Netboot: &tinkerbell.Netboot{},
						}},
					},
				}},
			}
			if tt.ipv4 {
				c.DHCP.SyslogIP = netip.MustParseAddr("192.0.2.10")
				c.TinkServer.AddrPort = "192.0.2.10:42113"
			}
			if tt.ipv6 {
				c.DHCPv6.SyslogIP = netip.MustParseAddr("2001:db8::10")
				c.TinkServer.AddrPortV6 = "[2001:db8::10]:42113"
			}
			if tt.fqdn {
				c.IPXE.HTTPScriptServer.SyslogFQDN = "syslog-v4.example.com"
				c.IPXE.HTTPScriptServer.SyslogFQDNV6 = "syslog-v6.example.com"
			}
			c.TinkServer.UseTLS = true
			c.IPXE.HTTPScriptServer.ExtraKernelArgs = []string{"test_arg=1"}

			mux := http.NewServeMux()
			for route, factory := range map[string]func(logr.Logger) (http.Handler, error){
				ISOURI: c.ISOHandler, ISOURIV6: c.ISOHandlerV6,
			} {
				handler, err := factory(logr.Discard())
				if err != nil {
					t.Fatal(err)
				}
				mux.Handle(route, handler)
			}

			// A BMC's download connection does not select the target's endpoint family.
			for _, remoteAddr := range []string{"192.0.2.20:12345", "[2001:db8::20]:12345"} {
				req := httptest.NewRequest(http.MethodGet, tt.route+mac+"/hook.iso", nil)
				req.RemoteAddr = remoteAddr
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, req)
				if w.Code != http.StatusOK {
					t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
				}
				want := "console=ttyAMA0 console=ttyS0 console=tty0 console=tty1 console=ttyS1" +
					" hw_addr=" + mac + " syslog_host=" + tt.wantSyslog + " grpc_authority=" + tt.wantGRPC +
					" tinkerbell_tls=true worker_id=" + mac + " test_arg=1"
				if diff := cmp.Diff(want, strings.TrimSpace(w.Body.String())); diff != "" {
					t.Fatalf("patched ISO mismatch (-want +got):\n%s", diff)
				}
				if w.Body.Len() != len(isoMagicString) {
					t.Fatalf("patched ISO size = %d, want %d", w.Body.Len(), len(isoMagicString))
				}
			}
		})
	}
}

func TestISOHandlersDisabledAndInvalidSource(t *testing.T) {
	c := Config{}
	for name, factory := range map[string]func(logr.Logger) (http.Handler, error){
		"IPv4": c.ISOHandler, "IPv6": c.ISOHandlerV6,
	} {
		t.Run(name, func(t *testing.T) {
			c.ISO.Enabled = false
			if handler, err := factory(logr.Discard()); handler != nil || err != nil {
				t.Fatalf("disabled handler = %v, error = %v; want nil, nil", handler, err)
			}
			c.ISO.Enabled = true
			c.ISO.UpstreamURL = &url.URL{Scheme: "ftp", Host: "example.com", Path: "/hook.iso"}
			if _, err := factory(logr.Discard()); err == nil {
				t.Fatal("expected an error for unsupported ISO source scheme")
			}
		})
	}
}

func TestISOHandlerV6RejectsStaticIPAM(t *testing.T) {
	const mac = "de:ed:be:ef:fe:ed"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(isoMagicString))
	}))
	defer upstream.Close()
	sourceURL, err := url.Parse(upstream.URL + "/source.iso")
	if err != nil {
		t.Fatal(err)
	}
	c := Config{ISO: ISO{Enabled: true, UpstreamURL: sourceURL, StaticIPAMEnabled: true}}
	h, err := c.ISOHandlerV6(logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	// No backend is configured: rejection must happen before hardware lookup.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, ISOURIV6+mac+"/hook.iso", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "static IPAM is not supported for IPv6 ISO boot") {
		t.Fatalf("missing explanation in response: %q", w.Body.String())
	}

	// The same configuration must still serve IPv4 ISOs with static IPAM.
	c.Backend = dhcpv6TestBackend{hardware: &tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{Interfaces: []tinkerbell.Interface{{
			DHCP: &tinkerbell.DHCP{
				MAC: mac,
				IP:  &tinkerbell.IP{Address: "192.0.2.20", Netmask: "255.255.255.0", Gateway: "192.0.2.1"},
			},
			Netboot: &tinkerbell.Netboot{},
		}}},
	}}
	h, err = c.ISOHandler(logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, ISOURI+mac+"/hook.iso", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("IPv4 status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ipam=de-ed-be-ef-fe-ed::192.0.2.20:255.255.255.0:192.0.2.1::::") {
		t.Fatalf("IPv4 static IPAM missing from patched ISO: %q", w.Body.String())
	}

	c.ISO.Enabled = false
	if h, err := c.ISOHandlerV6(logr.Discard()); h != nil || err != nil {
		t.Fatalf("disabled handler = %v, error = %v; want nil, nil", h, err)
	}
}
