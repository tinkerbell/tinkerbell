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

// SecureBootAction represents enabling or disabling UEFI Secure Boot.
type SecureBootAction struct {
	// Enable indicates whether Secure Boot should be enabled or disabled.
	Enable bool `json:"enable"`
}

// SecureBootResetKeysAction represents resetting the UEFI Secure Boot key databases.
type SecureBootResetKeysAction struct {
	// ResetType is the type of key reset to perform.
	// +kubebuilder:validation:Enum=ResetAllKeysToDefault;DeleteAllKeys;DeletePK
	ResetType string `json:"resetType"`
}

// SecureBootDatabaseResetKeysAction represents resetting a single UEFI Secure Boot key database,
// as opposed to SecureBootResetKeysAction which resets the whole subsystem (including PK).
type SecureBootDatabaseResetKeysAction struct {
	// Database is the UEFI Secure Boot key database to reset (e.g. "db", "KEK").
	Database string `json:"database"`

	// ResetType is the type of key reset to perform. PK is not a valid target at the
	// per-database granularity, so DeletePK is not offered here (see SecureBootResetKeysAction).
	// +kubebuilder:validation:Enum=ResetAllKeysToDefault;DeleteAllKeys
	ResetType string `json:"resetType"`
}

// ConfigMapKeyReference references a specific key within a ConfigMap.
type ConfigMapKeyReference struct {
	// Name of the ConfigMap.
	Name string `json:"name"`

	// Namespace the ConfigMap resides in.
	Namespace string `json:"namespace"`

	// Key within the ConfigMap's data.
	Key string `json:"key"`
}

// SecureBootCertificateImportAction represents importing a certificate into a single UEFI
// Secure Boot key database, without touching any other database (e.g. leaving PK, and
// therefore Setup Mode, untouched).
type SecureBootCertificateImportAction struct {
	// Database is the UEFI Secure Boot key database to import the certificate into (e.g. "db", "KEK").
	Database string `json:"database"`

	// CertificatePEMConfigMapRef references the ConfigMap key holding the PEM-encoded
	// certificate to import - a certificate isn't secret, but is typically too large and
	// too reused-across-Workflows to inline directly into an Action.
	CertificatePEMConfigMapRef ConfigMapKeyReference `json:"certificatePEMConfigMapRef"`
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
