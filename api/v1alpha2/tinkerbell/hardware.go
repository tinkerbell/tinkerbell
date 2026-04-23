package tinkerbell

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hardware,scope=Namespaced,categories=tinkerbell,singular=hardware,shortName=hw
// +kubebuilder:storageversion
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io=
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io/move=
// +kubebuilder:printcolumn:JSONPath=".metadata.annotations['tinkerbell.org/disabled']",name="Disabled",type=string,priority=1

// Hardware is the Schema for the Hardware API.
type Hardware struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec HardwareSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// HardwareList contains a list of Hardware.
type HardwareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Hardware `json:"items"`
}

// HardwareSpec defines the desired state of Hardware.
type HardwareSpec struct {
	// AgentID is the unique identifier an Agent uses that is associated with this Hardware.
	// This is used to identify Hardware during the discovery and enrollment process.
	// It is typically the MAC address of the primary network interface.
	AgentID string `json:"agentID,omitempty"`

	// Attributes related to Hardware. These are attributes that are needed/used to make logical decisions
	// around builtin capabilities. For example, the Arch determines which OSIE files to server (x86_64 or aarch64).
	// The UEFI boolean us needed/used to determine options in IPMI calls.
	Attributes *Attributes `json:"attributes,omitempty"`

	// Auto is the configuration for the automatic capabilities.
	Auto AutoCapabilities `json:"auto,omitempty"`

	// Instance describes data that is less permanent than any physical attributes of the Hardware.
	// +optional
	Instance *Instance `json:"instance,omitempty"`

	// NetworkInterfaces defines the desired DHCP and netboot configuration for a network interface.
	// +optional
	NetworkInterfaces NetworkInterfaces `json:"networkInterfaces,omitempty"`

	// References allow for linking custom resource objects of any kind to this Hardware object.
	// These are available in Workflows for templating. They are referenced by the name of the reference.
	// For example, given a reference with the name "lvm", you can access it in a Workflow with {{ .references.lvm }}.
	// +optional
	References *References `json:"references,omitempty"`

	// StorageDevices is a list of storage devices that will be available in the OSIE.
	// +optional
	StorageDevices []StorageDevice `json:"storageDevices,omitempty"`
}

// Attributes related to Hardware. These are attributes that are needed/used to make logical decisions
// around builtin capabilities. For example, the Arch determines which OSIE files to server (x86_64 or aarch64).
// The UEFI boolean us needed/used to determine options in IPMI calls.
type Attributes struct {
	// Arch represents the Hardware's architecture type.
	// For example; x86_64 or aarch64
	Arch string `json:"arch,omitempty"`

	// UEFI reports whether the Hardware uses UEFI.
	UEFI bool `json:"uefi,omitempty"`
}

// AutoCapabilities defines the configuration for the automatic capabilities of this Hardware.
type AutoCapabilities struct {
	// EnrollmentEnabled enables automatic enrollment of the Hardware.
	// When set to true, auto enrollment will create Workflows for this Hardware.
	// +kubebuilder:default=false
	EnrollmentEnabled bool `json:"enrollmentEnabled,omitempty"`
}

// Instance describes data that is less permanent than any physical attributes of the Hardware.
type Instance struct {
	// Metadata is data following the cloud-init NoCloud datasource for meta-data.
	//
	// https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html#meta-data
	//
	// This string can be templated with values from the Hardware object by using the
	// standard Go templating format, https://pkg.go.dev/text/template.
	// Always start with ".hardware", then use dot notation to the desired key.
	//
	// {{ .hardware.location.to.a.key.in.the.spec }}
	//
	// For example:
	// Reference the first ssh key in the list:
	// {{ .hardware.instance.sshKeys[0] }}
	//
	// or
	//
	// Reference the IPv4 address of a network interface:
	// {{ (index .hardware.networkInterfaces "52:54:00:41:05:c6").ipam.ipv4.address }}
	// +optional
	Metadata *string `json:"metadata,omitempty"`

	// NetworkConfig is config following the cloud-init NoCloud datasource for network configuration.
	//
	// https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html#network-config
	//
	// This string can be templated with values from the Hardware object by using the
	// standard Go templating format, https://pkg.go.dev/text/template.
	// Always start with ".hardware", then use dot notation to the desired key.
	//
	// {{ .hardware.location.to.a.key.in.the.spec }}
	//
	// For example:
	// Reference the first ssh key in the list:
	// {{ .hardware.instance.sshKeys[0] }}
	//
	// or
	//
	// Reference the IPv4 address of a network interface:
	// {{ (index .hardware.networkInterfaces "52:54:00:41:05:c6").ipam.ipv4.address }}
	// +optional
	NetworkConfig *string `json:"networkConfig,omitempty"`

	// OSIE (Operating System Installation Environment) configuration.
	// +optional
	OSIE *OSIE `json:"osie,omitempty"`

	// SSHKeys are public SSH keys associated with this Hardware.
	SSHKeys []string `json:"sshKeys,omitempty"`

	// Userdata is data following the cloud-init NoCloud datasource for user-data.
	//
	// https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html#user-data
	//
	// This string can be templated with values from the Hardware object by using the
	// standard Go templating format, https://pkg.go.dev/text/template.
	// Always start with ".hardware", then use dot notation to the desired key.
	//
	// {{ .hardware.location.to.a.key.in.the.spec }}
	//
	// For example:
	// Reference the first ssh key in the list:
	// {{ .hardware.instance.sshKeys[0] }}
	//
	// or
	//
	// Reference the IPv4 address of a network interface:
	// {{ (index .hardware.networkInterfaces "52:54:00:41:05:c6").ipam.ipv4.address }}
	// +optional
	Userdata *string `json:"userdata,omitempty"`

	// Vendordata is data following the cloud-init NoCloud datasource for vendor-data.
	//
	// https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html#vendor-data
	//
	// This string can be templated with values from the Hardware object by using the
	// standard Go templating format, https://pkg.go.dev/text/template.
	// Always start with ".hardware", then use dot notation to the desired key.
	//
	// {{ .hardware.location.to.a.key.in.the.spec }}
	//
	// For example:
	// Reference the first ssh key in the list:
	// {{ .hardware.instance.sshKeys[0] }}
	//
	// or
	//
	// Reference the IPv4 address of a network interface:
	// {{ (index .hardware.networkInterfaces "52:54:00:41:05:c6").ipam.ipv4.address }}
	// +optional
	Vendordata *string `json:"vendordata,omitempty"`
}

