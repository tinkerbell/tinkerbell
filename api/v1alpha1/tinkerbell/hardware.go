package tinkerbell

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HardwareState represents the hardware state.
type HardwareState string

const (
	// HardwareError represents hardware that is in an error state.
	HardwareError = HardwareState("Error")

	// HardwareReady represents hardware that is in a ready state.
	HardwareReady = HardwareState("Ready")
)

// +kubebuilder:object:root=true

// HardwareList contains a list of Hardware.
type HardwareList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Hardware `json:"items"`
}

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hardware,scope=Namespaced,categories=tinkerbell,singular=hardware,shortName=hw
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".status.state",name=State,type=string
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io=
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io/move=

// Hardware is the Schema for the Hardware API.
type Hardware struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HardwareSpec   `json:"spec,omitempty"`
	Status HardwareStatus `json:"status,omitempty"`
}

// HardwareSpec defines the desired state of Hardware.
type HardwareSpec struct {
	// AgentID is the unique identifier an Agent uses that is associated with this Hardware.
	// This is used to identify Hardware during the discovery and enrollment process.
	// It is typically the MAC address of the primary network interface.
	AgentID string `json:"agentID,omitempty"`

	// Auto is the configuration for the automatic capabilities.
	Auto AutoCapabilities `json:"auto,omitempty"`

	// BMCRef contains a relation to a BMC state management type in the same
	// namespace as the Hardware. This may be used for BMC management by
	// orchestrators.
	//+optional
	BMCRef *corev1.TypedLocalObjectReference `json:"bmcRef,omitempty"`

	//+optional
	Interfaces []Interface `json:"interfaces,omitempty"`

	// References allows for linking custom resource objects of any kind to this Hardware object.
	// These are available in Templates for templating. They are referenced by the name of the reference.
	// For example, given a reference with the name "lvm", you can access it in a template with .references.lvm.
	//+optional
	References map[string]Reference `json:"references,omitempty"`

	//+optional
	// Metadata string `json:"metadata,omitempty"`

	//+optional
	Metadata *HardwareMetadata `json:"metadata,omitempty"`

	//+optional
	TinkVersion int64 `json:"tinkVersion,omitempty"`

	//+optional
	Disks []Disk `json:"disks,omitempty"`

	// Resources represents known resources that are available on a machine.
	// Resources may be used for scheduling by orchestrators.
	//+optional
	Resources map[string]resource.Quantity `json:"resources,omitempty"`

	// UserData is the user data to configure in the hardware's
	// metadata
	//+optional
	UserData *string `json:"userData,omitempty"`

	// VendorData is the vendor data to configure in the hardware's
	// metadata
	//+optional
	VendorData *string `json:"vendorData,omitempty"`
}

type Reference struct {
	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	Namespace string `json:"namespace,omitempty"`

	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name,omitempty"`

	// Group of the referent.
	// More info: https://kubernetes.io/docs/reference/using-api/#api-groups
	Group string `json:"group,omitempty"`

	// API version of the referent.
	// More info: https://kubernetes.io/docs/reference/using-api/#api-versioning
	Version string `json:"version,omitempty"`

	// Resource of the referent. Must be the pluralized kind of the referent. Must be all lowercase.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Resource string `json:"resource,omitempty"`
}

// Interface represents configuration related to a network interface.
type Interface struct {
	// Netboot configuration.
	//+optional
	Netboot *Netboot `json:"netboot,omitempty"`

	// Isoboot configuration.
	// +optional
	Isoboot *Isoboot `json:"isoboot,omitempty"`

	// DHCP configuration.
	//+optional
	DHCP *DHCP `json:"dhcp,omitempty"`

	// DisableDHCP disables DHCP for this interface.
	// +kubebuilder:default=false
	// +optional
	DisableDHCP bool `json:"disableDhcp,omitempty"`
}

// Netboot configuration.
type Netboot struct {
	//+optional
	AllowPXE *bool `json:"allowPXE,omitempty"`

	//+optional
	AllowWorkflow *bool `json:"allowWorkflow,omitempty"`

	//+optional
	IPXE *IPXE `json:"ipxe,omitempty"`

	//+optional
	OSIE *OSIE `json:"osie,omitempty"`
}

