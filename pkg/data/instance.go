package data

// Ec2Instance is a struct that contains the hardware data exposed from the EC2 API endpoints. For
// an explanation of the endpoints refer to the AWS EC2 Ec2Instance Metadata documentation.
//
//	https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-categories.html
//
// Note not all AWS EC2 Ec2Instance Metadata categories are supported as some are not applicable.
// Deviations from the AWS EC2 Ec2Instance Metadata should be documented here.
type Ec2Instance struct {
	Userdata string
	Metadata Metadata
}

// Metadata is a part of Instance.
type Metadata struct {
	InstanceID      string
	Hostname        string
	LocalHostname   string
	IQN             string
	Plan            string
	Facility        string
	Tags            []string
	PublicKeys      []string
	PublicIPv4      string
	PublicIPv6      string
	LocalIPv4       string
	OperatingSystem OperatingSystem
}

// OperatingSystem is part of Metadata.
type OperatingSystem struct {
	Slug              string
	Distro            string
	Version           string
	ImageTag          string
	LicenseActivation LicenseActivation
}

// LicenseActivation is part of OperatingSystem.
type LicenseActivation struct {
	State string
}

// NoCloudInstance is a struct that contains the hardware data exposed from the NoCloud API endpoints.
// It supports network bonding and IPv6 configuration for bare metal servers.
type NoCloudInstance struct {
	Userdata      string
	Metadata      Metadata
	NetworkConfig interface{} // Network Config data (typically NetworkConfigV2)
	// Note: Uses interface{} for flexibility in YAML marshaling. The actual structure
	// follows the NetworkConfigV2 format defined below.
}

// NetworkConfigV2 represents a Network Config Version 2 configuration.
// Based on https://cloudinit.readthedocs.io/en/latest/reference/network-config-format-v2.html
type NetworkConfigV2 struct {
	Network NetworkSpec `json:"network" yaml:"network"`
}

// NetworkSpec contains the network configuration specification.
type NetworkSpec struct {
	Version   int                       `json:"version" yaml:"version"`
	Ethernets map[string]EthernetConfig `json:"ethernets,omitempty" yaml:"ethernets,omitempty"`
	Bonds     map[string]BondConfig     `json:"bonds,omitempty" yaml:"bonds,omitempty"`
	Bridges   map[string]BridgeConfig   `json:"bridges,omitempty" yaml:"bridges,omitempty"`
	Vlans     map[string]VlanConfig     `json:"vlans,omitempty" yaml:"vlans,omitempty"`
	Renderer  string                    `json:"renderer,omitempty" yaml:"renderer,omitempty"`
}

// EthernetConfig represents an ethernet device configuration.
type EthernetConfig struct {
	Match       *MatchConfig       `json:"match,omitempty" yaml:"match,omitempty"`
	SetName     string             `json:"set-name,omitempty" yaml:"set-name,omitempty"`
	Dhcp4       bool               `json:"dhcp4,omitempty" yaml:"dhcp4,omitempty"`
	Dhcp6       bool               `json:"dhcp6,omitempty" yaml:"dhcp6,omitempty"`
	Addresses   []string           `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	Gateway4    string             `json:"gateway4,omitempty" yaml:"gateway4,omitempty"`
	Gateway6    string             `json:"gateway6,omitempty" yaml:"gateway6,omitempty"`
	Nameservers *NameserversConfig `json:"nameservers,omitempty" yaml:"nameservers,omitempty"`
	Routes      []RouteConfig      `json:"routes,omitempty" yaml:"routes,omitempty"`
	MTU         int                `json:"mtu,omitempty" yaml:"mtu,omitempty"`
}

// BondConfig represents a bond device configuration.
type BondConfig struct {
	Interfaces  []string           `json:"interfaces" yaml:"interfaces"`
	Parameters  BondParameters     `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Dhcp4       bool               `json:"dhcp4,omitempty" yaml:"dhcp4,omitempty"`
	Dhcp6       bool               `json:"dhcp6,omitempty" yaml:"dhcp6,omitempty"`
	Addresses   []string           `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	Gateway4    string             `json:"gateway4,omitempty" yaml:"gateway4,omitempty"`
	Gateway6    string             `json:"gateway6,omitempty" yaml:"gateway6,omitempty"`
	Nameservers *NameserversConfig `json:"nameservers,omitempty" yaml:"nameservers,omitempty"`
	Routes      []RouteConfig      `json:"routes,omitempty" yaml:"routes,omitempty"`
	MTU         int                `json:"mtu,omitempty" yaml:"mtu,omitempty"`
}

// BridgeConfig represents a bridge device configuration.
type BridgeConfig struct {
	Interfaces  []string           `json:"interfaces" yaml:"interfaces"`
	Parameters  BridgeParameters   `json:"parameters,omitempty" yaml:"parameters,omitempty"`
	Dhcp4       bool               `json:"dhcp4,omitempty" yaml:"dhcp4,omitempty"`
	Dhcp6       bool               `json:"dhcp6,omitempty" yaml:"dhcp6,omitempty"`
	Addresses   []string           `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	Gateway4    string             `json:"gateway4,omitempty" yaml:"gateway4,omitempty"`
	Gateway6    string             `json:"gateway6,omitempty" yaml:"gateway6,omitempty"`
	Nameservers *NameserversConfig `json:"nameservers,omitempty" yaml:"nameservers,omitempty"`
	Routes      []RouteConfig      `json:"routes,omitempty" yaml:"routes,omitempty"`
}

