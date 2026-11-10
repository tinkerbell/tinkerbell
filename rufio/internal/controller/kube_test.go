package controller_test

import (
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
)

func TestToPowerState(t *testing.T) {
	tests := map[string]struct {
		raw  string
		want bmc.PowerState
	}{
		"redfish On":                {raw: "On", want: bmc.On},
		"lowercase on":              {raw: "on", want: bmc.On},
		"redfish Off":               {raw: "Off", want: bmc.Off},
		"lowercase off":             {raw: "off", want: bmc.Off},
		"surrounding whitespace":    {raw: " Off\n", want: bmc.Off},
		"ipmi on":                   {raw: "Chassis Power is on", want: bmc.On},
		"ipmitool off with newline": {raw: "Chassis Power is off\n", want: bmc.Off},
		"ipmitool on after warning": {
			raw:  "Unable to Get Channel Cipher Suites\nChassis Power is on\n",
			want: bmc.On,
		},
		"ipmitool off after warnings": {
			raw:  "Get HPM.x Capabilities request failed, compcode = d4\nUnable to Get Channel Cipher Suites\nChassis Power is off\n",
			want: bmc.Off,
		},
		"ipmitool warning only": {
			raw:  "Unable to establish IPMI v2 / RMCP+ session\n",
			want: bmc.Unknown,
		},
		"transition PoweringOff": {raw: "PoweringOff", want: bmc.Unknown},
		"transition PoweringOn":  {raw: "PoweringOn", want: bmc.Unknown},
		"transition Resetting":   {raw: "Resetting", want: bmc.Unknown},
		"Unavailable":            {raw: "Unavailable", want: bmc.Unknown},
		"None":                   {raw: "None", want: bmc.Unknown},
		"empty":                  {raw: "", want: bmc.Unknown},
		"unrelated":              {raw: "unknown", want: bmc.Unknown},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := controller.ToPowerState(tt.raw); got != tt.want {
				t.Fatalf("ToPowerState(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