// Isoboot configuration for booting a client using an ISO image.
type Isoboot struct {
	// SourceISO is the source url where HookOS, an Operating System Installation Environment (OSIE), ISO lives.
	// It must be a valid url.URL{} object and must have a url.URL{}.Scheme of HTTP or HTTPS.
	//+optional
	// +kubebuilder:validation:Format=uri
	SourceISO string `json:"sourceISO,omitempty"`
}

// IPXE configuration.
type IPXE struct {
	URL      string `json:"url,omitempty"`
	Contents string `json:"contents,omitempty"`
	// Binary, when defined, overrides Smee's default mapping of architecture to iPXE binary.
	// The following binary names are supported:
	// - undionly.kpxe
	// - ipxe.efi
	// - snp-arm64.efi
	// - snp-x86_64.efi
	// See the iPXE Architecture Mapping documentation for more details.
	Binary string `json:"binary,omitempty"`
}

// OSIE configuration.
type OSIE struct {
	BaseURL string `json:"baseURL,omitempty"`
	Kernel  string `json:"kernel,omitempty"`
	Initrd  string `json:"initrd,omitempty"`
}

// DHCP configuration.
// +kubebuilder:validation:XValidation:rule=(has(self.tftp_server_name) && self.tftp_server_name != "") == (has(self.boot_file_name) && self.boot_file_name != ""),message="TFTPServerName and BootFileName must both be specified or both be empty"
type DHCP struct {
	// +kubebuilder:validation:Pattern="([0-9a-f]{2}[:]){5}([0-9a-f]{2})"
	MAC         string   `json:"mac,omitempty"`
	Hostname    string   `json:"hostname,omitempty"`
	DomainName  string   `json:"domain_name,omitempty"`
	LeaseTime   int64    `json:"lease_time,omitempty"`
	NameServers []string `json:"name_servers,omitempty"`
	TimeServers []string `json:"time_servers,omitempty"`
	Arch        string   `json:"arch,omitempty"`
	UEFI        bool     `json:"uefi,omitempty"`
	IfaceName   string   `json:"iface_name,omitempty"`
	IP          *IP      `json:"ip,omitempty"`
	// validation pattern for VLANDID is a string number between 0-4096
	// +kubebuilder:validation:Pattern="^(([0-9][0-9]{0,2}|[1-3][0-9][0-9][0-9]|40([0-8][0-9]|9[0-6]))(,[1-9][0-9]{0,2}|[1-3][0-9][0-9][0-9]|40([0-8][0-9]|9[0-6]))*)$"
	VLANID string `json:"vlan_id,omitempty"`
	// ClasslessStaticRoutes defines static routes to be sent via DHCP option 121 (RFC 3442).
	//+optional
	ClasslessStaticRoutes []ClasslessStaticRoute `json:"classless_static_routes,omitempty"`
	// TFTPServerName is the TFTP server name or IP address (DHCP option 66).
	// Used for explicit TFTP server configuration, required by some network boot clients
	// like NVIDIA NVOS switches.
	// If specified, BootFileName must also be specified.
	//+optional
	TFTPServerName string `json:"tftp_server_name,omitempty"`
	// BootFileName is the boot file name (DHCP option 67).
	// Used for explicit boot file configuration, required by some network boot clients
	// like NVIDIA NVOS switches.
	// If specified, TFTPServerName must also be specified.
	//+optional
	BootFileName string `json:"boot_file_name,omitempty"`
}

// IP configuration.
type IP struct {
	Address string `json:"address,omitempty"`
	Netmask string `json:"netmask,omitempty"`
	Gateway string `json:"gateway,omitempty"`
	Family  int64  `json:"family,omitempty"`
}

// ClasslessStaticRoute represents a classless static route for DHCP option 121 (RFC 3442).
type ClasslessStaticRoute struct {
	// DestinationDescriptor is the network address and prefix length.
	// The format is "network/prefixlength", e.g., "192.168.1.0/24" or "10.0.0.0/8".
	// +kubebuilder:validation:Pattern=`^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)/(3[0-2]|[12]?[0-9])$`
	DestinationDescriptor string `json:"destination_descriptor"`
	// Router is the IP address of the router for this route.
	// +kubebuilder:validation:Pattern=`^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`
	Router string `json:"router"`
}

