package bmc

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestString(t *testing.T) {
	tests := map[string]struct {
		value    string
		expected string
	}{
		"PowerOn":         {PowerActionOn.String(), "on"},
		"PowerHardOff":    {PowerActionHardOff.String(), "off"},
		"PowerSoftOff":    {PowerActionSoftOff.String(), "soft"},
		"PowerCycle":      {PowerActionCycle.String(), "cycle"},
		"PowerReset":      {PowerActionReset.String(), "reset"},
		"PowerStatus":     {PowerActionStatus.String(), "status"},
		"BootDevicePXE":   {BootDevicePXE.String(), "pxe"},
		"BootDeviceDisk":  {BootDeviceDisk.String(), "disk"},
		"BootDeviceBIOS":  {BootDeviceBIOS.String(), "bios"},
		"BootDeviceCDROM": {BootDeviceCDROM.String(), "cdrom"},
		"BootDeviceSafe":  {BootDeviceSafe.String(), "safe"},
		"VirtualMediaCD":  {VirtualMediaCD.String(), "CD"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if diff := cmp.Diff(test.expected, test.value); diff != "" {
				t.Errorf("String() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
