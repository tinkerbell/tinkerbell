package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
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
		{"/31", "255.255.255.254", "31"},
		{"/32", "255.255.255.255", "32"},
		{"/17", "255.255.128.0", "17"},
		{"/25", "255.255.255.128", "25"},
		{"Empty returns empty", "", ""},
		{"Invalid format returns empty", "255.255", ""},
		{"Invalid characters return empty", "255.255.abc.0", ""},
		{"Out of range returns empty", "255.255.256.0", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cidrFromNetmask(tt.netmask)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// V2 Tests

func TestGenerateBondParametersV2(t *testing.T) {
	tests := []struct {
		name     string
		mode     int64
		expected map[string]interface{}
	}{
		{
			name: "mode 0 - balance-rr",
			mode: 0,
			expected: map[string]interface{}{
				"mode":                 "balance-rr",
				"mii-monitor-interval": 100,
			},
		},
		{
			name: "mode 1 - active-backup",
			mode: 1,
			expected: map[string]interface{}{
				"mode":                    "active-backup",
				"mii-monitor-interval":    100,
				"primary-reselect-policy": "always",
				"fail-over-mac-policy":    "none",
			},
		},
		{
			name: "mode 2 - balance-xor",
			mode: 2,
			expected: map[string]interface{}{
				"mode":                 "balance-xor",
				"mii-monitor-interval": 100,
				"transmit-hash-policy": "layer2",
			},
		},
		{
			name: "mode 3 - broadcast",
			mode: 3,
			expected: map[string]interface{}{
				"mode":                 "broadcast",
				"mii-monitor-interval": 100,
			},
		},
		{
			name: "mode 4 - 802.3ad",
			mode: 4,
			expected: map[string]interface{}{
				"mode":                 "802.3ad",
				"mii-monitor-interval": 100,
				"lacp-rate":            "fast",
				"transmit-hash-policy": "layer3+4",
				"ad-select":            "stable",
			},
		},
		{
			name: "mode 5 - balance-tlb",
			mode: 5,
			expected: map[string]interface{}{
				"mode":                 "balance-tlb",
				"mii-monitor-interval": 100,
			},
		},
		{
			name: "mode 6 - balance-alb",
			mode: 6,
			expected: map[string]interface{}{
				"mode":                 "balance-alb",
				"mii-monitor-interval": 100,
			},
		},
		{
			name: "unknown mode defaults to active-backup",
			mode: 99,
			expected: map[string]interface{}{
				"mode":                    "active-backup",
				"mii-monitor-interval":    100,
				"primary-reselect-policy": "always",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateBondParametersV2(tt.mode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateAddressConfigV2(t *testing.T) {
	ipv4DNS := []string{"8.8.8.8", "8.8.4.4"}
	ipv6DNS := []string{"2001:4860:4860::8888"}

	tests := []struct {
		name               string
		ips                []*tinkerbell.MetadataInstanceIP
		expectedAddresses  []string
		expectedGateway4   string
		expectedGateway6   string
		expectedNameserver []string
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
			expectedAddresses:  []string{"192.168.1.10/24"},
			expectedGateway4:   "192.168.1.1",
			expectedGateway6:   "",
			expectedNameserver: []string{"8.8.8.8", "8.8.4.4", "2001:4860:4860::8888"},
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
			expectedAddresses:  []string{"2001:db8::10/64"},
			expectedGateway4:   "",
			expectedGateway6:   "2001:db8::1",
			expectedNameserver: []string{"8.8.8.8", "8.8.4.4", "2001:4860:4860::8888"},
		},
		{
			name: "Dual stack with gateways",
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
			expectedAddresses:  []string{"192.168.1.10/24", "2001:db8::10/64"},
			expectedGateway4:   "192.168.1.1",
			expectedGateway6:   "2001:db8::1",
			expectedNameserver: []string{"8.8.8.8", "8.8.4.4", "2001:4860:4860::8888"},
		},
		{
			name: "Multiple IPv4 addresses - first gateway wins",
			ips: []*tinkerbell.MetadataInstanceIP{
				{
					Address: "192.168.1.10",
					Netmask: "255.255.255.0",
					Gateway: "192.168.1.1",
					Family:  4,
				},
				{
					Address: "10.0.0.10",
					Netmask: "255.255.255.0",
					Gateway: "10.0.0.1",
					Family:  4,
				},
			},
			expectedAddresses:  []string{"192.168.1.10/24", "10.0.0.10/24"},
			expectedGateway4:   "192.168.1.1", // First gateway
			expectedGateway6:   "",
			expectedNameserver: []string{"8.8.8.8", "8.8.4.4", "2001:4860:4860::8888"},
		},
		{
			name:               "No IPs",
			ips:                []*tinkerbell.MetadataInstanceIP{},
			expectedAddresses:  []string{},
			expectedGateway4:   "",
			expectedGateway6:   "",
			expectedNameserver: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addresses, gateway4, gateway6, nameservers := generateAddressConfigV2(tt.ips, ipv4DNS, ipv6DNS)
			assert.Equal(t, tt.expectedAddresses, addresses)
			assert.Equal(t, tt.expectedGateway4, gateway4)
			assert.Equal(t, tt.expectedGateway6, gateway6)
			assert.Equal(t, tt.expectedNameserver, nameservers)
		})
	}
}

func TestGenerateBondingConfigurationV2(t *testing.T) {
	tests := []struct {
		name              string
		hw                tinkerbell.Hardware
		validateEthernets func(t *testing.T, ethernets map[string]interface{})
		validateBonds     func(t *testing.T, bonds map[string]interface{})
	}{
		{
			name: "802.3ad bond with 2 interfaces and static IPs",
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
								{
									Address: "2001:db8::10/64",
									Gateway: "2001:db8::1",
									Family:  6,
								},
							},
						},
					},
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"}},
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:02"}},
					},
				},
			},
			validateEthernets: func(t *testing.T, ethernets map[string]interface{}) {
				t.Helper()
				assert.Len(t, ethernets, 2)

				iface1 := ethernets["bond0phy0"].(map[string]interface{})
				assert.Equal(t, false, iface1["dhcp4"])
				match := iface1["match"].(map[string]interface{})
				assert.Equal(t, "aa:bb:cc:dd:ee:01", match["macaddress"])
				_, hasSetName := iface1["set-name"]
				assert.False(t, hasSetName, "set-name should not be present")

				iface2 := ethernets["bond0phy1"].(map[string]interface{})
				assert.Equal(t, false, iface2["dhcp4"])
				match2 := iface2["match"].(map[string]interface{})
				assert.Equal(t, "aa:bb:cc:dd:ee:02", match2["macaddress"])
			},
			validateBonds: func(t *testing.T, bonds map[string]interface{}) {
				t.Helper()
				bond0 := bonds["bond0"].(map[string]interface{})

				// Check interfaces
				interfaces := bond0["interfaces"].([]string)
				assert.Equal(t, []string{"bond0phy0", "bond0phy1"}, interfaces)

				// Check parameters
				params := bond0["parameters"].(map[string]interface{})
				assert.Equal(t, "802.3ad", params["mode"])
				assert.Equal(t, 100, params["mii-monitor-interval"])
				assert.Equal(t, "fast", params["lacp-rate"])
				assert.Equal(t, "layer3+4", params["transmit-hash-policy"])
				assert.Equal(t, "stable", params["ad-select"])

				// Check addresses
				addresses := bond0["addresses"].([]string)
				assert.Equal(t, []string{"192.168.1.10/24", "2001:db8::10/64"}, addresses)

				// Check gateways
				assert.Equal(t, "192.168.1.1", bond0["gateway4"])
				assert.Equal(t, "2001:db8::1", bond0["gateway6"])

				// No nameservers configured in this test - nameservers field should not be set
			},
		},
		{
			name: "active-backup bond with DHCP",
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
			validateEthernets: func(t *testing.T, ethernets map[string]interface{}) {
				t.Helper()
				assert.Len(t, ethernets, 2)
			},
			validateBonds: func(t *testing.T, bonds map[string]interface{}) {
				t.Helper()
				bond0 := bonds["bond0"].(map[string]interface{})

				// Should have DHCP when no static IPs
				assert.Equal(t, true, bond0["dhcp4"])

				// Check parameters for active-backup
				params := bond0["parameters"].(map[string]interface{})
				assert.Equal(t, "active-backup", params["mode"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ethernets, bonds := generateBondingConfigurationV2(tt.hw)
			tt.validateEthernets(t, ethernets)
			tt.validateBonds(t, bonds)
		})
	}
}

