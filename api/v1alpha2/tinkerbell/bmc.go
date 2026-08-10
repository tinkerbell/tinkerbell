package tinkerbell

import "github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell/bmc"

// FirmwareMode is the firmware boot interface of the Hardware.
// +kubebuilder:validation:Enum=UEFI;BIOS
type FirmwareMode string

const (
	// FirmwareModeUEFI indicates the Hardware boots using UEFI firmware.
	FirmwareModeUEFI FirmwareMode = "UEFI"

	// FirmwareModeBIOS indicates the Hardware boots using legacy BIOS firmware.
	FirmwareModeBIOS FirmwareMode = "BIOS"
)

// BMC contains connection and configuration data for a BMC (Baseboard Management Controller).
type BMC struct {
	// BootMode specifies the firmware boot mode of the Hardware.
	// One of UEFI or BIOS.
	// +kubebuilder:default=UEFI
	// +optional
	BootMode FirmwareMode `json:"bootMode,omitempty"`

	// Connection contains connection data for a Baseboard Management Controller.
	Connection *bmc.Connection `json:"connection,omitempty"`
}
