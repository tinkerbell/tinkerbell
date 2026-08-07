package smee

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

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
			c.DHCP.SyslogIP = tt.syslogIP

			if got := c.syslogHost(); got != tt.want {
				t.Errorf("syslogHost() = %q, want %q", got, tt.want)
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
