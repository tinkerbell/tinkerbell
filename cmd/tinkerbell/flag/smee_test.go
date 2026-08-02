package flag

import (
	"net/netip"
	"testing"

	"github.com/tinkerbell/tinkerbell/smee"
)

func TestSmeeConvertBindAddressPrecedence(t *testing.T) {
	globalBindAddr := netip.MustParseAddr("192.0.2.10")
	syslogBindAddr := netip.MustParseAddr("192.0.2.20")
	tftpBindAddr := netip.MustParseAddr("192.0.2.21")

	tests := []struct {
		name       string
		syslogAddr netip.Addr
		tftpAddr   netip.Addr
		wantSyslog netip.Addr
		wantTFTP   netip.Addr
	}{
		{
			name:       "global address is the default",
			wantSyslog: globalBindAddr,
			wantTFTP:   globalBindAddr,
		},
		{
			name:       "service-specific addresses take precedence",
			syslogAddr: syslogBindAddr,
			tftpAddr:   tftpBindAddr,
			wantSyslog: syslogBindAddr,
			wantTFTP:   tftpBindAddr,
		},
		{
			name:       "service-specific syslog address takes precedence independently",
			syslogAddr: syslogBindAddr,
			wantSyslog: syslogBindAddr,
			wantTFTP:   globalBindAddr,
		},
		{
			name:       "service-specific TFTP address takes precedence independently",
			tftpAddr:   tftpBindAddr,
			wantSyslog: globalBindAddr,
			wantTFTP:   tftpBindAddr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &SmeeConfig{Config: smee.NewConfig(smee.Config{
				Syslog: smee.Syslog{BindAddr: tt.syslogAddr},
				TFTP:   smee.TFTP{BindAddr: tt.tftpAddr},
			})}

			cfg.Convert(nil, netip.Addr{}, globalBindAddr, 8080)

			if got := cfg.Config.Syslog.BindAddr; got != tt.wantSyslog {
				t.Errorf("Syslog.BindAddr = %v, want %v", got, tt.wantSyslog)
			}
			if got := cfg.Config.TFTP.BindAddr; got != tt.wantTFTP {
				t.Errorf("TFTP.BindAddr = %v, want %v", got, tt.wantTFTP)
			}
		})
	}
}