type HardwareMetadata struct {
	State        string                `json:"state,omitempty"`
	BondingMode  int64                 `json:"bonding_mode,omitempty"`
	Manufacturer *MetadataManufacturer `json:"manufacturer,omitempty"`
	Instance     *MetadataInstance     `json:"instance,omitempty"`
	Custom       *MetadataCustom       `json:"custom,omitempty"`
	Facility     *MetadataFacility     `json:"facility,omitempty"`
}

type MetadataManufacturer struct {
	ID   string `json:"id,omitempty"`
	Slug string `json:"slug,omitempty"`
}

type MetadataInstance struct {
	ID                  string                           `json:"id,omitempty"`
	State               string                           `json:"state,omitempty"`
	Hostname            string                           `json:"hostname,omitempty"`
	AllowPxe            bool                             `json:"allow_pxe,omitempty"`
	Rescue              bool                             `json:"rescue,omitempty"`
	OperatingSystem     *MetadataInstanceOperatingSystem `json:"operating_system,omitempty"`
	AlwaysPxe           bool                             `json:"always_pxe,omitempty"`
	IpxeScriptURL       string                           `json:"ipxe_script_url,omitempty"`
	Ips                 []*MetadataInstanceIP            `json:"ips,omitempty"`
	Userdata            string                           `json:"userdata,omitempty"`
	CryptedRootPassword string                           `json:"crypted_root_password,omitempty"`
	Tags                []string                         `json:"tags,omitempty"`
	Storage             *MetadataInstanceStorage         `json:"storage,omitempty"`
	SSHKeys             []string                         `json:"ssh_keys,omitempty"`
	NetworkReady        bool                             `json:"network_ready,omitempty"`
}

type MetadataInstanceOperatingSystem struct {
	Slug     string `json:"slug,omitempty"`
	Distro   string `json:"distro,omitempty"`
	Version  string `json:"version,omitempty"`
	ImageTag string `json:"image_tag,omitempty"`
	OsSlug   string `json:"os_slug,omitempty"`
}

type MetadataInstanceIP struct {
	Address    string `json:"address,omitempty"`
	Netmask    string `json:"netmask,omitempty"`
	Gateway    string `json:"gateway,omitempty"`
	Family     int64  `json:"family,omitempty"`
	Public     bool   `json:"public,omitempty"`
	Management bool   `json:"management,omitempty"`
}

type MetadataInstanceStorage struct {
	Disks       []*MetadataInstanceStorageDisk       `json:"disks,omitempty"`
	Raid        []*MetadataInstanceStorageRAID       `json:"raid,omitempty"`
	Filesystems []*MetadataInstanceStorageFilesystem `json:"filesystems,omitempty"`
}

type MetadataInstanceStorageDisk struct {
	Device     string                                  `json:"device,omitempty"`
	WipeTable  bool                                    `json:"wipe_table,omitempty"`
	Partitions []*MetadataInstanceStorageDiskPartition `json:"partitions,omitempty"`
}

