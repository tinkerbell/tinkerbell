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
	BootDeviceUSB   BootDevice = "usb"
	// BootDeviceUSBCDROM is the BMC's virtual CD as a USB CD: Redfish UsbCd, IPMI boot device selector 8.
	BootDeviceUSBCDROM BootDevice = "usb_cdrom"

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
// By default, the boot device is set for the next boot only. If Persistent is set to true, the boot device is set persistently as the first boot device.
type BootDeviceConfig struct {
	// Name is the name of the device to set as the first boot device: a bmclib boot device, such as pxe,
	// disk, cdrom, usb, usb_cdrom, uefi_http or bios. To boot virtual media, use the device the BMC
	// presents it as, for example usb_cdrom on some Supermicro, usb on NVIDIA's OpenBMC (GB200) and cdrom
	// where it is an optical device.
	Name BootDevice `json:"name,omitempty"`

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
	// +kubebuilder:validation:XValidation:rule="self.startsWith('http://') || self.startsWith('https://')",message="httpBootURL must be an http or https URL"
	HTTPBootURL *string `json:"httpBootURL,omitempty"`

	// HTTPBootTLSMode sets the TLS authentication mode UEFI HTTP Boot uses to connect to the HTTP
	// boot server: "None" allows fetching the boot image over plain HTTP, "OneWay" requires HTTPS
	// with the server authenticated by the client. Some hardware defaults to "OneWay", which
	// rejects an HTTPBootURL that isn't HTTPS. Independent of HTTPBootEnabled and HTTPBootURL.
	// +kubebuilder:validation:Enum=None;OneWay
	HTTPBootTLSMode *string `json:"httpBootTLSMode,omitempty"`
}

// SecureBootAction represents a UEFI Secure Boot operation. Exactly one field must be set; to
// run several operations, use one Action per operation. Operations are sent to the BMC in the
// order the Actions are listed, but when the BMC applies them (typically on the next reboot) is
// up to the BMC.
// +kubebuilder:validation:MinProperties:=1
// +kubebuilder:validation:MaxProperties:=1
type SecureBootAction struct {
	// Enable enables (true) or disables (false) UEFI Secure Boot.
	Enable *bool `json:"enable,omitempty"`

	// AllowCustomKeys enables (true) or disables (false) the platform's out-of-band acceptance
	// of custom UEFI Secure Boot keys. This is distinct from enabling Secure Boot itself and
	// from the standardized Setup/Audit/User/Deployed platform mode: it only gates whether a key
	// store change made through the BMC will be accepted. Not every BMC supports it; the Task
	// fails with the provider's error if the BMC doesn't.
	AllowCustomKeys *bool `json:"allowCustomKeys,omitempty"`

	// ResetKeys resets the Secure Boot key databases as a whole, PK included, as opposed to
	// DatabaseResetKeys which targets a single database other than PK.
	ResetKeys *SecureBootResetKeysAction `json:"resetKeys,omitempty"`

	// DatabaseResetKeys resets a single Secure Boot key database.
	DatabaseResetKeys *SecureBootDatabaseResetKeysAction `json:"databaseResetKeys,omitempty"`

	// CertificateImport imports a certificate into a single Secure Boot key database.
	CertificateImport *SecureBootCertificateImportAction `json:"certificateImport,omitempty"`
}

// SecureBootResetKeysAction represents resetting the UEFI Secure Boot key databases.
type SecureBootResetKeysAction struct {
	// ResetType is the type of key reset to perform: ResetAllKeysToDefault restores every
	// database to its platform default, DeleteAllKeys empties every database including PK, and
	// DeletePK empties only PK. DeleteAllKeys and DeletePK both put the firmware in Setup Mode.
	// +kubebuilder:validation:Enum=ResetAllKeysToDefault;DeleteAllKeys;DeletePK
	ResetType string `json:"resetType"`
}

// SecureBootDatabaseResetKeysAction represents resetting a single UEFI Secure Boot key database
// other than PK, as opposed to SecureBootResetKeysAction which resets the key databases as a
// whole.
type SecureBootDatabaseResetKeysAction struct {
	// Database is the UEFI Secure Boot key database to reset. PK is not offered; use the
	// secureBoot resetKeys operation with DeletePK to clear it.
	// +kubebuilder:validation:Enum=db;KEK;dbx
	Database string `json:"database"`

	// ResetType is the type of key reset to perform: ResetAllKeysToDefault restores the
	// database to its platform default, DeleteAllKeys empties it.
	// +kubebuilder:validation:Enum=ResetAllKeysToDefault;DeleteAllKeys
	ResetType string `json:"resetType"`
}

// ConfigMapKeyReference references a specific key within a ConfigMap in the same namespace as
// the object that holds the reference.
type ConfigMapKeyReference struct {
	// Name of the ConfigMap.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key within the ConfigMap's data.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// SecureBootCertificateImportAction represents importing a certificate into a single UEFI
// Secure Boot key database, without touching any other database (e.g. leaving PK, and
// therefore Setup Mode, untouched).
type SecureBootCertificateImportAction struct {
	// Database is the UEFI Secure Boot key database to import the certificate into.
	// +kubebuilder:validation:Enum=PK;KEK;db;dbx
	Database string `json:"database"`

	// CertificatePEMConfigMapRef references the key of a ConfigMap, in the namespace of the
	// Workflow, Job or Task, that holds the PEM-encoded certificate to import. A certificate
	// isn't secret, but is typically too large and too reused across Workflows to inline
	// directly into an Action.
	CertificatePEMConfigMapRef ConfigMapKeyReference `json:"certificatePEMConfigMapRef"`
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
// +kubebuilder:validation:XValidation:rule="(has(self.powerAction) ? 1 : 0) + (has(self.bootDevice) ? 1 : 0) + (has(self.virtualMediaAction) ? 1 : 0) + (has(self.networkBootConfig) ? 1 : 0) + (has(self.secureBoot) ? 1 : 0) == 1",message="exactly one of powerAction, bootDevice, virtualMediaAction, networkBootConfig, or secureBoot must be specified"
type Operations struct {
	// PowerAction represents a baseboard management power operation.
	// +kubebuilder:validation:Enum=on;off;soft;status;cycle;reset
	PowerAction *PowerAction `json:"powerAction,omitempty"`

	// BootDevice is the device to set as the first boot device on the Machine.
	BootDevice *BootDeviceConfig `json:"bootDevice,omitempty"`

	// VirtualMediaAction represents a baseboard management virtual media insert/eject.
	VirtualMediaAction *VirtualMediaAction `json:"virtualMediaAction,omitempty"`

	// NetworkBootConfig enables/disables network boot protocol capabilities (UEFI HTTP Boot,
	// legacy PXE) in BIOS/UEFI firmware, and/or sets the URL UEFI HTTP Boot fetches its boot
	// image from. Support for the specific BMC vendor is implemented in bmclib.
	NetworkBootConfig *NetworkBootConfig `json:"networkBootConfig,omitempty"`

	// SecureBoot represents a UEFI Secure Boot operation: enabling or disabling Secure Boot,
	// resetting its key databases, or importing a certificate into one.
	SecureBoot *SecureBootAction `json:"secureBoot,omitempty"`

	// TimeoutSeconds defines the maximum time in seconds the Operation is allowed to run.
	// If not defined, the default from the runtime is used.
	TimeoutSeconds int64 `json:"timeoutSeconds,omitempty"`
}
