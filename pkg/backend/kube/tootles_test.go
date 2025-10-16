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

func TestGenerateBondingConfigurationV2(t *testing.T) {
	tests := []struct {
		name              string
		hw                tinkerbell.Hardware
		validateEthernets func(t *testing.T, ethernets map[string]data.EthernetConfig)
		validateBonds     func(t *testing.T, bonds map[string]data.BondConfig)
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
			validateEthernets: func(t *testing.T, ethernets map[string]data.EthernetConfig) {
				t.Helper()
				assert.Len(t, ethernets, 2)

				iface1 := ethernets["bond0phy0"]
				assert.Equal(t, false, iface1.Dhcp4)
				assert.NotNil(t, iface1.Match)
				assert.Equal(t, "aa:bb:cc:dd:ee:01", iface1.Match.MACAddress)
				assert.Equal(t, "bond0phy0", iface1.SetName)

				iface2 := ethernets["bond0phy1"]
				assert.Equal(t, false, iface2.Dhcp4)
				assert.NotNil(t, iface2.Match)
				assert.Equal(t, "aa:bb:cc:dd:ee:02", iface2.Match.MACAddress)
				assert.Equal(t, "bond0phy1", iface2.SetName)
			},
			validateBonds: func(t *testing.T, bonds map[string]data.BondConfig) {
				t.Helper()
				bond0 := bonds["bond0"]

				// Check interfaces
				assert.Equal(t, []string{"bond0phy0", "bond0phy1"}, bond0.Interfaces)

				// Check parameters
				assert.Equal(t, "802.3ad", bond0.Parameters.Mode)
				assert.Equal(t, 100, bond0.Parameters.MIIMonitorInterval)
				assert.Equal(t, "fast", bond0.Parameters.LACPRate)
				assert.Equal(t, "layer3+4", bond0.Parameters.TransmitHashPolicy)
				assert.Equal(t, "stable", bond0.Parameters.ADSelect)

				// Check addresses
				assert.Equal(t, []string{"192.168.1.10/24", "2001:db8::10/64"}, bond0.Addresses)

				// Check gateways
				assert.Equal(t, "192.168.1.1", bond0.Gateway4)
				assert.Equal(t, "2001:db8::1", bond0.Gateway6)

				// No nameservers configured in this test - nameservers field should be nil
				assert.Nil(t, bond0.Nameservers)
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
			validateEthernets: func(t *testing.T, ethernets map[string]data.EthernetConfig) {
				t.Helper()
				assert.Len(t, ethernets, 2)
			},
			validateBonds: func(t *testing.T, bonds map[string]data.BondConfig) {
				t.Helper()
				bond0 := bonds["bond0"]

				// Should have DHCP when no static IPs
				assert.Equal(t, true, bond0.Dhcp4)

				// Check parameters for active-backup
				assert.Equal(t, "active-backup", bond0.Parameters.Mode)
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
		validate func(t *testing.T, result *data.NetworkConfig)
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
				assert.NotNil(t, result.NetworkConfig)
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
				assert.NotNil(t, result.NetworkConfig)
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
