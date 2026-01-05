package bmc

const (
	PowerActionOn      PowerAction = "on"
	PowerActionHardOff PowerAction = "off"
	PowerActionSoftOff PowerAction = "soft"
	PowerActionCycle   PowerAction = "cycle"
	PowerActionReset   PowerAction = "reset"
	PowerActionStatus  PowerAction = "status"

	BootDevicePXE   BootDevice = "pxe"
	BootDeviceDisk  BootDevice = "disk"
	BootDeviceBIOS  BootDevice = "bios"
	BootDeviceCDROM BootDevice = "cdrom"
	BootDeviceSafe  BootDevice = "safe"

	// VirtualMediaCD represents a virtual CD-ROM.
	VirtualMediaCD VirtualMediaKind = "CD"
)

// BootDevice represents boot device of the Machine.
type BootDevice string

// VirtualMediaKind represents the kind of virtual media.
type VirtualMediaKind string

// PowerAction represents the power control operation on the baseboard management.
// +kubebuilder:validation:Enum=on;off;soft;status;cycle;reset
type PowerAction string

// VirtualMediaAction represents a virtual media action.
type VirtualMediaAction struct {
	// mediaURL represents the URL of the image to be inserted into the virtual media, or empty to eject media.
	MediaURL string `json:"mediaURL,omitempty"`

	// Kind represents the kind of virtual media.
	Kind VirtualMediaKind `json:"kind"`
}

// BootDeviceConfig represents the configuration for setting a boot device.
type BootDeviceConfig struct {
	// Device is the name of the device to set as the first boot device.
	Device BootDevice `json:"device,omitempty"`

	// Persistent indicates whether the boot device should be set persistently as the first boot device.
	Persistent bool `json:"persistent,omitempty"`

	// EFIBoot indicates whether the boot device should be set to efiboot mode.
	EFIBoot bool `json:"efiBoot,omitempty"`
}

func (b BootDevice) String() string {
	return string(b)
}

func (v VirtualMediaKind) String() string {
	return string(v)
}

func (p PowerAction) String() string {
	return string(p)
}

// Operations defines the operations that can be performed against a BMC.
// Only a single field in the struct should be defined at any given time
// as only one operation can be performed at a time.
// For example either PowerAction or BootDevice.
// +kubebuilder:validation:XValidation:rule="(has(self.powerAction) ? 1 : 0) + (has(self.bootDevice) ? 1 : 0) + (has(self.virtualMediaAction) ? 1 : 0) == 1",message="only one of powerAction, bootDevice, or virtualMediaAction can be specified"
type Operations struct {
	// PowerAction represents a baseboard management power operation.
	// +kubebuilder:validation:Enum=on;off;soft;status;cycle;reset
	PowerAction *PowerAction `json:"powerAction,omitempty"`

	// BootDevice is the device to set as the first boot device on the Machine.
	BootDevice *BootDeviceConfig `json:"bootDevice,omitempty"`

	// VirtualMediaAction represents a baseboard management virtual media insert/eject.
	VirtualMediaAction *VirtualMediaAction `json:"virtualMediaAction,omitempty"`
}