type MetadataInstanceStorageDiskPartition struct {
	Label    string `json:"label,omitempty"`
	Number   int64  `json:"number,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Start    int64  `json:"start,omitempty"`
	TypeGUID string `json:"type_guid,omitempty"`
}

type MetadataInstanceStorageRAID struct {
	Name    string   `json:"name,omitempty"`
	Level   string   `json:"level,omitempty"`
	Devices []string `json:"devices,omitempty"`
	Spare   int64    `json:"spare,omitempty"`
}

type MetadataInstanceStorageFilesystem struct {
	Mount *MetadataInstanceStorageMount `json:"mount,omitempty"`
}

type MetadataInstanceStorageMount struct {
	Device string                                         `json:"device,omitempty"`
	Format string                                         `json:"format,omitempty"`
	Files  []*MetadataInstanceStorageFile                 `json:"files,omitempty"`
	Create *MetadataInstanceStorageMountFilesystemOptions `json:"create,omitempty"`
	Point  string                                         `json:"point,omitempty"`
}

type MetadataInstanceStorageFile struct {
	Path     string `json:"path,omitempty"`
	Contents string `json:"contents,omitempty"`
	Mode     int64  `json:"mode,omitempty"`
	UID      int64  `json:"uid,omitempty"`
	GID      int64  `json:"gid,omitempty"`
}

type MetadataInstanceStorageMountFilesystemOptions struct {
	Force   bool     `json:"force,omitempty"`
	Options []string `json:"options,omitempty"`
}

type MetadataCustom struct {
	PreinstalledOperatingSystemVersion *MetadataInstanceOperatingSystem `json:"preinstalled_operating_system_version,omitempty"`
	PrivateSubnets                     []string                         `json:"private_subnets,omitempty"`
}

type MetadataFacility struct {
	PlanSlug        string `json:"plan_slug,omitempty"`
	PlanVersionSlug string `json:"plan_version_slug,omitempty"`
	FacilityCode    string `json:"facility_code,omitempty"`
}

// Disk represents a disk device for Tinkerbell Hardware.
type Disk struct {
	//+optional
	Device string `json:"device,omitempty"`
}

// HardwareStatus defines the observed state of Hardware.
type HardwareStatus struct {
	//+optional
	State HardwareState `json:"state,omitempty"`

	// BMCInventory contains hardware attributes collected out-of-band via the BMC's
	// Redfish API. It is updated by the Machine controller when BMC connectivity is
	// established. This data is available before the machine boots. IPMI-only BMCs
	// cannot provide this data and will not populate this field.
	//+optional
	BMCInventory *BMCInventory `json:"bmcInventory,omitempty"`
}

// BMCInventory is hardware data collected out-of-band via the BMC.
// Fields mirror the bmclib/common.Device structure. Every field is optional: BMC
// vendors/protocols vary widely in what they report (Redfish, IPMI, vendor-specific
// APIs all populate a different subset), so absence of a field here reflects what
// the BMC/collection driver reports, not an error.
type BMCInventory struct {
	// LastUpdated is the time at which this inventory was last refreshed.
	//+optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// CollectionMethod identifies which bmclib driver produced this inventory
	// (e.g. "redfish", "dell", "supermicro", "asrockrack", "openbmc"). Field
	// completeness varies significantly by driver, so consumers should use this to
	// interpret absent fields rather than treating them as errors.
	//+optional
	CollectionMethod string `json:"collectionMethod,omitempty"`

	// Product describes the overall system identity (e.g. the Redfish "System"
	// resource's own vendor/model/serial) — the machine's own asset tag, distinct
	// from any individual component like the Mainboard or BMC.
	//+optional
	Product *BMCProduct `json:"product,omitempty"`

	// BIOS describes the system BIOS firmware.
	//+optional
	BIOS *BMCFirmwareComponent `json:"bios,omitempty"`

	// BMC describes the BMC firmware.
	//+optional
	BMC *BMCFirmwareComponent `json:"bmc,omitempty"`

	// Mainboard describes the mainboard.
	//+optional
	Mainboard *BMCComponent `json:"mainboard,omitempty"`

	// CPUs lists CPUs as reported by the BMC.
	//+optional
	CPUs []BMCCPUComponent `json:"cpus,omitempty"`

	// Memory lists memory modules as reported by the BMC, with firmware details.
	//+optional
	Memory []BMCMemoryComponent `json:"memory,omitempty"`

	// NICs lists network adapters as reported by the BMC, with firmware details.
	//+optional
	NICs []BMCNICComponent `json:"nics,omitempty"`

	// Drives lists storage drives as reported by the BMC, with firmware details.
	//+optional
	Drives []BMCDriveComponent `json:"drives,omitempty"`

	// StorageControllers lists storage controllers as reported by the BMC.
	//+optional
	StorageControllers []BMCComponent `json:"storageControllers,omitempty"`

	// PSUs lists power supply units.
	//+optional
	PSUs []BMCPSUComponent `json:"psus,omitempty"`

	// TPMs lists trusted platform modules.
	//+optional
	TPMs []BMCComponent `json:"tpms,omitempty"`

	// GPUs lists GPU/accelerator devices as reported by the BMC.
	//+optional
	GPUs []BMCComponent `json:"gpus,omitempty"`
}

// BMCProduct describes the overall system identity as reported by the BMC.
type BMCProduct struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	ProductName string `json:"productName,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Status *BMCStatus `json:"status,omitempty"`
}

// BMCStatus is health/state info, mirroring bmclib/common.Status. PostCode is only
// meaningful for BIOS (POST diagnostics) and will be empty elsewhere.
type BMCStatus struct {
	//+optional
	Health string `json:"health,omitempty"`
	//+optional
	State string `json:"state,omitempty"`
	//+optional
	PostCode int32 `json:"postCode,omitempty"`
	//+optional
	PostCodeStatus string `json:"postCodeStatus,omitempty"`
}

