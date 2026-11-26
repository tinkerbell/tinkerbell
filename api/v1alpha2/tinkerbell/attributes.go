package tinkerbell

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// AgentAttributes is a Quamina pattern that matches against Agent attributes.
// Multiple fields within a single AgentAttributes are combined with AND logic (all specified fields must match).
// Multiple values within each FieldPattern field are combined with OR logic (any value can match).

// HardwareAttributes holds hardware attribute subtrees, one per collection path.
// Each subtree is written by a single owner and the owners touch disjoint paths,
// so no merge keys or per-field precedence rules are needed.
type HardwareAttributes struct {
	// OutOfBand holds attributes collected via the BMC (Redfish or vendor API).
	// Available before the machine boots. IPMI-only BMCs cannot provide this data
	// and will not populate this field.
	//+optional
	OutOfBand *Attributes `json:"outOfBand,omitempty"`

	// InBand holds attributes reported by the Tink Agent from within the running
	// OS. Only available once the machine has booted and the Agent has checked
	// in; unlike OutOfBand, it cannot see hardware the OS doesn't expose (e.g. the
	// BMC's own firmware, PSUs) but can see hardware the BMC doesn't (e.g. the PCI
	// bus, loaded drivers).
	//+optional
	InBand *Attributes `json:"inBand,omitempty"`
}

// Attributes is a source-agnostic description of a machine's hardware, shared by
// both the out-of-band and in-band collection paths. It is the superset of what
// either can report: every field is optional, and an absent field means the
// producing source did not report it, not an error. Field coverage varies by
// source, and for the out-of-band path also by BMC vendor/protocol. Fields only
// one source can populate are marked as such.
type Attributes struct {
	// LastUpdated is the time at which this subtree was last refreshed.
	//+optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// CollectionMethod identifies what produced this subtree: "agent" for in-band,
	// or the bmclib driver for out-of-band (e.g. "redfish", "dell", "supermicro",
	// "asrockrack", "openbmc"). Consumers should use this to interpret absent
	// fields rather than treating them as errors.
	//+optional
	CollectionMethod string `json:"collectionMethod,omitempty"`

	// CPU holds aggregate CPU totals and per-socket detail. Both in-band and
	// out-of-band report sockets; CPUSocket.Slot is a numeric processor ID
	// in-band and a physical socket label (e.g. "CPU1") out-of-band.
	//+optional
	CPU *CPU `json:"cpu,omitempty"`

	// Memory holds aggregate memory totals and per-module detail. In-band reports
	// the totals; out-of-band reports the modules.
	//+optional
	Memory *Memory `json:"memory,omitempty"`

	// BlockDevices lists storage drives.
	//+optional
	BlockDevices []BlockDevice `json:"blockDevices,omitempty"`

	// NetworkInterfaces lists the host's network adapters. Distinct from
	// Hardware.spec.interfaces, which is DHCP/netboot configuration rather than
	// observed hardware.
	//+optional
	NetworkInterfaces []AttributeNetworkInterface `json:"networkInterfaces,omitempty"`

	// GPUDevices lists GPU and accelerator devices.
	//+optional
	GPUDevices []GPUDevice `json:"gpuDevices,omitempty"`

	// PCIDevices lists PCI devices. In-band only: enumerating the PCI bus requires
	// a running OS.
	//+optional
	PCIDevices []PCIDevice `json:"pciDevices,omitempty"`

	// Chassis describes the enclosure.
	//+optional
	Chassis *Chassis `json:"chassis,omitempty"`

	// Baseboard describes the mainboard/motherboard.
	//+optional
	Baseboard *Baseboard `json:"baseboard,omitempty"`

	// BIOS describes the system BIOS firmware.
	//+optional
	BIOS *BIOS `json:"bios,omitempty"`

	// Product describes the overall system identity — the machine's own asset
	// details, distinct from any individual component.
	//+optional
	Product *Product `json:"product,omitempty"`

	// BMC describes the BMC's own firmware and management NIC. Out-of-band only.
	//+optional
	BMC *AttributeBMC `json:"bmc,omitempty"`

	// PSUs lists power supply units. Out-of-band only.
	//+optional
	PSUs []PSU `json:"psus,omitempty"`

	// TPMs lists trusted platform modules. Out-of-band only.
	//+optional
	TPMs []AttributeTPM `json:"tpms,omitempty"`

	// StorageControllers lists storage controllers. Out-of-band only.
	//+optional
	StorageControllers []StorageController `json:"storageControllers,omitempty"`
}

