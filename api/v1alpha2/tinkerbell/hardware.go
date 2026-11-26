package tinkerbell

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=hardware,scope=Namespaced,categories=tinkerbell,singular=hardware,shortName=hw
// +kubebuilder:storageversion
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io=
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io/move=
// +kubebuilder:printcolumn:JSONPath=".metadata.annotations['tinkerbell.org/disabled']",name="Disabled",type=string,priority=1

// Hardware is the Schema for the Hardware API.
type Hardware struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HardwareSpec   `json:"spec,omitempty"`
	Status HardwareStatus `json:"status,omitempty"`
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
	// +optional
	Agent *Agent `json:"agent,omitempty"`

	// Arch represents the Hardware's architecture type.
	// For example: x86_64, amd64, aarch64, arm64, etc.
	// +optional
	Arch string `json:"arch,omitempty"`

	// Auto is the configuration for the automatic capabilities.
	// +optional
	Auto *AutoCapabilities `json:"auto,omitempty"`

	// BMC contains connection and configuration data for the BMC (Baseboard Management Controller) of this Hardware.
	// +optional
	BMC *BMC `json:"bmc,omitempty"`

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
	References map[string]Reference `json:"references,omitempty"`

	// StorageDevices is a list of storage devices that will be available in the OSIE.
	// +optional
	// +listType=map
	// +listMapKey=name
	StorageDevices []StorageDevice `json:"storageDevices,omitempty"`
}

type Agent struct {
	// ID is the unique identifier an Agent uses that is associated with this Hardware.
	// This is used to identify Hardware during the discovery and enrollment process.
	// It is typically the MAC address of the primary network interface.
	// +optional
	ID string `json:"id,omitempty"`

	// Authentication
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
}

type Authentication struct {
	// +optional
	TPM *TPM `json:"tpm,omitempty"`
	// +optional
	Wireguard *Wireguard `json:"wireguard,omitempty"`
}

type TPM struct {
	// +optional
	EKPublicKeyHash string `json:"ekPublicKeyHash,omitempty"`
}

type Wireguard struct {
	// +optional
	PublicKey string `json:"publicKey,omitempty"`
}

// AutoCapabilities defines the configuration for the automatic capabilities of this Hardware.
type AutoCapabilities struct {
	// EnrollmentEnabled enables automatic enrollment of the Hardware.
	// When set to true, auto enrollment will create Workflows for this Hardware.
	// Unset is equivalent to false.
	// +optional
	EnrollmentEnabled *bool `json:"enrollmentEnabled,omitempty"`
}

// Instance describes data that is less permanent than any physical attributes of the Hardware.
type Instance struct {
	// LookupID is an identifier used by the metadata service (Tootles) to route incoming requests
	// to this Hardware object. This is the value that must appear in the NoCloud seedfrom URL path
	// or the EC2 metadata endpoint path for Tootles to match the request to this Hardware.
	//
	// This field is required when using bring-your-own DHCP (BYO DHCP), where IP-based lookup is
	// not available. The value must match whatever identifier will appear in the request URL.
	// For example, if cloud-init is configured with:
	//
	//   ds=nocloud;s=http://tootles:8080/nocloud/<lookupID>/
	//
	// then this field must be set to the same <lookupID> value. When using DMI variable expansion
	// (e.g., __dmi.chassis-serial-number__), this field must match the machine's actual DMI value.
	//
	// This field does not support templating. The value is stored and indexed as-is.
	// +optional
	LookupID string `json:"lookupID,omitempty"`

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
	// +optional
	// +listType=set
	SSHKeys []string `json:"sshKeys,omitempty"`

	// UserData is data following the cloud-init NoCloud datasource for user-data.
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
	UserData *string `json:"userData,omitempty"`

	// Vars are custom variables that can be filled with arbitrary key-value pairs.
	// +optional
	Vars map[string]string `json:"vars,omitempty"`

	// VendorData is data following the cloud-init NoCloud datasource for vendor-data.
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
	VendorData *string `json:"vendorData,omitempty"`

	// WorkflowsDisabled, when true, prevents Workflows that reference this Hardware from running.
	// Unset is equivalent to false.
	// +optional
	WorkflowsDisabled *bool `json:"workflowsDisabled,omitempty"`
}