func TestGenerateNetworkConfigV2(t *testing.T) {
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
				network := config["network"].(map[string]interface{})

				// Check version
				assert.Equal(t, 2, network["version"])

				// Check ethernets exist
				ethernets := network["ethernets"].(map[string]interface{})
				assert.Len(t, ethernets, 2)

				// Check bonds exist
				bonds := network["bonds"].(map[string]interface{})
				assert.Len(t, bonds, 1)
				assert.Contains(t, bonds, "bond0")
			},
		},
		{
			name: "no interfaces - no network config",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{},
			},
			validate: func(t *testing.T, result interface{}) {
				t.Helper()
				config := result.(map[string]interface{})
				network := config["network"].(map[string]interface{})

				assert.Equal(t, 2, network["version"])

				// No interfaces defined means no ethernets config
				// Let cloud-init handle its default DHCP behavior
				_, hasEthernets := network["ethernets"]
				assert.False(t, hasEthernets)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateNetworkConfigV2(tt.hw)
			tt.validate(t, result)
		})
	}
}

func TestToNoCloudInstance(t *testing.T) {
	tests := []struct {
		name     string
		hw       tinkerbell.Hardware
		validate func(t *testing.T, instance interface{})
	}{
		{
			name: "complete hardware resource",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						Instance: &tinkerbell.MetadataInstance{
							ID:       "server-001",
							Hostname: "server001.example.com",
							Ips: []*tinkerbell.MetadataInstanceIP{
								{Address: "192.168.1.10", Netmask: "255.255.255.0", Gateway: "192.168.1.1", Family: 4},
							},
						},
					},
					UserData: strPtr("#cloud-config\npackage_update: true\n"),
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"}},
					},
				},
			},
			validate: func(t *testing.T, instance interface{}) {
				t.Helper()
				i := instance.(map[string]interface{})
				assert.Equal(t, "server-001", i["InstanceID"])
				assert.Equal(t, "server001.example.com", i["LocalHostname"])
				assert.Equal(t, "#cloud-config\npackage_update: true\n", i["Userdata"])
				assert.NotNil(t, i["NetworkConfig"])
			},
		},
		{
			name: "minimal hardware resource",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{},
			},
			validate: func(t *testing.T, instance interface{}) {
				t.Helper()
				i := instance.(map[string]interface{})
				assert.Equal(t, "", i["InstanceID"])
				assert.Equal(t, "", i["LocalHostname"])
				assert.Equal(t, "", i["Userdata"])
				assert.NotNil(t, i["NetworkConfig"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backend{}
			result := b.toNoCloudInstance(tt.hw)

			// Convert to map for validation
			resultMap := map[string]interface{}{
				"InstanceID":    result.Metadata.InstanceID,
				"LocalHostname": result.Metadata.LocalHostname,
				"Userdata":      result.Userdata,
				"NetworkConfig": result.NetworkConfig,
			}
			tt.validate(t, resultMap)
		})
	}
}


