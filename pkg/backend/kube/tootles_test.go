package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	tinkerbell "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

func TestGenerateBondParametersV2(t *testing.T) {
	tests := []struct {
		name     string
		mode     int64
		validate func(t *testing.T, result data.BondParameters)
	}{
		{
			name: "mode 0 - balance-rr",
			mode: 0,
			validate: func(t *testing.T, result data.BondParameters) {
				t.Helper()
				assert.Equal(t, "balance-rr", result.Mode)
				assert.Equal(t, 100, result.MIIMonitorInterval)
			},
		},
		{
			name: "mode 1 - active-backup",
			mode: 1,
			validate: func(t *testing.T, result data.BondParameters) {
				t.Helper()
				assert.Equal(t, "active-backup", result.Mode)
				assert.Equal(t, 100, result.MIIMonitorInterval)
				assert.Equal(t, "always", result.PrimaryReselectPolicy)
				assert.Equal(t, "none", result.FailOverMACPolicy)
			},
		},
		{
			name: "mode 2 - balance-xor",
			mode: 2,
			validate: func(t *testing.T, result data.BondParameters) {
				t.Helper()
				assert.Equal(t, "balance-xor", result.Mode)
				assert.Equal(t, 100, result.MIIMonitorInterval)
				assert.Equal(t, "layer2", result.TransmitHashPolicy)
			},
		},
		{
			name: "mode 3 - broadcast",
			mode: 3,
			validate: func(t *testing.T, result data.BondParameters) {
				t.Helper()
				assert.Equal(t, "broadcast", result.Mode)
				assert.Equal(t, 100, result.MIIMonitorInterval)
			},
		},
		{
			name: "mode 4 - 802.3ad",
			mode: 4,
			validate: func(t *testing.T, result data.BondParameters) {
				t.Helper()
				assert.Equal(t, "802.3ad", result.Mode)
				assert.Equal(t, 100, result.MIIMonitorInterval)
				assert.Equal(t, "fast", result.LACPRate)
				assert.Equal(t, "layer3+4", result.TransmitHashPolicy)
				assert.Equal(t, "stable", result.ADSelect)
			},
		},
		{
			name: "mode 5 - balance-tlb",
			mode: 5,
			validate: func(t *testing.T, result data.BondParameters) {
				t.Helper()
				assert.Equal(t, "balance-tlb", result.Mode)
				assert.Equal(t, 100, result.MIIMonitorInterval)
			},
		},
		{
			name: "mode 6 - balance-alb",
			mode: 6,
			validate: func(t *testing.T, result data.BondParameters) {
				t.Helper()
				assert.Equal(t, "balance-alb", result.Mode)
				assert.Equal(t, 100, result.MIIMonitorInterval)
			},
		},
		{
			name: "unknown mode defaults to active-backup",
			mode: 99,
			validate: func(t *testing.T, result data.BondParameters) {
				t.Helper()
				assert.Equal(t, "active-backup", result.Mode)
				assert.Equal(t, 100, result.MIIMonitorInterval)
				assert.Equal(t, "always", result.PrimaryReselectPolicy)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateBondParametersV2(tt.mode)
			tt.validate(t, result)
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

func TestMultipleBonds(t *testing.T) {
	tests := []struct {
		name     string
		hw       tinkerbell.Hardware
		validate func(t *testing.T, result *data.NetworkConfig)
	}{
		{
			name: "two bonds with different modes",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						Instance: &tinkerbell.MetadataInstance{
							Tags: []string{"bond0:mode4", "bond1:mode1"},
						},
					},
					Interfaces: []tinkerbell.Interface{
						// Bond 0 - 802.3ad
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:01",
							IfaceName: "bond0phy0",
							IP: &tinkerbell.IP{
								Address: "192.168.1.10",
								Netmask: "255.255.255.0",
								Gateway: "192.168.1.1",
								Family:  4,
							},
							NameServers: []string{"8.8.8.8"},
						}},
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:02",
							IfaceName: "bond0phy1",
						}},
						// Bond 1 - active-backup
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:03",
							IfaceName: "bond1phy0",
							IP: &tinkerbell.IP{
								Address: "10.0.0.10",
								Netmask: "255.255.255.0",
								Gateway: "10.0.0.1",
								Family:  4,
							},
						}},
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:04",
							IfaceName: "bond1phy1",
						}},
					},
				},
			},
			validate: func(t *testing.T, result *data.NetworkConfig) {
				t.Helper()
				assert.NotNil(t, result)
				assert.Equal(t, 2, result.Network.Version)

				// Check ethernets (4 physical interfaces)
				assert.Len(t, result.Network.Ethernets, 4)
				assert.Contains(t, result.Network.Ethernets, "bond0phy0")
				assert.Contains(t, result.Network.Ethernets, "bond0phy1")
				assert.Contains(t, result.Network.Ethernets, "bond1phy0")
				assert.Contains(t, result.Network.Ethernets, "bond1phy1")

				// Check bonds
				assert.Len(t, result.Network.Bonds, 2)

				// Verify bond0 (802.3ad)
				bond0 := result.Network.Bonds["bond0"]
				assert.Equal(t, "802.3ad", bond0.Parameters.Mode)
				assert.Equal(t, []string{"192.168.1.10/24"}, bond0.Addresses)
				assert.Equal(t, "192.168.1.1", bond0.Gateway4)
				assert.NotNil(t, bond0.Nameservers)
				assert.Equal(t, []string{"8.8.8.8"}, bond0.Nameservers.Addresses)

				// Verify bond1 (active-backup)
				bond1 := result.Network.Bonds["bond1"]
				assert.Equal(t, "active-backup", bond1.Parameters.Mode)
				assert.Equal(t, []string{"10.0.0.10/24"}, bond1.Addresses)
				assert.Equal(t, "10.0.0.1", bond1.Gateway4)
			},
		},
		{
			name: "bond with unbonded interface",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						Instance: &tinkerbell.MetadataInstance{
							Tags: []string{"bond0:mode4"},
						},
					},
					Interfaces: []tinkerbell.Interface{
						// Bond 0
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:01",
							IfaceName: "bond0phy0",
							IP: &tinkerbell.IP{
								Address: "192.168.1.10",
								Netmask: "255.255.255.0",
								Gateway: "192.168.1.1",
								Family:  4,
							},
						}},
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:02",
							IfaceName: "bond0phy1",
						}},
						// Unbonded interface
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:03",
							IfaceName: "mgmt0",
							IP: &tinkerbell.IP{
								Address: "172.16.0.10",
								Netmask: "255.255.255.0",
								Gateway: "172.16.0.1",
								Family:  4,
							},
							NameServers: []string{"172.16.0.1"},
						}},
					},
				},
			},
			validate: func(t *testing.T, result *data.NetworkConfig) {
				t.Helper()
				assert.NotNil(t, result)

				// Check ethernets (2 for bond + 1 unbonded)
				assert.Len(t, result.Network.Ethernets, 3)
				assert.Contains(t, result.Network.Ethernets, "bond0phy0")
				assert.Contains(t, result.Network.Ethernets, "bond0phy1")
				assert.Contains(t, result.Network.Ethernets, "mgmt0")

				// Verify unbonded interface
				mgmt := result.Network.Ethernets["mgmt0"]
				assert.Equal(t, []string{"172.16.0.10/24"}, mgmt.Addresses)
				assert.Equal(t, "172.16.0.1", mgmt.Gateway4)
				assert.NotNil(t, mgmt.Nameservers)
				assert.Equal(t, []string{"172.16.0.1"}, mgmt.Nameservers.Addresses)

				// Check bond
				assert.Len(t, result.Network.Bonds, 1)
				bond0 := result.Network.Bonds["bond0"]
				assert.Equal(t, "802.3ad", bond0.Parameters.Mode)
			},
		},
		{
			name: "bond with DHCP (no static IP)",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						Instance: &tinkerbell.MetadataInstance{
							Tags: []string{"bond0:mode1"},
						},
					},
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:01",
							IfaceName: "bond0phy0",
						}},
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:02",
							IfaceName: "bond0phy1",
						}},
					},
				},
			},
			validate: func(t *testing.T, result *data.NetworkConfig) {
				t.Helper()
				assert.NotNil(t, result)

				// Bond should use DHCP
				bond0 := result.Network.Bonds["bond0"]
				assert.True(t, bond0.Dhcp4)
				assert.Empty(t, bond0.Addresses)
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

func TestGenerateNetworkConfigV2(t *testing.T) {
	tests := []struct {
		name     string
		hw       tinkerbell.Hardware
		validate func(t *testing.T, result *data.NetworkConfig)
	}{
		{
			name: "bond with interfaces using new convention",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						Instance: &tinkerbell.MetadataInstance{
							Tags: []string{"bond0:mode4"},
						},
					},
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:01",
							IfaceName: "bond0phy0",
							IP: &tinkerbell.IP{
								Address: "192.168.1.10",
								Netmask: "255.255.255.0",
								Gateway: "192.168.1.1",
								Family:  4,
							},
						}},
						{DHCP: &tinkerbell.DHCP{
							MAC:       "aa:bb:cc:dd:ee:02",
							IfaceName: "bond0phy1",
						}},
					},
				},
			},
			validate: func(t *testing.T, result *data.NetworkConfig) {
				t.Helper()

				// Check that result is not nil
				assert.NotNil(t, result)

				// Check version
				assert.Equal(t, 2, result.Network.Version)

				// Check ethernets exist
				assert.Len(t, result.Network.Ethernets, 2)

				// Check bonds exist
				assert.Len(t, result.Network.Bonds, 1)
				assert.Contains(t, result.Network.Bonds, "bond0")
			},
		},
		{
			name: "no interfaces - returns nil",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{},
			},
			validate: func(t *testing.T, result *data.NetworkConfig) {
				t.Helper()

				// Should return nil when no bonding is configured
				assert.Nil(t, result)
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
		validate func(t *testing.T, result data.NoCloudInstance)
	}{
		{
			name: "complete hardware resource with all metadata fields",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						BondingMode: 4,
						Facility: &tinkerbell.MetadataFacility{
							FacilityCode: "ewr1",
							PlanSlug:     "c3.small.x86",
						},
						Instance: &tinkerbell.MetadataInstance{
							ID:       "server-001",
							Hostname: "server001.example.com",
							Tags:     []string{"production", "web"},
							SSHKeys:  []string{"ssh-rsa AAAAB3NzaC1yc2EA..."},
							Ips: []*tinkerbell.MetadataInstanceIP{
								{Address: "192.168.1.10", Netmask: "255.255.255.0", Gateway: "192.168.1.1", Family: 4, Public: false},
								{Address: "147.75.1.100", Netmask: "255.255.255.240", Gateway: "147.75.1.97", Family: 4, Public: true},
								{Address: "2001:db8::10/64", Gateway: "2001:db8::1", Family: 6},
							},
							OperatingSystem: &tinkerbell.MetadataInstanceOperatingSystem{
								Slug:     "ubuntu_20_04",
								Distro:   "ubuntu",
								Version:  "20.04",
								ImageTag: "latest",
							},
						},
					},
					UserData: strPtr("#cloud-config\npackage_update: true\n"),
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"}},
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:02"}},
					},
				},
			},
			validate: func(t *testing.T, result data.NoCloudInstance) {
				t.Helper()
				assert.Equal(t, "server-001", result.Metadata.InstanceID)
				assert.Equal(t, "server001.example.com", result.Metadata.Hostname)
				assert.Equal(t, "server001.example.com", result.Metadata.LocalHostname)
				assert.Equal(t, []string{"production", "web"}, result.Metadata.Tags)
				assert.Equal(t, []string{"ssh-rsa AAAAB3NzaC1yc2EA..."}, result.Metadata.PublicKeys)
				assert.Equal(t, "192.168.1.10", result.Metadata.LocalIPv4)
				assert.Equal(t, "147.75.1.100", result.Metadata.PublicIPv4)
				assert.Equal(t, "2001:db8::10/64", result.Metadata.PublicIPv6)
				assert.Equal(t, "ewr1", result.Metadata.Facility)
				assert.Equal(t, "c3.small.x86", result.Metadata.Plan)
				assert.Equal(t, "ubuntu_20_04", result.Metadata.OperatingSystem.Slug)
				assert.Equal(t, "ubuntu", result.Metadata.OperatingSystem.Distro)
				assert.Equal(t, "20.04", result.Metadata.OperatingSystem.Version)
				assert.Equal(t, "latest", result.Metadata.OperatingSystem.ImageTag)
				assert.Equal(t, "#cloud-config\npackage_update: true\n", result.Userdata)
				// NetworkConfig will be nil since interfaces don't have IfaceName set
				assert.Nil(t, result.NetworkConfig)
			},
		},
		{
			name: "hardware resource with bonding and minimal metadata",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						BondingMode: 4,
						Instance: &tinkerbell.MetadataInstance{
							ID:       "server-002",
							Hostname: "server002.example.com",
							Ips: []*tinkerbell.MetadataInstanceIP{
								{Address: "192.168.1.10", Netmask: "255.255.255.0", Gateway: "192.168.1.1", Family: 4},
							},
						},
					},
					UserData: strPtr("#cloud-config\npackage_update: true\n"),
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"}},
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:02"}},
					},
				},
			},
			validate: func(t *testing.T, result data.NoCloudInstance) {
				t.Helper()
				assert.Equal(t, "server-002", result.Metadata.InstanceID)
				assert.Equal(t, "server002.example.com", result.Metadata.LocalHostname)
				assert.Equal(t, "#cloud-config\npackage_update: true\n", result.Userdata)
				// NetworkConfig will be nil since interfaces don't have IfaceName set
				assert.Nil(t, result.NetworkConfig)
				// Optional fields should be empty
				assert.Nil(t, result.Metadata.Tags)
				assert.Nil(t, result.Metadata.PublicKeys)
				assert.Equal(t, "", result.Metadata.Facility)
				assert.Equal(t, "", result.Metadata.Plan)
			},
		},
		{
			name: "hardware resource without bonding",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{
					Metadata: &tinkerbell.HardwareMetadata{
						Facility: &tinkerbell.MetadataFacility{
							FacilityCode: "sjc1",
						},
						Instance: &tinkerbell.MetadataInstance{
							ID:       "server-003",
							Hostname: "server003.example.com",
							Ips: []*tinkerbell.MetadataInstanceIP{
								{Address: "10.0.0.10", Netmask: "255.255.255.0", Family: 4, Public: false},
							},
						},
					},
					Interfaces: []tinkerbell.Interface{
						{DHCP: &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:01"}},
					},
				},
			},
			validate: func(t *testing.T, result data.NoCloudInstance) {
				t.Helper()
				assert.Equal(t, "server-003", result.Metadata.InstanceID)
				assert.Equal(t, "server003.example.com", result.Metadata.LocalHostname)
				assert.Equal(t, "10.0.0.10", result.Metadata.LocalIPv4)
				assert.Equal(t, "sjc1", result.Metadata.Facility)
				assert.Nil(t, result.NetworkConfig)
			},
		},
		{
			name: "minimal hardware resource",
			hw: tinkerbell.Hardware{
				Spec: tinkerbell.HardwareSpec{},
			},
			validate: func(t *testing.T, result data.NoCloudInstance) {
				t.Helper()
				assert.Equal(t, "", result.Metadata.InstanceID)
				assert.Equal(t, "", result.Metadata.LocalHostname)
				assert.Equal(t, "", result.Userdata)
				assert.Nil(t, result.NetworkConfig)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Backend{}
			result := b.toNoCloudInstance(tt.hw)
			tt.validate(t, result)
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
