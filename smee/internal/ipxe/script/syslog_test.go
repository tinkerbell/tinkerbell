package script

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/smee/internal/metric"
)

func TestHandlerSyslogUsesScriptFamily(t *testing.T) {
	metric.JobDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "test_syslog_jobs_duration_seconds"}, []string{"from", "op"})
	metric.JobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_syslog_jobs_total"}, []string{"from", "op"})
	metric.JobsInProgress = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_syslog_jobs_in_progress"}, []string{"from", "op"})
	const mac = "08:00:27:9e:f5:3a"
	allowNetboot := true
	backend := testBackend{hardware: map[string]*tinkerbell.Hardware{
		mac: {Spec: tinkerbell.HardwareSpec{Interfaces: []tinkerbell.Interface{{
			DHCP:    &tinkerbell.DHCP{MAC: mac, Arch: x8664Arch},
			Netboot: &tinkerbell.Netboot{AllowPXE: &allowNetboot},
		}}}},
	}}
	for mode, backend := range map[string]testBackend{"hook": backend, "static": {}} {
		for _, route := range []struct {
			name, family, setting, other, remoteAddr string
		}{
			{"auto.ipxe", "ipv4", "syslog", "syslog6", "[2001:db8::1]:12345"},
			{"auto6.ipxe", "ipv6", "syslog6", "syslog", "192.0.2.1:12345"},
		} {
			for _, tc := range []struct{ name, host4, host6 string }{
				{"hostnames", "syslog-v4.invalid", "syslog-v6.invalid"},
				{"literals", "192.0.2.10", "2001:db8::10"},
				{"wrong family literals", "2001:db8::10", "192.0.2.10"},
				{"IPv6 only config", "", "syslog-v6.invalid"},
				{"IPv4 only config", "syslog-v4.invalid", ""},
				{"unset", "", ""},
			} {
				t.Run(mode+"/"+route.name+"/"+tc.name, func(t *testing.T) {
					h := &Handler{
						Backend:             backend,
						PublicSyslogFQDN:    tc.host4,
						PublicSyslogFQDNV6:  tc.host6,
						StaticIPXEEnabled:   mode == "static",
						StaticIPXEV6Enabled: mode == "static",
					}
					req := httptest.NewRequest(http.MethodGet, "/ipxe/script/"+mac+"/"+route.name, nil)
					// Select by route even when the HTTP peer uses the other family.
					req.RemoteAddr = route.remoteAddr
					resp := httptest.NewRecorder()
					h.HandlerFunc().ServeHTTP(resp, req)
					if resp.Code != http.StatusOK {
						t.Fatalf("expected status %d, got %d", http.StatusOK, resp.Code)
					}
					host := tc.host4
					if route.family == "ipv6" {
						host = tc.host6
					}
					body := resp.Body.String()
					if host == "" {
						if strings.Contains(body, "syslog-address") || strings.Contains(body, "nslookup ") {
							t.Fatalf("unexpected syslog configuration without a host:\n%s", body)
						}
					} else {
						want := fmt.Sprintf("clear syslog-address\nset syslog-address:%s %s || nslookup syslog-address %s && set %s ${syslog-address} || echo [WARN] Failed to configure %s host %s: resolution failed or expected %s address\nclear syslog-address",
							route.family, host, host, route.setting, route.setting, host, route.family)
						if !strings.Contains(body, want) {
							t.Fatalf("expected client-side lookup %q in script:\n%s", want, body)
						}
					}
					if strings.Contains(body, "set "+route.other+" ") || strings.Contains(body, "clear "+route.other+"\n") {
						t.Fatalf("script changes the other family's syslog setting:\n%s", body)
					}
					kernelHost := "syslog_host=" + host + " "
					if mode == "static" {
						kernelHost = "set syslog_host " + host + "\n"
					}
					if !strings.Contains(body, kernelHost) {
						t.Fatalf("kernel syslog host was not preserved: want %q in:\n%s", kernelHost, body)
					}
				})
			}
		}
	}
}