// NetworkInterfaces maps a MAC address to a NetworkInterface.
type NetworkInterfaces map[MAC]NetworkInterface

// MAC is a Media Access Control address. MACs must use lower case letters.
// +kubebuilder:validation:Pattern=`^([0-9a-f]{2}:){5}([0-9a-f]{2})$`
type MAC string

// NetworkInterface is the desired configuration for a particular network interface.
type NetworkInterface struct {
	// DHCP is the DHCP configuration for this interface.
	// +optional
	DHCP *DHCP `json:"dhcp,omitempty"`

	// IPAM is the IP address management for this interface.
	// +optional
	IPAM *IPAM `json:"ipam,omitempty"`

	// Isoboot configuration.
	// +optional
	Isoboot *Isoboot `json:"isoboot,omitempty"`

	// Netboot configuration.
	// +optional
	Netboot *Netboot `json:"netboot,omitempty"`
}

// DHCP is the DHCP configuration for a network interface.
type DHCP struct {
	// V4 is the options for serving DHCPv4 requests.
	// +optional
	V4 *DHCPv4 `json:"v4,omitempty"`

	// V6 is the options for serving DHCPv6 requests.
	// +optional
	V6 *DHCPv6 `json:"v6,omitempty"`
}

// DHCPv4 describes basic network configuration to be served in DHCPv4 responses.
// +kubebuilder:validation:XValidation:rule=(has(self.tftpServerName) && self.tftpServerName != "") == (has(self.bootFileName) && self.bootFileName != ""),message="TFTPServerName and BootFileName must both be specified or both be empty"
type DHCPv4 struct {
	// Disabled indicates that DHCPv4 should not be served for this interface.
	// When true, no DHCPv4 offer or ack will be sent for this MAC address.
	// +optional
	Disabled bool `json:"disabled,omitempty"`

	// BootFileName is the boot file name. DHCP option 67.
	// Used for explicit boot file configuration, required by some network boot clients
	// like NVIDIA NVOS switches.
	// If specified, TFTPServerName must also be specified.
	// +optional
	BootFileName string `json:"bootFileName,omitempty"`

	// ClasslessStaticRoutes defines static routes to be sent via DHCP option 121 (RFC 3442).
	// +optional
	ClasslessStaticRoutes []ClasslessStaticRoute `json:"classlessStaticRoutes,omitempty"`

	// DomainName to be written. DHCP option 15.
	// +optional
	DomainName string `json:"domainName,omitempty"`

	// DomainSearchList defines DNS search suffixes via DHCP option 119 (RFC 3397).
	// +optional
	// +kubebuilder:validation:MaxItems=32
	DomainSearchList []string `json:"domainSearchList,omitempty"`

	// Hostname is DHCP option 12.
	// +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\\-]*[a-zA-Z0-9])$`
	// +optional
	Hostname *string `json:"hostname,omitempty"`

	// LeaseTimeSeconds to serve. 24h default. Maximum equates to max uint32 as defined by RFC 2132
	// § 9.2 (https://www.rfc-editor.org/rfc/rfc2132.html#section-9.2). DHCP option 51.
	// +kubebuilder:default=86400
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	// +optional
	LeaseTimeSeconds *int64 `json:"leaseTimeSeconds,omitempty"`

	// Nameservers corresponding to DHCPv4 option 6.
	// +optional
	// +kubebuilder:validation:MaxItems=10
	Nameservers []Nameserver `json:"nameservers,omitempty"`

	// TFTPServerName is the TFTP server name or IP address. DHCP option 66.
	// Used for explicit TFTP server configuration, required by some network boot clients
	// like NVIDIA NVOS switches.
	// If specified, BootFileName must also be specified.
	// +optional
	TFTPServerName string `json:"tftpServerName,omitempty"`

	// NTPServers corresponding to DHCPv4 option 42 (RFC 2132 §8.3).
	// +optional
	// +kubebuilder:validation:MaxItems=10
	NTPServers []Timeserver `json:"ntpServers,omitempty"`

	// VLANID is a VLAN ID between 0 and 4096. DHCP option 43 suboption 116.
	// +kubebuilder:validation:Pattern=`^(([0-9][0-9]{0,2}|[1-3][0-9][0-9][0-9]|40([0-8][0-9]|9[0-6]))(,[1-9][0-9]{0,2}|[1-3][0-9][0-9][0-9]|40([0-8][0-9]|9[0-6]))*)$`
	// +optional
	VLANID *string `json:"vlanID,omitempty"`
}