// OSIE (Operating System Installation Environment) configuration.
type OSIE struct {
	// InitrdURL, when defined, overrides the default initrd URL.
	// +optional
	InitrdURL string `json:"initrdURL,omitempty"`

	// ISOURL, when defined, overrides is the default source URL where the Operating System Installation Environment (OSIE) ISO lives.
	// It must be a valid URL and must have a Scheme of HTTP or HTTPS.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == '' || (isURL(self) && url(self).getScheme() in ['http','https'])",message="isoURL must be a valid http or https URL"
	ISOURL string `json:"isoURL,omitempty"`

	// KernelParams, when defined, overrides the default kernel parameters passed to the kernel command line when launching the OSIE.
	// Typically they will be in the format "key=value" and align with the Linux Kernel
	// and OSIE documentation, but they can be any string.
	// +optional
	// +listType=atomic
	KernelParams []string `json:"kernelParams,omitempty"`

	// KernelURL, when defined, overrides the default URL to a kernel image.
	// +optional
	KernelURL string `json:"kernelURL,omitempty"`

	// Files, when defined, is the set of additional files to be made available to the OSIE (Operating System Installation Environment).
	// +kubebuilder:validation:MaxItems=64
	// +kubebuilder:validation:XValidation:rule="self.all(a, self.exists_one(b, (b.path.startsWith('/') ? b.path.substring(1) : b.path) == (a.path.startsWith('/') ? a.path.substring(1) : a.path)))",message="osieFiles paths must be unique"
	// +kubebuilder:validation:XValidation:rule="self.all(a, !self.exists(b, (b.path.startsWith('/') ? b.path.substring(1) : b.path).startsWith((a.path.startsWith('/') ? a.path.substring(1) : a.path) + '/')))",message="an osieFiles path must not be the parent directory of another"
	// +optional
	// +listType=atomic
	Files []OSIEFile `json:"files,omitempty"`
}

// OSIEFile represents a file to be made available to the OSIE (Operating System Installation Environment).
// All files are relative to the default /osie_files directory in the OSIE (Operating System Installation Environment).
type OSIEFile struct {
	// Path is the file path where the file should be made available within the default /osie_files directory in the OSIE (Operating System Installation Environment).
	// A leading "/" is ignored. Empty, "." and ".." segments are not allowed.
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:Pattern=`^/?[^/]+(/[^/]+)*$`
	// +kubebuilder:validation:XValidation:rule="!self.split('/').exists(s, s == '.' || s == '..')",message="path must not contain '.' or '..' segments"
	// +required
	Path string `json:"path"`

	// Contents is the contents of the file.
	// +optional
	Contents string `json:"contents,omitempty"`

	// Mode is the file's permission bits as an octal string, e.g. "0644", "644" or "0o644".
	// Only permission bits are allowed (max "0777"); setuid, setgid and sticky bits are rejected.
	// Symbolic forms like "rw-r--r--" and decimal values are invalid.
	// Omitted or "0" means the default, "0400" (read-only for the owner).
	// +kubebuilder:validation:MaxLength=8
	// +kubebuilder:validation:Pattern=`^(0[oO])?0*[0-7]{1,3}$`
	// +optional
	Mode string `json:"mode,omitempty"`
}

// NetworkInterfaces maps a MAC address to a NetworkInterface.
// A Hardware can have at most 64 network interfaces.
// +kubebuilder:validation:MaxProperties=64
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

	// Netboot configuration.
	// +optional
	Netboot *Netboot `json:"netboot,omitempty"`
}

// DHCP is the DHCP configuration for a network interface.
type DHCP struct {
	// IPv4 is the options for serving dhcpv4 requests.
	// +optional
	IPv4 *DHCPv4 `json:"ipv4,omitempty"`

	// IPv6 is the options for serving dhcpv6 requests.
	// +optional
	IPv6 *DHCPv6 `json:"ipv6,omitempty"`
}

// DHCPv4 describes basic network configuration to be served in DHCPv4 responses.
type DHCPv4 struct {
	// Disabled indicates that DHCPv4 should not be served for this interface.
	// When true, no DHCPv4 offer or ack will be sent for this MAC address.
	// Unset is equivalent to false.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// LeaseTimeSeconds is the lease time to serve. DHCP option 51.
	// The maximum is the largest uint32, as defined by RFC 2132 § 9.2
	// (https://www.rfc-editor.org/rfc/rfc2132.html#section-9.2).
	// +kubebuilder:default=604800
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	// +optional
	LeaseTimeSeconds *int64 `json:"leaseTimeSeconds,omitempty"`
}

