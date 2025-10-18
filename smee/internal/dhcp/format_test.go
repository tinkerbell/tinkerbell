package dhcp

import (
	"net"
	"testing"
)

func TestStringDash(t *testing.T) {
	tests := map[string]struct {
		addr     net.HardwareAddr
		expected string
	}{
		"empty":              {net.HardwareAddr{}, ""},
		"single byte":        {net.HardwareAddr{0x01}, "01"},
		"two bytes":          {net.HardwareAddr{0x01, 0x02}, "01-02"},
		"six bytes":          {net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}, "01-02-03-04-05-06"},
		"48-bit MAC address": {net.HardwareAddr{0x00, 0x00, 0x5e, 0x00, 0x53, 0x01}, "00-00-5e-00-53-01"},
		"64-bit EUI-64":      {net.HardwareAddr{0x00, 0x00, 0xfe, 0x80, 0x02, 0x5e, 0x10, 0x01}, "00-00-fe-80-02-5e-10-01"},
		"20-octet IP over InfiniBand link-layer address": {net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x5e, 0x10, 0x00, 0x00, 0x00, 0x01}, "00-00-00-00-fe-80-00-00-00-00-00-02-00-5e-10-00-00-00-01"},
		"nil": {nil, ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := dashNotation(tt.addr)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestStringDot(t *testing.T) {
	tests := map[string]struct {
		addr     net.HardwareAddr
		expected string
	}{
		"empty":              {net.HardwareAddr{}, ""},
		"nil":                {nil, ""},
		"single byte":        {net.HardwareAddr{0x01}, "01"},
		"two bytes":          {net.HardwareAddr{0x01, 0x02}, "0102"},
		"six bytes":          {net.HardwareAddr{0x01, 0x02, 0x03, 0x04}, "0102.0304"},
		"48-bit MAC address": {net.HardwareAddr{0x00, 0x00, 0x5e, 0x00, 0x53, 0x01}, "0000.5e00.5301"},
		"64-bit EUI-64":      {net.HardwareAddr{0x00, 0x00, 0xfe, 0x80, 0x02, 0x5e, 0x10, 0x01}, "0000.fe80.025e.1001"},
		"20-octet IP over InfiniBand link-layer address": {net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x5e, 0x10, 0x00, 0x00, 0x00, 0x01}, "0000.0000.fe80.0000.0000.0000.0200.5e10.0000.0001"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := dotNotation(tt.addr)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestNoDelimiter(t *testing.T) {
	tests := map[string]struct {
		addr     net.HardwareAddr
		expected string
	}{
		"empty":              {net.HardwareAddr{}, ""},
		"nil":                {nil, ""},
		"single byte":        {net.HardwareAddr{0x01}, "01"},
		"two bytes":          {net.HardwareAddr{0x01, 0x02}, "0102"},
		"six bytes":          {net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06}, "010203040506"},
		"48-bit MAC address": {net.HardwareAddr{0x00, 0x00, 0x5e, 0x00, 0x53, 0x01}, "00005e005301"},
		"64-bit EUI-64":      {net.HardwareAddr{0x00, 0x00, 0xfe, 0x80, 0x02, 0x5e, 0x10, 0x01}, "0000fe80025e1001"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := noDelimiter(tt.addr)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}
