package workflow

import (
	"testing"
)

func TestNetmaskToPrefixLength(t *testing.T) {
	tests := []struct {
		name      string
		netmask   string
		want      string
		wantError bool
	}{
		{
			name:      "valid /24 netmask",
			netmask:   "255.255.255.0",
			want:      "24",
			wantError: false,
		},
		{
			name:      "valid /16 netmask",
			netmask:   "255.255.0.0",
			want:      "16",
			wantError: false,
		},
		{
			name:      "valid /8 netmask",
			netmask:   "255.0.0.0",
			want:      "8",
			wantError: false,
		},
		{
			name:      "valid /32 netmask",
			netmask:   "255.255.255.255",
			want:      "32",
			wantError: false,
		},
		{
			name:      "valid /0 netmask",
			netmask:   "0.0.0.0",
			want:      "0",
			wantError: false,
		},
		{
			name:      "valid /28 netmask",
			netmask:   "255.255.255.240",
			want:      "28",
			wantError: false,
		},
		{
			name:      "valid /30 netmask",
			netmask:   "255.255.255.252",
			want:      "30",
			wantError: false,
		},
		{
			name:      "invalid netmask format",
			netmask:   "invalid",
			want:      "",
			wantError: true,
		},
		{
			name:      "empty netmask",
			netmask:   "",
			want:      "",
			wantError: true,
		},
		{
			name:      "incomplete netmask",
			netmask:   "255.255.255",
			want:      "",
			wantError: true,
		},
		{
			name:      "out of range values",
			netmask:   "256.255.255.0",
			want:      "",
			wantError: true,
		},
		{
			name:      "IPv6 address",
			netmask:   "::1",
			want:      "",
			wantError: true,
		},
		{
			name:      "IPv6 full address",
			netmask:   "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			want:      "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := netmaskToPrefixLength(tt.netmask)
			if (err != nil) != tt.wantError {
				t.Errorf("netmaskToPrefixLength() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if got != tt.want {
				t.Errorf("netmaskToPrefixLength() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatPartition(t *testing.T) {
	tests := []struct {
		name      string
		dev       string
		partition int
		want      string
	}{
		{
			name:      "nvme device partition 1",
			dev:       "/dev/nvme0n1",
			partition: 1,
			want:      "/dev/nvme0n1p1",
		},
		{
			name:      "nvme device partition 2",
			dev:       "/dev/nvme0n1",
			partition: 2,
			want:      "/dev/nvme0n1p2",
		},
		{
			name:      "sda device partition 1",
			dev:       "/dev/sda",
			partition: 1,
			want:      "/dev/sda1",
		},
		{
			name:      "vda device partition 1",
			dev:       "/dev/vda",
			partition: 1,
			want:      "/dev/vda1",
		},
		{
			name:      "xvda device partition 1",
			dev:       "/dev/xvda",
			partition: 1,
			want:      "/dev/xvda1",
		},
		{
			name:      "hda device partition 1",
			dev:       "/dev/hda",
			partition: 1,
			want:      "/dev/hda1",
		},
		{
			name:      "unknown device returns unchanged",
			dev:       "/dev/unknown",
			partition: 1,
			want:      "/dev/unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPartition(tt.dev, tt.partition)
			if got != tt.want {
				t.Errorf("formatPartition() = %v, want %v", got, tt.want)
			}
		})
	}
}