// DHCPv6 describes network configuration to be served in DHCPv6 responses.
// The DHCPv6 protocol is client-driven: the client sends either an Information-Request
// (stateless, configuration only) or a Solicit (stateful, address assignment + configuration).
// The server responds to both message types using the fields configured here.
// +kubebuilder:validation:XValidation:rule="!has(self.preferredLifetimeSeconds) || !has(self.validLifetimeSeconds) || self.preferredLifetimeSeconds <= self.validLifetimeSeconds",message="preferredLifetimeSeconds must not exceed validLifetimeSeconds"
type DHCPv6 struct {
	// Disabled indicates that DHCPv6 should not be served for this interface.
	// When true, no DHCPv6 responses will be sent for this MAC address.
	// Unset is equivalent to false.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// InformationRefreshTimeSeconds is the upper bound in seconds for how long a client should wait
	// before refreshing information retrieved from DHCPv6. DHCPv6 option 32 (RFC 9915 §21.23).
	// Included in replies to Information-Request messages. RFC 9915 sets the minimum at 600 seconds.
	// +optional
	// +kubebuilder:validation:Minimum=600
	// +kubebuilder:validation:Maximum=4294967295
	InformationRefreshTimeSeconds *int64 `json:"informationRefreshTimeSeconds,omitempty"`

	// PreferredLifetimeSeconds is the preferred lifetime in seconds for IPv6 addresses
	// assigned via IA_NA. Included in replies to Solicit/Request messages.
	// It must not exceed the valid lifetime. 4294967295 basically means infinity.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	PreferredLifetimeSeconds *int64 `json:"preferredLifetimeSeconds,omitempty"`

	// ValidLifetimeSeconds is the valid lifetime in seconds for IPv6 addresses
	// assigned via IA_NA. Included in replies to Solicit/Request messages.
	// 4294967295 basically means infinity.
	// +optional
	// +kubebuilder:default=604800
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4294967295
	ValidLifetimeSeconds *int64 `json:"validLifetimeSeconds,omitempty"`
}

// IPAM IP address management info.
// +kubebuilder:validation:XValidation:rule="!has(self.ipv4) || !has(self.ipv4.classlessStaticRoutes) || self.ipv4.classlessStaticRoutes.all(r, isCIDR(r.destinationDescriptor) && cidr(r.destinationDescriptor).ip().family() == 4 && isIP(r.router) && ip(r.router).family() == 4)",message="ipv4.classlessStaticRoutes entries must use IPv4 addresses for both destinationDescriptor and router"
// +kubebuilder:validation:XValidation:rule="!has(self.ipv6) || !has(self.ipv6.classlessStaticRoutes) || self.ipv6.classlessStaticRoutes.all(r, isCIDR(r.destinationDescriptor) && cidr(r.destinationDescriptor).ip().family() == 6 && isIP(r.router) && ip(r.router).family() == 6)",message="ipv6.classlessStaticRoutes entries must use IPv6 addresses for both destinationDescriptor and router"
// +kubebuilder:validation:XValidation:rule="!has(self.ipv4) || ((!has(self.ipv4.address) || (isIP(self.ipv4.address) && ip(self.ipv4.address).family() == 4)) && (!has(self.ipv4.gateway) || (isIP(self.ipv4.gateway) && ip(self.ipv4.gateway).family() == 4)))",message="ipv4.address and ipv4.gateway must be IPv4 addresses"
// +kubebuilder:validation:XValidation:rule="!has(self.ipv6) || ((!has(self.ipv6.address) || (isIP(self.ipv6.address) && ip(self.ipv6.address).family() == 6)) && (!has(self.ipv6.gateway) || (isIP(self.ipv6.gateway) && ip(self.ipv6.gateway).family() == 6)))",message="ipv6.address and ipv6.gateway must be IPv6 addresses"
// +kubebuilder:validation:XValidation:rule="!has(self.ipv4) || ((!has(self.ipv4.nameservers) || self.ipv4.nameservers.all(s, isIP(s) && ip(s).family() == 4)) && (!has(self.ipv4.ntpServers) || self.ipv4.ntpServers.all(s, isIP(s) && ip(s).family() == 4)))",message="ipv4.nameservers and ipv4.ntpServers must be IPv4 addresses"
// +kubebuilder:validation:XValidation:rule="!has(self.ipv6) || ((!has(self.ipv6.nameservers) || self.ipv6.nameservers.all(s, isIP(s) && ip(s).family() == 6)) && (!has(self.ipv6.ntpServers) || self.ipv6.ntpServers.all(s, isIP(s) && ip(s).family() == 6)))",message="ipv6.nameservers and ipv6.ntpServers must be IPv6 addresses"
// +kubebuilder:validation:XValidation:rule="!has(self.ipv4) || !has(self.ipv4.prefix) || self.ipv4.prefix <= 32",message="ipv4.prefix must be at most 32"
// +kubebuilder:validation:XValidation:rule="!has(self.ipv4) || !has(self.ipv4.address) || has(self.ipv4.prefix)",message="ipv4.prefix is required when ipv4.address is set"
type IPAM struct {
	// Hostname is served as DHCPv4 option 12 and DHCPv6 option 39. Must be a valid hostname or
	// fully qualified domain name as defined by RFC 1123.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`
	// +optional
	Hostname *string `json:"hostname,omitempty"`

	// IPv4 is the IPv4 address and associated network info.
	// +optional
	IPv4 *IP `json:"ipv4,omitempty"`

	// IPv6 is the IPv6 address and associated network info.
	// +optional
	IPv6 *IP `json:"ipv6,omitempty"`

	// VLANID is the VLAN ID, between 0 and 4096, that this interface is on. DHCPv4 option 43 suboption 116.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=4096
	// +optional
	VLANID *int32 `json:"vlanID,omitempty"`
}

