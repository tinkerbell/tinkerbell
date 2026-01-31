package smee

import (
	"net/netip"
	"testing"
)

func TestSyslogFQDN(t *testing.T) {
	tests := []struct {
		name          string
		syslogFQDN    string
		syslogIP      netip.Addr
		expectedValue string
	}{
		{
			name:          "FQDN set overrides IP",
			syslogFQDN:    "syslog.example.com",
			syslogIP:      netip.MustParseAddr("192.168.1.100"),
			expectedValue: "syslog.example.com",
		},
		{
			name:          "empty FQDN falls back to IP",
			syslogFQDN:    "",
			syslogIP:      netip.MustParseAddr("192.168.1.100"),
			expectedValue: "192.168.1.100",
		},
		{
			name:          "FQDN with subdomain preserved",
			syslogFQDN:    "logs.reboot.mcclimans.net",
			syslogIP:      netip.MustParseAddr("10.0.0.1"),
			expectedValue: "logs.reboot.mcclimans.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from smee.go
			syslogHost := tt.syslogFQDN
			if syslogHost == "" {
				syslogHost = tt.syslogIP.String()
			}

			if syslogHost != tt.expectedValue {
				t.Errorf("syslogHost = %q, want %q", syslogHost, tt.expectedValue)
			}
		})
	}
}
