// Package binary handles embedding of the iPXE binaries.
package binary

// embed lib does the work of embedding the on disk iPXE binaries.
import _ "embed"

// IpxeEFI is the UEFI iPXE binary for x86 architectures.
//go:embed ipxe.efi
var IpxeEFI []byte

// Undionly is the BIOS iPXE binary for x86 architectures.
//go:embed undionly.kpxe
var Undionly []byte

// SNP is the UEFI iPXE binary for ARM architectures.
//go:embed snp.efi
var SNP []byte

// Files is the mapping to the embedded iPXE binaries.
var Files = map[string][]byte{
	"undionly.kpxe": Undionly,
	"ipxe.efi":      IpxeEFI,
	"snp.efi":       SNP,
}