// IP configuration. All addresses must be of the family of the enclosing IPAM field (ipv4 or ipv6).
type IP struct {
	// Address is the unique network address.
	// +kubebuilder:validation:MaxLength=45
	// +optional
	Address string `json:"address,omitempty"`

	// ClasslessStaticRoutes defines static routes to be sent via DHCP option 121 (RFC 3442) or for DHCPv6 these will be available for statically being configured.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=32
	ClasslessStaticRoutes []ClasslessStaticRoute `json:"classlessStaticRoutes,omitempty"`

	// DomainSearchList to serve via DHCPv6 option 24 (RFC 3646) and DHCPv4 option 15 (or 119) (RFC 3397). They must be listed in order of priority.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=32
	DomainSearchList []string `json:"domainSearchList,omitempty"`

	// Gateway is the default gateway address. It is served as DHCPv4 option 3. DHCPv6 cannot serve a
	// default gateway, as that comes from Router Advertisements, so for IPv6 it is only available for static configuration.
	// +kubebuilder:validation:MaxLength=45
	// +optional
	Gateway string `json:"gateway,omitempty"`

	// Nameservers to serve as DHCPv4 option 6 (RFC 2132) or DHCPv6 option 23 (RFC 3646).
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=10
	Nameservers []Nameserver `json:"nameservers,omitempty"`

	// NTPServers to serve as DHCPv4 option 42 (RFC 2132 §8.3) or DHCPv6 option 56 (RFC 5908).
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=10
	NTPServers []Timeserver `json:"ntpServers,omitempty"`

	// Prefix is the subnet prefix length, for example 24 for an IPv4 /24 or 64 for an IPv6 /64.
	// It is required for an IPv4 address and must be at most 32 for IPv4. DHCPv6 cannot serve a
	// prefix length, as that comes from Router Advertisements, so for IPv6 it is only available for static configuration.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=128
	// +optional
	Prefix int32 `json:"prefix,omitempty"`
}

// ClasslessStaticRoute represents a classless static route for DHCP option 121 (RFC 3442) (IPv4)
// or a static route exposed alongside DHCPv6 configuration for client-side configuration (IPv6).
// The IP family of DestinationDescriptor and Router must match the parent IPAM field
// (ipv4 or ipv6) they are nested under. This is enforced by CEL validation on the IPAM struct.
type ClasslessStaticRoute struct {
	// DestinationDescriptor is the network address and prefix length in CIDR form,
	// e.g., "192.168.1.0/24" or "2001:db8::/64". Must be a valid IPv4 or IPv6 CIDR;
	// the family must match the enclosing IPAM field (ipv4 or ipv6).
	// +kubebuilder:validation:MaxLength=49
	// +required
	DestinationDescriptor string `json:"destinationDescriptor"`
	// Router is the IP address of the router for this route. Must be a valid IPv4 or IPv6
	// address; the family must match the enclosing IPAM field (ipv4 or ipv6).
	// +kubebuilder:validation:MaxLength=45
	// +required
	Router string `json:"router"`
}

