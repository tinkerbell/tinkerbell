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