// BMCFirmwareComponent is a hardware component that carries firmware version info.
type BMCFirmwareComponent struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	FirmwareInstalled string `json:"firmwareInstalled,omitempty"`
	//+optional
	Status *BMCStatus `json:"status,omitempty"`
	// NIC is the BMC's own out-of-band management network interface. It is
	// distinct from BMCInventory.NICs, which lists the host's NICs.
	//+optional
	NIC *BMCNICComponent `json:"nic,omitempty"`
}

// BMCComponent is a basic hardware component without specialized fields.
type BMCComponent struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Description string `json:"description,omitempty"`
	//+optional
	FirmwareInstalled string `json:"firmwareInstalled,omitempty"`
	//+optional
	Status *BMCStatus `json:"status,omitempty"`
}

// BMCPSUComponent describes a power supply unit as reported by the BMC.
type BMCPSUComponent struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Description string `json:"description,omitempty"`
	//+optional
	Status *BMCStatus `json:"status,omitempty"`
	//+optional
	PowerCapacityWatts int64 `json:"powerCapacityWatts,omitempty"`
}

// BMCCPUComponent describes a CPU as reported by the BMC. Note: on the Redfish
// collection path (Dell/Supermicro/gofish/OpenBMC), SerialNumber is always empty
// upstream in bmclib — not a bug in the conversion, just what the BMC exposes.
type BMCCPUComponent struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Slot string `json:"slot,omitempty"`
	//+optional
	Cores uint32 `json:"cores,omitempty"`
	//+optional
	Threads uint32 `json:"threads,omitempty"`
	//+optional
	ClockSpeedMHz uint32 `json:"clockSpeedMHz,omitempty"`
	//+optional
	FirmwareInstalled string `json:"firmwareInstalled,omitempty"`
}

// BMCMemoryComponent describes a memory module as reported by the BMC. Note: on
// the Redfish collection path, Model is always empty upstream in bmclib — not a
// bug in the conversion, just what the BMC exposes.
type BMCMemoryComponent struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Slot string `json:"slot,omitempty"`
	//+optional
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	//+optional
	SpeedMHz uint32 `json:"speedMHz,omitempty"`
	//+optional
	FormFactor string `json:"formFactor,omitempty"`
	//+optional
	PartNumber string `json:"partNumber,omitempty"`
	//+optional
	FirmwareInstalled string `json:"firmwareInstalled,omitempty"`
}

// BMCNICComponent describes a network adapter as reported by the BMC.
type BMCNICComponent struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	FirmwareInstalled string `json:"firmwareInstalled,omitempty"`
	// Ports lists the individual physical ports on this adapter.
	//+optional
	Ports []BMCNICPort `json:"ports,omitempty"`
}

// BMCNICPort describes a single physical port on a network adapter.
type BMCNICPort struct {
	//+optional
	PortID string `json:"portID,omitempty"`
	//+optional
	MACAddress string `json:"macAddress,omitempty"`
	//+optional
	LinkStatus string `json:"linkStatus,omitempty"`
	//+optional
	SpeedMbps uint32 `json:"speedMbps,omitempty"`
	//+optional
	MTU uint32 `json:"mtu,omitempty"`
}

// BMCDriveComponent describes a storage drive as reported by the BMC.
type BMCDriveComponent struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	// WWN is the World Wide Name, a standard unique storage identifier.
	//+optional
	WWN string `json:"wwn,omitempty"`
	//+optional
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	//+optional
	Type string `json:"type,omitempty"`
	// SmartStatus is the drive's self-reported SMART health status (e.g. "ok",
	// "predict-failure").
	//+optional
	SmartStatus string `json:"smartStatus,omitempty"`
	//+optional
	FirmwareInstalled string `json:"firmwareInstalled,omitempty"`
}

// AutoCapabilities defines the configuration for the automatic capabilities of this Hardware.
type AutoCapabilities struct {
	// EnrollmentEnabled enables automatic enrollment of the Hardware.
	// When set to true, auto enrollment will create Workflows for this Hardware.
	// +kubebuilder:default=false
	EnrollmentEnabled bool `json:"enrollmentEnabled,omitempty"`
}
