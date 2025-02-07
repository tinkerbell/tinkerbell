package v1alpha1

// PowerAction represents the power control operation on the baseboard management.
type PowerAction string

const (
	PowerOn      PowerAction = "on"
	PowerHardOff PowerAction = "off"
	PowerSoftOff PowerAction = "soft"
	PowerCycle   PowerAction = "cycle"
	PowerReset   PowerAction = "reset"
	PowerStatus  PowerAction = "status"
)

// Pointer provides an easy way to retrieve the power action as a pointer for use in job
// tasks.
func (p PowerAction) Ptr() *PowerAction {
	return &p
}

// BootDevice represents boot device of the Machine.
type BootDevice string

const (
	PXE   BootDevice = "pxe"
	Disk  BootDevice = "disk"
	BIOS  BootDevice = "bios"
	CDROM BootDevice = "cdrom"
	Safe  BootDevice = "safe"
)

// OnTimeBootDeviceAction represents a baseboard management one time set boot device operation.
type OneTimeBootDeviceAction struct {
	// Devices represents the boot devices, in order for setting one time boot.
	// Currently only the first device in the slice is used to set one time boot.
	Devices []BootDevice `json:"device"`

	// EFIBoot instructs the machine to use EFI boot.
	EFIBoot bool `json:"efiBoot,omitempty"`
}

type VirtualMediaKind string

const (
	// VirtualMediaCD represents a virtual CD-ROM.
	VirtualMediaCD VirtualMediaKind = "CD"
)

// VirtualMediaAction represents a virtual media action.
type VirtualMediaAction struct {
	// mediaURL represents the URL of the image to be inserted into the virtual media, or empty to
	// eject media.
	MediaURL string `json:"mediaURL,omitempty"`

	Kind VirtualMediaKind `json:"kind"`
}