func TestGetNameServers(t *testing.T) {
	tests := []struct {
		name         string
		hw           tinkerbell.Hardware
		expectedIPv4 []string
		expectedIPv6 []string
	}{
		{
			name: "both IPv4 and IPv6 nameservers",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC: "aa:bb:cc:dd:ee:01",
								NameServers: []string{
									"8.8.8.8",
									"8.8.4.4",
									"2001:4860:4860::8888",
								},
							},
						},
					},
				},
			},
			expectedIPv4: []string{"8.8.8.8", "8.8.4.4"},
			expectedIPv6: []string{"2001:4860:4860::8888"},
		},
		{
			name: "only IPv4 nameservers",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{
							DHCP: &tinkerbell.DHCP{
								MAC:         "aa:bb:cc:dd:ee:01",
								NameServers: []string{"1.1.1.1", "1.0.0.1"},
							},
						},
					},
				},
			},
			expectedIPv4: []string{"1.1.1.1", "1.0.0.1"},
			expectedIPv6: nil,
		},
		{
			name: "no nameservers",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"}},
					},
				},
			},
			expectedIPv4: nil,
			expectedIPv6: nil,
		},
		{
			name: "no interfaces",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{},
			},
			expectedIPv4: nil,
			expectedIPv6: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ipv4, ipv6 := getNameServers(tt.hw)
			assert.Equal(t, tt.expectedIPv4, ipv4)
			assert.Equal(t, tt.expectedIPv6, ipv6)
		})
	}
}



func strPtr(s string) *string {
	return &s
}