// Nameserver is an IP address of the family of the enclosing IPAM field (ipv4 or ipv6).
// +kubebuilder:validation:MaxLength=45
type Nameserver string

// Timeserver is an IP address of the family of the enclosing IPAM field (ipv4 or ipv6).
// +kubebuilder:validation:MaxLength=45
type Timeserver string

// Netboot configuration.
type Netboot struct {
	// Disabled indicates that netbooting should not be enabled for this interface.
	// When true, no netboot options will be provided in DHCP and iPXE script requests will return 404.
	// Unset is equivalent to false.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// IPv4 is the options specific for network booting via IPv4.
	// +optional
	IPv4 *Netbootv4 `json:"ipv4,omitempty"`

	// IPv6 is the options specific for network booting via IPv6.
	// +optional
	IPv6 *Netbootv6 `json:"ipv6,omitempty"`

	// IPXE configuration.
	// +optional
	IPXE *IPXE `json:"ipxe,omitempty"`

	// PXELINUX configuration.
	// +optional
	PXELINUX *PXELINUX `json:"pxelinux,omitempty"`

	// RPI (Raspberry Pi) configuration.
	// +optional
	RPI *RPI `json:"rpi,omitempty"`
}

// +kubebuilder:validation:XValidation:rule=(has(self.tftpServerName) && self.tftpServerName != "") == (has(self.bootFileName) && self.bootFileName != ""),message="TFTPServerName and BootFileName must both be specified or both be empty"
type Netbootv4 struct {
	// BootFileName is the boot file name. DHCP option 67.
	// Used for explicit boot file configuration, required by some network boot clients
	// like NVIDIA NVOS switches.
	// If specified, TFTPServerName must also be specified.
	// +optional
	BootFileName string `json:"bootFileName,omitempty"`

	// TFTPServerName is the TFTP server name or IP address. DHCP option 66.
	// Used for explicit TFTP server configuration, required by some network boot clients
	// like NVIDIA NVOS switches.
	// If specified, BootFileName must also be specified.
	// +optional
	TFTPServerName string `json:"tftpServerName,omitempty"`
}
type Netbootv6 struct {
	// BootFileParams are parameters for the boot file. DHCPv6 option 60 (RFC 5970).
	// +optional
	// +listType=atomic
	BootFileParams []string `json:"bootFileParams,omitempty"`

	// BootFileURL is the URL for the boot file. DHCPv6 option 59 (RFC 5970).
	// It must be a tftp, http or https URL, and an IPv6 literal host must be in brackets.
	// For example, "tftp://[2001:db8::1]/bootx64.efi".
	// +optional
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:Pattern=`^[a-zA-Z][a-zA-Z0-9+.-]*://(\[[0-9a-fA-F:.]+\]|[^/:\[\]]+)(:[0-9]+)?([/?#].*)?$`
	// +kubebuilder:validation:XValidation:rule="size(self) == 0 || (isURL(self) && url(self).getScheme() in [\"tftp\", \"http\", \"https\"])",message="bootFileURL must be a tftp, http or https URL"
	BootFileURL string `json:"bootFileURL,omitempty"`
}

// IPXE configuration.
// +kubebuilder:validation:XValidation:rule="!(has(self.script) && self.script != \"\" && has(self.url) && self.url != \"\")",message="script and url are mutually exclusive"
type IPXE struct {
	// Binary, when defined, overrides the default mapping of architecture to iPXE binary.
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
	// +kubebuilder:validation:XValidation:rule="self == '' || self.startsWith('#!ipxe')",message="script must start with '#!ipxe'"
	Script string `json:"script,omitempty"`

	// URL, when defined, overrides the Tinkerbell iPXE script and uses iPXE's chainloading capabilities
	// to download and run the script at the defined URL.
	// +optional
	URL string `json:"url,omitempty"`
}

