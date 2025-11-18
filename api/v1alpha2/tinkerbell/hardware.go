package tinkerbell

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hardware,scope=Namespaced,categories=tinkerbell,singular=hardware,shortName=hw
// +kubebuilder:storageversion
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io=
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io/move=

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
	Items           []Hardware `json:"items"`
}

// HardwareSpec defines the desired state of Hardware.
type HardwareSpec struct {
	// AgentID is the unique identifier an Agent uses that is associated with this Hardware.
	// This is used to identify Hardware during the discovery and enrollment process.
	// It is typically the MAC address of the primary network interface.
	AgentID string `json:"agentID,omitempty"`

	// Auto is the configuration for the automatic capabilities.
	Auto AutoCapabilities `json:"auto,omitempty"`

	// Instance describes instance specific data that is generally unused by Tinkerbell core.
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

// AutoCapabilities defines the configuration for the automatic capabilities of this Hardware.
type AutoCapabilities struct {
	// EnrollmentEnabled enables automatic enrollment of the Hardware.
	// When set to true, auto enrollment will create Workflows for this Hardware.
	// +kubebuilder:default=false
	EnrollmentEnabled bool `json:"enrollmentEnabled,omitempty"`
}

// Instance describes instance specific data. Instance specific data is typically dependent on the
// permanent OS that a piece of hardware runs. This data is often served by an instance metadata
// service such as Tinkerbell's Hegel. The core Tinkerbell stack does not leverage this data.
type Instance struct {
	// KernelParams passed to the kernel when launching the OSIE. Parameters are joined with a space.
	// +optional
	KernelParams []string `json:"kernelParams,omitempty"`

	// Metadata is data following the cloud-init NoCloud datasource for meta-data
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
	// {{ (index .hardware.networkInterfaces "52:54:00:41:05:c6").dhcp.ipam.ipv4.address }}
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
	// {{ (index .hardware.networkInterfaces "52:54:00:41:05:c6").dhcp.ipam.ipv4.address }}
	// +optional
	NetworkConfig *string `json:"networkConfig,omitempty"`

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
	// {{ (index .hardware.networkInterfaces "52:54:00:41:05:c6").dhcp.ipam.ipv4.address }}
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
	// Attributes related to the network interface.
	Attributes *Attributes `json:"attributes,omitempty"`

	// DHCP is the basic network information for serving DHCP requests. Required when DisableDHCP is false.
	// +optional
	DHCP *DHCP `json:"dhcp,omitempty"`

	// DisableDHCP disables DHCP for this interface. Implies DisableNetboot.
	// +kubebuilder:default=false
	DisableDHCP bool `json:"disableDHCP,omitempty"`

	// DisableNetboot disables netbooting for this interface. The interface will still receive
	// network information specified by DHCP.
	// +kubebuilder:default=false
	DisableNetboot bool `json:"disableNetboot,omitempty"`

	// IPAM is the IP address management for this interface.
	IPAM IPAM `json:"ipam,omitempty"`

	// Isoboot configuration.
	// +optional
	Isoboot *Isoboot `json:"isoboot,omitempty"`

	// Netboot configuration.
	// +optional
	Netboot *Netboot `json:"netboot,omitempty"`
}

// Attributes related to a network interface.
type Attributes struct {
	// These are from the old v1alpha1. Need to figure out what to do with them.
	Arch      string `json:"arch,omitempty"`
	IfaceName string `json:"ifaceName,omitempty"`
	UEFI      bool   `json:"uefi,omitempty"`

	// VLANID is a VLAN ID between 0 and 4096.
	// +kubebuilder:validation:Pattern=`^(([0-9][0-9]{0,2}|[1-3][0-9][0-9][0-9]|40([0-8][0-9]|9[0-6]))(,[1-9][0-9]{0,2}|[1-3][0-9][0-9][0-9]|40([0-8][0-9]|9[0-6]))*)$`
	// +optional
	VLANID *string `json:"vlanID,omitempty"`
}

// DHCP describes basic network configuration to be served in DHCP OFFER responses. It can be
// considered a DHCP reservation.
// +kubebuilder:validation:XValidation:rule=(has(self.tftpServerName) && self.tftpServerName != "") == (has(self.bootFileName) && self.bootFileName != ""),message="TFTPServerName and BootFileName must both be specified or both be empty"
type DHCP struct {
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
	DomainName string `json:"domainName,omitempty"`

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
	LeaseTimeSeconds *int64 `json:"leaseTimeSeconds"`

	// Nameservers corresponding to DHCP option 6.
	// +optional
	Nameservers []Nameserver `json:"nameservers,omitempty"`

	// TFTPServerName is the TFTP server name or IP address. DHCP option 66.
	// Used for explicit TFTP server configuration, required by some network boot clients
	// like NVIDIA NVOS switches.
	// If specified, BootFileName must also be specified.
	// +optional
	TFTPServerName string `json:"tftpServerName,omitempty"`

	// Timeservers corresponding to DHCP option 42.
	// +optional
	Timeservers []Timeserver `json:"timeservers,omitempty"`
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

// Nameserver is an IP or hostname.
// +kubebuilder:validation:Pattern=`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$|^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$`
type Nameserver string

// Timeserver is an IP or hostname.
// +kubebuilder:validation:Pattern=`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$|^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$`
type Timeserver string

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
	Address string `json:"address,omitempty"`
	// Gateway is the default gateway address to serve.
	Gateway string `json:"gateway,omitempty"`
	// Prefix is the subnet length.
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
	// IPXE configuration.
	// +optional
	IPXE *IPXE `json:"ipxe,omitempty"`

	// OSIE (Operating System Installation Environment) configuration.
	// +optional
	OSIE *OSIE `json:"osie,omitempty"`
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
	Binary string `json:"binary,omitempty"`
	// Contents of an iPXE script.
	Contents string `json:"contents,omitempty"`
	// URL of an iPXE script.
	URL string `json:"url,omitempty"`
}

// OSIE configuration.
type OSIE struct {
	// BaseURL where OSIE kernel and initrd are hosted.
	BaseURL string `json:"baseURL,omitempty"`
	// Initrd name of the OSIE.
	Initrd string `json:"initrd,omitempty"`
	// Kernel name of the OSIE.
	Kernel string `json:"kernel,omitempty"`
}

// References
type References struct {
	// Additional
	Additional map[string]Reference `json:"additional,omitempty"`
	// Builtin
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
	// +optional
	Name string `json:"name,omitempty"`
}