// CPU holds aggregate CPU totals plus per-socket detail.
type CPU struct {
	//+optional
	TotalCores int32 `json:"totalCores,omitempty"`
	//+optional
	TotalThreads int32 `json:"totalThreads,omitempty"`
	// Sockets lists the individual physical CPUs.
	//+optional
	Sockets []CPUSocket `json:"sockets,omitempty"`
}

// CPUSocket describes a single physical CPU. Note: on the out-of-band Redfish
// collection path, SerialNumber is always empty — that is what the BMC exposes,
// not a mapping gap.
type CPUSocket struct {
	// Slot identifies the physical socket (e.g. "CPU1").
	//+optional
	Slot string `json:"slot,omitempty"`
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	Cores int32 `json:"cores,omitempty"`
	//+optional
	Threads int32 `json:"threads,omitempty"`
	//+optional
	ClockSpeedMHz int32 `json:"clockSpeedMHz,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	// Capabilities lists CPU feature flags. In-band only.
	//+optional
	Capabilities []string `json:"capabilities,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
}

// Memory holds aggregate memory totals plus per-module detail.
type Memory struct {
	// TotalBytes is the total physical memory installed.
	//+optional
	TotalBytes int64 `json:"totalBytes,omitempty"`
	// UsableBytes is the memory usable by the OS. In-band only.
	//+optional
	UsableBytes int64 `json:"usableBytes,omitempty"`
	// Modules lists the individual memory modules. Out-of-band only: the physical
	// DIMM topology is not visible from inside the OS.
	//+optional
	Modules []MemoryModule `json:"modules,omitempty"`
}

// MemoryModule describes a single memory module. Note: on the out-of-band Redfish
// collection path, Model is always empty — that is what the BMC exposes.
type MemoryModule struct {
	// Slot identifies the physical slot (e.g. "DIMM.A1").
	//+optional
	Slot string `json:"slot,omitempty"`
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	PartNumber string `json:"partNumber,omitempty"`
	//+optional
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	//+optional
	SpeedMHz int32 `json:"speedMHz,omitempty"`
	//+optional
	FormFactor string `json:"formFactor,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
}

// BlockDevice describes a storage drive.
type BlockDevice struct {
	// Name is the OS-visible device name (e.g. "nvme0n1"). In-band only.
	//+optional
	Name string `json:"name,omitempty"`
	//+optional
	ControllerType string `json:"controllerType,omitempty"`
	//+optional
	DriveType string `json:"driveType,omitempty"`
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	// WWN is the World Wide Name, a standard unique storage identifier. Where both
	// sources report a drive, this is the key they can be joined on.
	//+optional
	WWN string `json:"wwn,omitempty"`
	// Path is the device's position in the topology (udev ID_PATH, e.g.
	// "pci-0000:17:00.0-nvme-1"), i.e. /dev/disk/by-path/<path>. Stable where Name
	// is not, and identifies the slot rather than the drive: WWN and SerialNumber
	// identify the drive, Path the port it is attached to. In-band only.
	//+optional
	Path string `json:"path,omitempty"`
	//+optional
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	//+optional
	PhysicalBlockSizeBytes int64 `json:"physicalBlockSizeBytes,omitempty"`
	// SmartStatus is the drive's self-reported SMART health status (e.g. "ok",
	// "predict-failure"). Out-of-band only.
	//+optional
	SmartStatus string `json:"smartStatus,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	//+optional
	Status *ComponentStatus `json:"status,omitempty"`
}

// AttributeNetworkInterface describes a network adapter and its ports.
type AttributeNetworkInterface struct {
	// Name is the OS-visible interface name (e.g. "eno1"). In-band only.
	//+optional
	Name string `json:"name,omitempty"`
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	// Ports lists the physical ports on this adapter. The in-band collector reports
	// one port per interface; the out-of-band collector groups ports under the
	// adapter that carries them.
	//+optional
	Ports []NetworkPort `json:"ports,omitempty"`
}

// NetworkPort describes a single physical port on a network adapter.
type NetworkPort struct {
	// PortID identifies the port as the BMC names it (e.g. "NIC.Embedded.1").
	// Out-of-band only.
	//+optional
	PortID string `json:"portID,omitempty"`
	// MAC is the port's hardware address. Where both sources report a port, this is
	// the key they can be joined on.
	//+optional
	MAC string `json:"mac,omitempty"`
	//+optional
	SpeedMbps int32 `json:"speedMbps,omitempty"`
	//+optional
	MTU int32 `json:"mtu,omitempty"`
	//+optional
	LinkStatus string `json:"linkStatus,omitempty"`
	// EnabledCapabilities lists enabled offloads and features. In-band only.
	//+optional
	EnabledCapabilities []string `json:"enabledCapabilities,omitempty"`

	// LLDPNeighbors lists every neighbor advertising to this port over LLDP; a
	// port can see more than one (e.g. a switch plus a hypervisor vSwitch or a
	// daisy-chained device). In-band only: this comes from passively listening
	// on the wire, not from the BMC.
	//+optional
	LLDPNeighbors []LLDPNeighbor `json:"lldpNeighbors,omitempty"`
}

