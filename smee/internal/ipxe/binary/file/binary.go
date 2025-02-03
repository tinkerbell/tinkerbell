// Package binary handles embedding of the iPXE binaries.
package binary

// embed lib does the work of embedding the on disk iPXE binaries.
import (
	"bytes"
	_ "embed"
	"errors"
)

// IpxeEFI is the UEFI iPXE binary for x86 architectures.
//
//go:embed ipxe.efi
var IpxeEFI []byte

// Undionly is the BIOS iPXE binary for x86 architectures.
//
//go:embed undionly.kpxe
var Undionly []byte

// SNP is the UEFI iPXE binary for ARM architectures.
//
//go:embed snp.efi
var SNP []byte

// IpxeISO is the iPXE ISO image.
//
//go:embed ipxe.iso
var IpxeISO []byte

// MagicString is included in each iPXE binary within the embedded script. It
// can be overwritten to change the behavior at startup.
var magicString = []byte(`#a8b7e61f1075c37a793f2f92cee89f7bba00c4a8d7842ce3d40b5889032d8881
#ddd16a4fc4926ecefdfb6941e33c44ed3647133638f5e84021ea44d3152e7f97`)

var magicStringPadding = bytes.Repeat([]byte{' '}, len(magicString))

// Files is the mapping to the embedded iPXE binaries.
var Files = map[string][]byte{
	"undionly.kpxe": Undionly,
	"ipxe.efi":      IpxeEFI,
	"snp.efi":       SNP,
	"ipxe.iso":      IpxeISO,
}

var ErrPatchTooLong = errors.New("patch string is too long")

// Replace the magic string in the content with the patch. Returns the original content
// when the patch is empty or the magic string is not found, and returns an error when
// the patch is too long.
func Patch(content, patch []byte) ([]byte, error) {
	// Noop when no patch is passed.
	if len(patch) == 0 {
		return content, nil
	}

	// Also noop when there's no magic patch string available in the content.
	i := bytes.Index(content, magicString)
	if i == -1 {
		return content, nil
	}

	if len(patch) > len(magicString) {
		return nil, ErrPatchTooLong
	}

	// Duplicate the content before applying the patch so we don't overwrite
	// the underlying array.
	dup := make([]byte, len(content))
	copy(dup, content)
	copy(dup[i:], magicStringPadding)
	copy(dup[i:], patch)

	return dup, nil
}
