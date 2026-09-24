package bmc

const (
	PowerOn      PowerAction = "on"
	PowerHardOff PowerAction = "off"
	PowerSoftOff PowerAction = "soft"
	PowerCycle   PowerAction = "cycle"
	PowerReset   PowerAction = "reset"
	PowerStatus  PowerAction = "status"

	PXE   BootDevice = "pxe"
	Disk  BootDevice = "disk"
	BIOS  BootDevice = "bios"
	CDROM BootDevice = "cdrom"
	Safe  BootDevice = "safe"

	// VirtualMediaCD represents a virtual CD-ROM.
	VirtualMediaCD VirtualMediaKind = "CD"
)

// BootDevice represents boot device of the Machine.
type BootDevice string

// VirtualMediaKind represents the kind of virtual media.
type VirtualMediaKind string

// PowerAction represents the power control operation on the baseboard management.
type PowerAction string

// OneTimeBootDeviceAction represents a single operation to set the machine's one-time boot device via the BMC.
// Deprecated. Will be removed in a future release. Use BootDeviceConfig instead.
type OneTimeBootDeviceAction struct {
	// Devices represents the boot devices, in order for setting one time boot.
	// Currently only the first device in the slice is used to set one time boot.
	Devices []BootDevice `json:"device"`

	// EFIBoot instructs the machine to use EFI boot.
	EFIBoot bool `json:"efiBoot,omitempty"`
}

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

// NetworkBootConfig represents the configuration for enabling/disabling network boot protocol
// capabilities (UEFI HTTP Boot, legacy PXE) in BIOS/UEFI firmware, and for pointing UEFI HTTP
// Boot at a boot image URL. Unlike BootDevice (which selects among existing boot options), this
// creates or removes the boot options themselves. HTTPBootEnabled, PXEBootEnabled, HTTPBootURL,
// and HTTPBootTLSMode are independent — any combination may be set at once; a nil field leaves
// that setting untouched.
// +kubebuilder:validation:MinProperties:=1
type NetworkBootConfig struct {
	// HTTPBootEnabled enables (true) or disables (false) UEFI HTTP Boot capability, IPv4 and
	// IPv6 both.
	HTTPBootEnabled *bool `json:"httpBootEnabled,omitempty"`

	// PXEBootEnabled enables (true) or disables (false) legacy PXE boot capability.
	PXEBootEnabled *bool `json:"pxeBootEnabled,omitempty"`

	// HTTPBootURL sets the URL UEFI HTTP Boot fetches its boot image from, via the standard
	// Redfish ComputerSystem.Boot.HttpBootUri property. Independent of HTTPBootEnabled: setting
	// the URL does not enable the capability, and enabling the capability does not require a URL.
	// +kubebuilder:validation:Format=uri
	HTTPBootURL *string `json:"httpBootURL,omitempty"`

	// HTTPBootTLSMode sets the TLS authentication mode UEFI HTTP Boot uses to connect to the HTTP
	// boot server: "None" allows fetching the boot image over plain HTTP, "OneWay" requires HTTPS
	// with the server authenticated by the client. Some hardware defaults to "OneWay", which
	// rejects an HTTPBootURL that isn't HTTPS. Independent of HTTPBootEnabled and HTTPBootURL.
	// +kubebuilder:validation:Enum=None;OneWay
	HTTPBootTLSMode *string `json:"httpBootTLSMode,omitempty"`
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