// ClasslessStaticRoute represents a classless static route for DHCP option 121 (RFC 3442).
type ClasslessStaticRoute struct {
	// DestinationDescriptor is the network address and prefix length.
	// The format is "network/prefixlength", e.g., "192.168.1.0/24" or "10.0.0.0/8".
	// +kubebuilder:validation:Pattern=`^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)/(3[0-2]|[12]?[0-9])$`
	DestinationDescriptor string `json:"destinationDescriptor"`
	// Router is the IP address of the router for this route.
	// +kubebuilder:validation:Pattern=`^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`
	Router string `json:"router"`
}

// Nameserver is an IPv4 address, IPv6 address, or hostname.
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern=`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$|^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$|^(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:))$`
type Nameserver string

// Timeserver is an IPv4 address, IPv6 address, or hostname.
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern=`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$|^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$|^(([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:))$`
type Timeserver string

// DHCPv6 describes network configuration to be served in DHCPv6 responses.
// The DHCPv6 protocol is client-driven: the client sends either an Information-Request
// (stateless, configuration only) or a Solicit (stateful, address assignment + configuration).
// The server responds to both message types using the fields configured here.
type DHCPv6 struct {
	// Disabled indicates that DHCPv6 should not be served for this interface.
	// When true, no DHCPv6 responses will be sent for this MAC address.
	// +optional
	Disabled bool `json:"disabled,omitempty"`

	// Nameservers to serve via DHCPv6 option 23 (RFC 3646). Must be valid IPv6 addresses.
	// +optional
	// +kubebuilder:validation:MaxItems=10
	Nameservers []Nameserver `json:"nameservers,omitempty"`

	// DomainSearchList to serve via DHCPv6 option 24 (RFC 3646).
	// +optional
	// +kubebuilder:validation:MaxItems=32
	DomainSearchList []string `json:"domainSearchList,omitempty"`

	// NTPServers to serve via DHCPv6 option 56 (RFC 5908). Must be valid IPv6 addresses.
	// +optional
	// +kubebuilder:validation:MaxItems=10
	NTPServers []Timeserver `json:"ntpServers,omitempty"`

	// InformationRefreshTime is the upper bound in seconds for how long a client should wait
	// before refreshing information retrieved from DHCPv6. DHCPv6 option 32 (RFC 8415 §21.23).
	// Included in replies to Information-Request messages.
	// +optional
	// +kubebuilder:validation:Minimum=0
	InformationRefreshTime *int64 `json:"informationRefreshTime,omitempty"`

	// PreferredLifetime is the preferred lifetime in seconds for IPv6 addresses
	// assigned via IA_NA. Included in replies to Solicit/Request messages.
	// +optional
	// +kubebuilder:validation:Minimum=0
	PreferredLifetime *int64 `json:"preferredLifetime,omitempty"`

	// ValidLifetime is the valid lifetime in seconds for IPv6 addresses
	// assigned via IA_NA. Included in replies to Solicit/Request messages.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ValidLifetime *int64 `json:"validLifetime,omitempty"`

	// BootFileURL is the URL for the boot file. DHCPv6 option 59 (RFC 5970).
	// For example, "tftp://[2001:db8::1]/bootx64.efi".
	// +optional
	BootFileURL string `json:"bootFileURL,omitempty"`

	// BootFileParams are parameters for the boot file. DHCPv6 option 60 (RFC 5970).
	// +optional
	BootFileParams []string `json:"bootFileParams,omitempty"`
}

