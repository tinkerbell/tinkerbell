package constant

const (
	// MacAddrFormatColon is a MAC address format with colon delimiters between pairs of characters.
	MacAddrFormatColon MACFormat = "colon"
	// MacAddrFormatDot is a MAC address format with dot delimiters between groups of 4 characters.
	MacAddrFormatDot MACFormat = "dot"
	// MacAddrFormatDash is a MAC address format with dash delimiters between pairs of characters.
	MacAddrFormatDash MACFormat = "dash"
	// MacAddrFormatNoDelimiter removes all delimiters from a MAC address. Note that this is not a valid MAC address format.
	// It is useful for cases where delimiters can potentially cause issues, such as in URLs.
	MacAddrFormatNoDelimiter MACFormat = "no-delimiter"
	// MacAddrFormatEmpty converts a MAC address to an empty string. Note that this is not a valid MAC address format.
	MacAddrFormatEmpty MACFormat = "empty"

	// IPXEBinaryIPXEEFI is the Tinkerbell built and embedded iPXE binary for UEFI x86_64 architectures.
	IPXEBinaryIPXEEFI IPXEBinary = "ipxe.efi"
	// IPXEBinarySNPAMD64 is the Tinkerbell built and embedded iPXE binary for UEFI ARM64 architectures using iPXE's Simple Network Protocol (SNP).
	IPXEBinarySNPARM64 IPXEBinary = "snp-arm64.efi"
	// IPXEBinarySNPX86_64 is the Tinkerbell built and embedded iPXE binary for UEFI x86_64 architectures using iPXE's Simple Network Protocol (SNP).
	IPXEBinarySNPAMD64 IPXEBinary = "snp-x86_64.efi"
	// IPXEBinaryUndionlyKPXE is the Tinkerbell built and embedded iPXE binary for BIOS x86 architectures.
	IPXEBinaryUndionlyKPXE IPXEBinary = "undionly.kpxe"
	// IPXEBinaryISOEFIAMD64 is the Tinkerbell built and embedded iPXE binary for UEFI x86_64 architectures in ISO format.
	IPXEBinaryISOEFIAMD64 IPXEBinary = "ipxe.iso"
	// IPXEBinaryIMGEFIAMD64 is the Tinkerbell built and embedded iPXE binary for UEFI x86_64 architectures in IMG format.
	IPXEBinaryIMGEFIAMD64 IPXEBinary = "ipxe-efi.img"
)

// MACFormat is a format for a MAC address.
type MACFormat string

func (m MACFormat) String() string {
	return string(m)
}

// IPXEBinary is a type for iPXE binary names.
type IPXEBinary string

func (i IPXEBinary) String() string {
	return string(i)
}