// VlanConfig represents a VLAN device configuration.
type VlanConfig struct {
	ID          int                `json:"id" yaml:"id"`
	Link        string             `json:"link" yaml:"link"`
	Dhcp4       bool               `json:"dhcp4,omitempty" yaml:"dhcp4,omitempty"`
	Dhcp6       bool               `json:"dhcp6,omitempty" yaml:"dhcp6,omitempty"`
	Addresses   []string           `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	Gateway4    string             `json:"gateway4,omitempty" yaml:"gateway4,omitempty"`
	Gateway6    string             `json:"gateway6,omitempty" yaml:"gateway6,omitempty"`
	Nameservers *NameserversConfig `json:"nameservers,omitempty" yaml:"nameservers,omitempty"`
	Routes      []RouteConfig      `json:"routes,omitempty" yaml:"routes,omitempty"`
}

// MatchConfig specifies how to match a device.
type MatchConfig struct {
	MACAddress string `json:"macaddress,omitempty" yaml:"macaddress,omitempty"`
	Driver     string `json:"driver,omitempty" yaml:"driver,omitempty"`
	Name       string `json:"name,omitempty" yaml:"name,omitempty"`
}

// BondParameters contains bonding-specific parameters.
type BondParameters struct {
	Mode                  string `json:"mode,omitempty" yaml:"mode,omitempty"`
	MIIMonitorInterval    int    `json:"mii-monitor-interval,omitempty" yaml:"mii-monitor-interval,omitempty"`
	LACPRate              string `json:"lacp-rate,omitempty" yaml:"lacp-rate,omitempty"`
	TransmitHashPolicy    string `json:"transmit-hash-policy,omitempty" yaml:"transmit-hash-policy,omitempty"`
	ADSelect              string `json:"ad-select,omitempty" yaml:"ad-select,omitempty"`
	PrimaryReselectPolicy string `json:"primary-reselect-policy,omitempty" yaml:"primary-reselect-policy,omitempty"`
	FailOverMACPolicy     string `json:"fail-over-mac-policy,omitempty" yaml:"fail-over-mac-policy,omitempty"`
	Primary               string `json:"primary,omitempty" yaml:"primary,omitempty"`
	GratuitousARP         int    `json:"gratuitious-arp,omitempty" yaml:"gratuitious-arp,omitempty"`
	PacketsPerSlave       int    `json:"packets-per-slave,omitempty" yaml:"packets-per-slave,omitempty"`
}

// BridgeParameters contains bridge-specific parameters.
type BridgeParameters struct {
	AgeingTime   int  `json:"ageing-time,omitempty" yaml:"ageing-time,omitempty"`
	ForwardDelay int  `json:"forward-delay,omitempty" yaml:"forward-delay,omitempty"`
	HelloTime    int  `json:"hello-time,omitempty" yaml:"hello-time,omitempty"`
	MaxAge       int  `json:"max-age,omitempty" yaml:"max-age,omitempty"`
	Priority     int  `json:"priority,omitempty" yaml:"priority,omitempty"`
	STP          bool `json:"stp,omitempty" yaml:"stp,omitempty"`
}

// NameserversConfig specifies DNS nameservers and search domains.
type NameserversConfig struct {
	Addresses []string `json:"addresses,omitempty" yaml:"addresses,omitempty"`
	Search    []string `json:"search,omitempty" yaml:"search,omitempty"`
}

// RouteConfig represents a routing table entry.
type RouteConfig struct {
	To     string `json:"to,omitempty" yaml:"to,omitempty"`
	Via    string `json:"via,omitempty" yaml:"via,omitempty"`
	Metric int    `json:"metric,omitempty" yaml:"metric,omitempty"`
	Table  int    `json:"table,omitempty" yaml:"table,omitempty"`
}

// Instance is a representation of the instance metadata. Its based on the rooitio hub action
// and should have just enough information for it to work.
type HackInstance struct {
	Metadata struct {
		Instance struct {
			Storage struct {
				Disks []struct {
					Device     string `json:"device"`
					Partitions []struct {
						Label  string `json:"label"`
						Number int    `json:"number"`
						Size   uint64 `json:"size"`
					} `json:"partitions"`
					WipeTable bool `json:"wipe_table"`
				} `json:"disks"`
				Filesystems []struct {
					Mount struct {
						Create struct {
							Options []string `json:"options"`
						} `json:"create"`
						Device string `json:"device"`
						Format string `json:"format"`
						Point  string `json:"point"`
					} `json:"mount"`
				} `json:"filesystems"`
			} `json:"storage"`
		} `json:"instance"`
	} `json:"metadata"`
}
