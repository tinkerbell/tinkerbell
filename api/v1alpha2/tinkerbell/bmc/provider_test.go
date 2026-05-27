package bmc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestProviderString(t *testing.T) {
	tests := map[string]struct {
		provider ProviderName
		expected string
	}{
		"IPMITool":   {ProviderNameIPMITool, "ipmitool"},
		"AsrockRack": {ProviderNameAsrockRack, "asrockrack"},
		"Gofish":     {ProviderNameGofish, "gofish"},
		"IntelAMT":   {ProviderNameIntelAMT, "IntelAMT"},
		"Dell":       {ProviderNameDell, "dell"},
		"Supermicro": {ProviderNameSupermicro, "supermicro"},
		"OpenBMC":    {ProviderNameOpenBMC, "openbmc"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(test.expected, test.provider.String()); diff != "" {
				t.Errorf("String() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
