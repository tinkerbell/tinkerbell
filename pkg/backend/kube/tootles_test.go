package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestCidrFromNetmask(t *testing.T) {
	tests := []struct {
		name     string
		netmask  string
		expected string
	}{
		{"Class C", "255.255.255.0", "24"},
		{"Class B", "255.255.0.0", "16"},
		{"Class A", "255.0.0.0", "8"},
		{"/26", "255.255.255.192", "26"},
		{"/27", "255.255.255.224", "27"},
		{"/28", "255.255.255.240", "28"},
		{"/29", "255.255.255.248", "29"},
		{"/30", "255.255.255.252", "30"},
		{"Unknown defaults to /24", "255.255.128.0", "24"},
		{"Empty defaults to /24", "", "24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cidrFromNetmask(tt.netmask)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateBondParameters(t *testing.T) {
	tests := []struct {
		name     string
		mode     int64
		expected map[string]interface{}
	}{
		{
			name: "mode 0 - balance-rr",
			mode: 0,
			expected: map[string]interface{}{
				"bond-mode":        "balance-rr",
				"bond-miimon":      100,
				"bond-use_carrier": 1,
			},
		},
		{
			name: "mode 1 - active-backup",
			mode: 1,
			expected: map[string]interface{}{
				"bond-mode":             "active-backup",
				"bond-miimon":           100,
				"bond-primary_reselect": "always",
				"bond-fail_over_mac":    "none",
			},
		},
		{
			name: "mode 2 - balance-xor",
			mode: 2,
			expected: map[string]interface{}{
				"bond-mode":             "balance-xor",
				"bond-miimon":           100,
				"bond-xmit_hash_policy": "layer2",
			},
		},
		{
			name: "mode 3 - broadcast",
			mode: 3,
			expected: map[string]interface{}{
				"bond-mode":   "broadcast",
				"bond-miimon": 100,
			},
		},
		{
			name: "mode 4 - 802.3ad",
			mode: 4,
			expected: map[string]interface{}{
				"bond-mode":             "802.3ad",
				"bond-miimon":           100,
				"bond-lacp_rate":        "fast",
				"bond-xmit_hash_policy": "layer3+4",
				"bond-ad_select":        "stable",
			},
		},
		{
			name: "mode 5 - balance-tlb",
			mode: 5,
			expected: map[string]interface{}{
				"bond-mode":           "balance-tlb",
				"bond-miimon":         100,
				"bond-tlb_dynamic_lb": 1,
			},
		},
		{
			name: "mode 6 - balance-alb",
			mode: 6,
			expected: map[string]interface{}{
				"bond-mode":             "balance-alb",
				"bond-miimon":           100,
				"bond-rlb_update_delay": 0,
			},
		},
		{
			name: "unknown mode defaults to active-backup",
			mode: 99,
			expected: map[string]interface{}{
				"bond-mode":             "active-backup",
				"bond-miimon":           100,
				"bond-primary_reselect": "always",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateBondParameters(tt.mode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateSubnetsFromIPs(t *testing.T) {
	ipv4DNS := []string{"8.8.8.8", "8.8.4.4"}
	ipv6DNS := []string{"2001:4860:4860::8888"}

	tests := []struct {
		name     string
		ips      []*tinkerbell.MetadataInstanceIP
		expected []interface{}
	}{
		{
			name: "IPv4 with gateway",
			ips: []*tinkerbell.MetadataInstanceIP{
				{
					Address: "192.168.1.10",
					Netmask: "255.255.255.0",
					Gateway: "192.168.1.1",
					Family:  4,
				},
			},
			expected: []interface{}{
				map[string]interface{}{
					"type":            "static",
					"address":         "192.168.1.10/24",
					"gateway":         "192.168.1.1",
					"dns_nameservers": []string{"8.8.8.8", "8.8.4.4"},
				},
			},
		},
		{
			name: "IPv4 without gateway",
			ips: []*tinkerbell.MetadataInstanceIP{
				{
					Address: "192.168.1.10",
					Netmask: "255.255.255.240",
					Family:  4,
				},
			},
			expected: []interface{}{
				map[string]interface{}{
					"type":            "static",
					"address":         "192.168.1.10/28",
					"dns_nameservers": []string{"8.8.8.8", "8.8.4.4"},
				},
			},
		},
		{
			name: "IPv6 with gateway",
			ips: []*tinkerbell.MetadataInstanceIP{
				{
					Address: "2001:db8::10/64",
					Gateway: "2001:db8::1",
					Family:  6,
				},
			},
			expected: []interface{}{
				map[string]interface{}{
					"type":            "static6",
					"address":         "2001:db8::10/64",
					"gateway":         "2001:db8::1",
					"dns_nameservers": []string{"2001:4860:4860::8888"},
				},
			},
		},
		{
			name: "IPv6 without gateway",
			ips: []*tinkerbell.MetadataInstanceIP{
				{
					Address: "2001:db8::10/64",
					Family:  6,
				},
			},
			expected: []interface{}{
				map[string]interface{}{
					"type":            "static6",
					"address":         "2001:db8::10/64",
					"dns_nameservers": []string{"2001:4860:4860::8888"},
				},
			},
		},
		{
			name: "Dual stack",
			ips: []*tinkerbell.MetadataInstanceIP{
				{
					Address: "192.168.1.10",
					Netmask: "255.255.255.0",
					Gateway: "192.168.1.1",
					Family:  4,
				},
				{
					Address: "2001:db8::10/64",
					Gateway: "2001:db8::1",
					Family:  6,
				},
			},
			expected: []interface{}{
				map[string]interface{}{
					"type":            "static",
					"address":         "192.168.1.10/24",
					"gateway":         "192.168.1.1",
					"dns_nameservers": []string{"8.8.8.8", "8.8.4.4"},
				},
				map[string]interface{}{
					"type":            "static6",
					"address":         "2001:db8::10/64",
					"gateway":         "2001:db8::1",
					"dns_nameservers": []string{"2001:4860:4860::8888"},
				},
			},
		},
		{
			name: "No IPs - fallback to DHCP",
			ips:  []*tinkerbell.MetadataInstanceIP{},
			expected: []interface{}{
				map[string]interface{}{"type": "dhcp"},
				map[string]interface{}{"type": "dhcp6"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSubnetsFromIPs(tt.ips, ipv4DNS, ipv6DNS)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateBondingConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		hw       tinkerbell.Hardware
		expected []interface{}
	}{
		{
			name: "802.3ad bond with 2 interfaces",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						BondingMode: 4,
						Instance: &tinkerbell.MetadataInstance{
							Ips: []*tinkerbell.MetadataInstanceIP{
								{
									Address: "192.168.1.10",
									Netmask: "255.255.255.0",
									Gateway: "192.168.1.1",
									Family:  4,
								},
							},
						},
					},
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"},
						},
						{
							DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:02"},
						},
					},
				},
			},
			expected: []interface{}{
				map[string]interface{}{
					"type":        "physical",
					"name":        "eno1",
					"mac_address": "aa:bb:cc:dd:ee:01",
					"mtu":         1500,
				},
				map[string]interface{}{
					"type":        "physical",
					"name":        "eno2",
					"mac_address": "aa:bb:cc:dd:ee:02",
					"mtu":         1500,
				},
				map[string]interface{}{
					"type":            "bond",
					"name":            "bond0",
					"bond_interfaces": []string{"eno1", "eno2"},
					"mtu":             1500,
					"params": map[string]interface{}{
						"bond-mode":             "802.3ad",
						"bond-miimon":           100,
						"bond-lacp_rate":        "fast",
						"bond-xmit_hash_policy": "layer3+4",
						"bond-ad_select":        "stable",
					},
					"subnets": []interface{}{
						map[string]interface{}{
							"type":            "static",
							"address":         "192.168.1.10/24",
							"gateway":         "192.168.1.1",
							"dns_nameservers": []string{"8.8.8.8", "8.8.4.4"},
						},
					},
				},
			},
		},
		{
			name: "bond with no IPs - fallback to DHCP",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						BondingMode: 1,
					},
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"}},
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:02"}},
					},
				},
			},
			expected: []interface{}{
				map[string]interface{}{
					"type":        "physical",
					"name":        "eno1",
					"mac_address": "aa:bb:cc:dd:ee:01",
					"mtu":         1500,
				},
				map[string]interface{}{
					"type":        "physical",
					"name":        "eno2",
					"mac_address": "aa:bb:cc:dd:ee:02",
					"mtu":         1500,
				},
				map[string]interface{}{
					"type":            "bond",
					"name":            "bond0",
					"bond_interfaces": []string{"eno1", "eno2"},
					"mtu":             1500,
					"params": map[string]interface{}{
						"bond-mode":             "active-backup",
						"bond-miimon":           100,
						"bond-primary_reselect": "always",
						"bond-fail_over_mac":    "none",
					},
					"subnets": []interface{}{
						map[string]interface{}{"type": "dhcp"},
						map[string]interface{}{"type": "dhcp6"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateBondingConfiguration(tt.hw)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateNetworkConfig(t *testing.T) {
	tests := []struct {
		name     string
		hw       tinkerbell.Hardware
		validate func(t *testing.T, result interface{})
	}{
		{
			name: "bonding enabled with 2+ interfaces",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						BondingMode: 4,
						Instance: &tinkerbell.MetadataInstance{
							Ips: []*tinkerbell.MetadataInstanceIP{
								{Address: "192.168.1.10", Netmask: "255.255.255.0", Gateway: "192.168.1.1", Family: 4},
							},
						},
					},
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"}},
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:02"}},
					},
				},
			},
			validate: func(t *testing.T, result interface{}) {
				t.Helper()
				config := result.(map[string]interface{})
				assert.Equal(t, 1, config["version"])
				configSlice := config["config"].([]interface{})
				assert.Len(t, configSlice, 3) // 2 physical + 1 bond
			},
		},
		{
			name: "bonding disabled - single interface",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						BondingMode: 0,
						Instance: &tinkerbell.MetadataInstance{
							Ips: []*tinkerbell.MetadataInstanceIP{
								{Address: "192.168.1.10", Netmask: "255.255.255.0", Gateway: "192.168.1.1", Family: 4},
							},
						},
					},
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"}},
					},
				},
			},
			validate: func(t *testing.T, result interface{}) {
				t.Helper()
				config := result.(map[string]interface{})
				assert.Equal(t, 1, config["version"])
				configSlice := config["config"].([]interface{})
				assert.Len(t, configSlice, 1) // single physical interface
			},
		},
		{
			name: "no interfaces - fallback DHCP",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{},
			},
			validate: func(t *testing.T, result interface{}) {
				t.Helper()
				config := result.(map[string]interface{})
				assert.Equal(t, 1, config["version"])
				configSlice := config["config"].([]interface{})
				assert.Len(t, configSlice, 1)
				item := configSlice[0].(map[string]interface{})
				assert.Equal(t, "physical", item["type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateNetworkConfig(tt.hw)
			tt.validate(t, result)
		})
	}
}

func TestToNoCloudInstance(t *testing.T) {
	hw := tinkerbell.Hardware{
		Spec: tinkerbell.HardwareSpec{
			Metadata: &tinkerbell.HardwareMetadata{
				Instance: &tinkerbell.MetadataInstance{
					ID:       "server-001",
					Hostname: "server001.example.com",
				},
			},
			UserData: strPtr("#cloud-config\npackage_update: true\n"),
		},
	}

	result := toNoCloudInstance(hw)

	assert.Equal(t, "server-001", result.Metadata.InstanceID)
	assert.Equal(t, "server001.example.com", result.Metadata.LocalHostname)
	assert.Equal(t, "#cloud-config\npackage_update: true\n", result.Userdata)
	assert.NotNil(t, result.NetworkConfig)
}

func strPtr(s string) *string {
	return &s
}