// IPAM IP address management info.
type IPAM struct {
	// IPv4 is the IPv4 address and associated network data.
	IPv4 *IP `json:"ipv4,omitempty"`

	// IPv6 is the IPv6 address and associated network data.
	IPv6 *IP `json:"ipv6,omitempty"`
}

// IP configuration.
type IP struct {
	// Address is the unique network address.
	// +kubebuilder:validation:MaxLength=45
	Address string `json:"address,omitempty"`

	// Gateway is the default gateway address to serve.
	// +kubebuilder:validation:MaxLength=45
	Gateway string `json:"gateway,omitempty"`

	// Prefix is the subnet length.
	// +kubebuilder:validation:MaxLength=3
	Prefix string `json:"prefix,omitempty"`
}

// Isoboot configuration for booting a client using an ISO image.
type Isoboot struct {
	// SourceISO is the source url where HookOS, an Operating System Installation Environment (OSIE), ISO lives.
	// It must be a valid url.URL{} object and must have a url.URL{}.Scheme of HTTP or HTTPS.
	// +optional
	// +kubebuilder:validation:Format=uri
	SourceISO string `json:"sourceISO,omitempty"`
}

// Netboot configuration.
type Netboot struct {
	// Disabled indicates that netbooting should not be enabled for this interface.
	// When true, no netboot options will be provided in DHCP and iPXE script requests will return 404.
	// +optional
	Disabled bool `json:"disabled,omitempty"`

	// IPXE configuration.
	// +optional
	IPXE *IPXE `json:"ipxe,omitempty"`
}

// IPXE configuration.
type IPXE struct {
	// Binary, when defined, overrides Smee's default mapping of architecture to iPXE binary.
	// The following binary names are supported:
	// - undionly.kpxe
	// - ipxe.efi
	// - snp-arm64.efi
	// - snp-x86_64.efi
	// See the iPXE Architecture Mapping documentation for more details.
	// +optional
	Binary string `json:"binary,omitempty"`

	// Script, when defined, overrides the Tinkerbell iPXE script.
	// Must start with: #!ipxe
	// +optional
	Script string `json:"script,omitempty"`

	// URL, when defined, overrides the Tinkerbell iPXE script and uses iPXE's chainloading capabilities
	// to download and run the script at the defined URL.
	// +optional
	URL string `json:"url,omitempty"`
}

// OSIE (Operating System Installation Environment) configuration.
type OSIE struct {
	// InitrdURL is a URL to an initrd image.
	// +optional
	InitrdURL string `json:"initrdURL,omitempty"`

	// KernelParams passed to the kernel command line when launching the OSIE.
	// Typically they will be in the format "key=value" and align with the Linux Kernel
	// and OSIE documentation, but they can be any string.
	// +optional
	KernelParams []string `json:"kernelParams,omitempty"`

	// KernelURL is a URL to a kernel image.
	// +optional
	KernelURL string `json:"kernelURL,omitempty"`
}

// References represents builtin and additional reference maps.
type References struct {
	// Additional references are dynamic and defined by the user.
	// +optional
	Additional map[string]Reference `json:"additional,omitempty"`

	// Builtin references are predefined.
	// +optional
	Builtin BuiltinReferences `json:"builtin,omitempty"`
}

type Reference struct {
	// Group of the referent.
	// More info: https://kubernetes.io/docs/reference/using-api/#api-groups
	Group string `json:"group,omitempty"`

	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name,omitempty"`

	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	Namespace string `json:"namespace,omitempty"`

	// Resource of the referent. Must be the pluralized kind of the referent. Must be all lowercase.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Resource string `json:"resource,omitempty"`

	// API version of the referent.
	// More info: https://kubernetes.io/docs/reference/using-api/#api-versioning
	Version string `json:"version,omitempty"`
}

type BuiltinReferences struct {
	// BMC is the reference to a machine.bmc.tinkerbell.org object.
	// +optional
	BMC SimpleReference `json:"bmc,omitempty"`
}

// SimpleReference
// +kubebuilder:validation:XValidation:rule="(has(self.name) && self.name != \"\") == (has(self.namespace) && self.namespace != \"\")",message="name and namespace must both be specified or both be empty"
type SimpleReference struct {
	// Name of the object.
	Name string `json:"name,omitempty"`

	// Namespace where the object resides.
	Namespace string `json:"namespace,omitempty"`
}

// / StorageDevice describes a storage device that is be present on the Hardware.
type StorageDevice struct {
	// Name must be a valid Linux path. It should not contain partitions.
	//
	// Good
	//
	//	/dev/sda
	//	/dev/nvme0n1
	//
	// Bad (contains partitions)
	//
	//	/dev/sda1
	//	/dev/nvme0n1p1
	//
	// Bad (invalid Linux path)
	//
	//	\dev\sda
	Name string `json:"name,omitempty"`
}