// LLDPNeighbor describes the switch (or other LLDP-speaking device) discovered
// on the other end of a NetworkPort.
type LLDPNeighbor struct {
	// ChassisID identifies the neighbor's chassis, format depending on what the
	// neighbor advertises (e.g. a MAC address).
	//+optional
	ChassisID string `json:"chassisID,omitempty"`
	// SystemName is the neighbor's advertised hostname (e.g. the switch name).
	//+optional
	SystemName string `json:"systemName,omitempty"`
	// PortID identifies the neighbor's port that this port is connected to, as
	// the neighbor names it (e.g. a switch port name).
	//+optional
	PortID string `json:"portID,omitempty"`
	// PortDescription is the neighbor's description of that port (e.g. an
	// interface alias configured on the switch).
	//+optional
	PortDescription string `json:"portDescription,omitempty"`
}

// GPUDevice describes a GPU or accelerator device.
type GPUDevice struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Description string `json:"description,omitempty"`
	// Class is the PCI device class. In-band only.
	//+optional
	Class string `json:"class,omitempty"`
	// Driver is the loaded kernel driver. In-band only.
	//+optional
	Driver string `json:"driver,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	//+optional
	Status *ComponentStatus `json:"status,omitempty"`
}

// PCIDevice describes a device on the PCI bus. In-band only.
type PCIDevice struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	Class string `json:"class,omitempty"`
	//+optional
	Driver string `json:"driver,omitempty"`
}

// Chassis describes the enclosure.
type Chassis struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
}

// Baseboard describes the mainboard/motherboard.
type Baseboard struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Description string `json:"description,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	//+optional
	Status *ComponentStatus `json:"status,omitempty"`
}

// BIOS describes the system BIOS firmware.
type BIOS struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	// ReleaseDate is the BIOS release date as the source reports it, format
	// unspecified (e.g. "12/13/2021"). In-band only.
	//+optional
	ReleaseDate string `json:"releaseDate,omitempty"`
	//+optional
	Status *ComponentStatus `json:"status,omitempty"`
}

// Product describes the overall system identity.
type Product struct {
	//+optional
	Name string `json:"name,omitempty"`
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Status *ComponentStatus `json:"status,omitempty"`
}

// AttributeBMC describes the BMC's own firmware and out-of-band management NIC.
// Out-of-band only.
type AttributeBMC struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	// NIC is the BMC's own management network interface. It is distinct from
	// Attributes.NetworkInterfaces, which lists the host's NICs.
	//+optional
	NIC *AttributeNetworkInterface `json:"nic,omitempty"`
	//+optional
	Status *ComponentStatus `json:"status,omitempty"`
}

// PSU describes a power supply unit. Out-of-band only.
type PSU struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Description string `json:"description,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	//+optional
	PowerCapacityWatts int64 `json:"powerCapacityWatts,omitempty"`
	//+optional
	Status *ComponentStatus `json:"status,omitempty"`
}

// AttributeTPM describes a trusted platform module. Out-of-band only.
type AttributeTPM struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	// InterfaceType is the TPM interface (e.g. "TPM2_0").
	//+optional
	InterfaceType string `json:"interfaceType,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	//+optional
	Status *ComponentStatus `json:"status,omitempty"`
}

// StorageController describes a storage controller. Out-of-band only.
type StorageController struct {
	//+optional
	Vendor string `json:"vendor,omitempty"`
	//+optional
	Model string `json:"model,omitempty"`
	//+optional
	SerialNumber string `json:"serialNumber,omitempty"`
	//+optional
	Description string `json:"description,omitempty"`
	//+optional
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	//+optional
	Status *ComponentStatus `json:"status,omitempty"`
}

// ComponentStatus is health/state information reported for an individual
// component. PostCode and PostCodeStatus are device-level POST diagnostics and
// are only populated on Product (the machine's own identity), never on a
// per-component status.
type ComponentStatus struct {
	//+optional
	Health string `json:"health,omitempty"`
	//+optional
	State string `json:"state,omitempty"`
	// PostCode is a pointer because 0 is a meaningful POST code (success), which
	// omitempty on a value type would drop.
	//+optional
	PostCode *int32 `json:"postCode,omitempty"`
	//+optional
	PostCodeStatus string `json:"postCodeStatus,omitempty"`
}