// PXELINUX represents PXELinux configuration, for u-boot "pxelinux.cfg" booting.
// This is used to instruct Tinkerbell's TFTP serve to serve out certain files for PXELinux booting.
// The main pxelinux.cfg asset (in "extlinux" format; see u-boot's distroboot and bootstd docs) is
// generated on the fly from a template defined in PXELINUX struct.
// Other assets (eg, vmlinuz, initramfs, dtb) are served from a filesystem path defined in Tinkerbell configuration,
type PXELINUX struct {
	// Config, when defined, overrides the default PXELinux config to use for generating the pxelinux.cfg file.
	// It should be in "extlinux" format, as per u-boot's distroboot and bootstd documentation.
	// +optional
	Config string `json:"config,omitempty"`
}

// RPI represents Raspberry Pi network boot configuration.
// The RaspberryPi's EEPROM firmware can do its own network booting, without u-boot (PXELinux) or iPXE involved.
// It does so by prefixing requests to certain filenames with the Pi's serial number.
// Such serial number is NOT related to the MAC address, and is unique to each Pi.
// The firmware will send requests to "<PiSerialNum>/start4.elf" to get 2nd-stage firmware, then for
// "<PiSerialNum>/config.txt", and "<PiSerialNum>/cmdline.txt" when doing network booting.
// config.txt and cmdline.txt will come directly from the Hardware data,
// while everything else (eg all binaries) will be served after a PiSerialNum -> FirmwareURIPath mapping.
// All this depends on being able to find the Hardware via an IP-address lookup; no lookups are done vs the PiSerialNum.
// https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#TFTP_PREFIX
type RPI struct {
	// ConfigTxt, when defined, overrides the default Raspberry Pi config.txt file content.
	// When not specified, a default config.txt will be used.
	// +optional
	ConfigTxt string `json:"configTxt,omitempty"`

	// FirmwarePath, when defined, overrides the default path to firmware files.
	// This must be coordinated with where Tinkerbell serves Raspberry Pi firmware files. See the Tinkerbell docs for details.
	//
	// This is an internal URI rewrite. The Raspberry Pi will always request files from the TFTPPrefix.
	// This field defines a common backend location for the generic firmware files, which will be used for all other files and binaries.
	// A Raspberry Pi will still request all files from the TFTPPrefix, but Tinkerbell will serve files from FirmwarePath when the request path starts with the Pi's serial number.
	// For example, if FirmwarePath is set to "captainos/rpi-firmware", then a request for "<PiSerialNum>/start4.elf" will be served from "rpi-firmware/start4.elf" in Tinkerbell's filesystem.
	// There are 3 main purposes for this:
	// 1. avoid having to duplicate firmware files for each Raspberry Pi.
	// 2. allow each Hardware object to specify a different set of firmware files if needed.
	// 3. decouple generic firmware files from the, optionally defined, hardware specific config.txt and cmdline.txt files.
	// +optional
	FirmwarePath string `json:"firmwarePath,omitempty"`

	// SerialNumber is the Raspberry Pi's serial number.
	// This can be the full serial number or just the last 8 characters.
	// To use any of the fields in the RPI struct, SerialNumber must be specified to allow Tinkerbell to associate this configuration with the correct Raspberry Pi.
	// +required
	SerialNumber string `json:"serialNumber"`
}

// Reference identifies an object in the same namespace as the referring object.
type Reference struct {
	// Group of the referent. Empty means the core API group.
	// More info: https://kubernetes.io/docs/reference/using-api/#api-groups
	// +optional
	Group string `json:"group,omitempty"`

	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +required
	Name string `json:"name"`

	// Resource of the referent. Must be the pluralized kind of the referent. Must be all lowercase.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +required
	Resource string `json:"resource"`

	// API version of the referent. Templates depend on the referent's schema, so the version is
	// pinned rather than resolved to the cluster's preferred version.
	// More info: https://kubernetes.io/docs/reference/using-api/#api-versioning
	// +required
	Version string `json:"version"`
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
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`
}

// HardwareStatus defines the observed state of Hardware.
type HardwareStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Attributes describes the hardware itself, as observed by Tinkerbell. It is
	// split by collection path rather than merged, so each collector owns a
	// disjoint subtree and neither can clobber the other.
	//+optional
	Attributes *HardwareAttributes `json:"attributes,omitempty"`
}
