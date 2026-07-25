package binary

import "os"

// openAsset opens name for reading, confined to dir. Traversal outside dir
// (absolute paths, ".." segments, or symlinks pointing outside) is rejected
// by os.Root, so a client- or hardware-supplied path cannot escape the asset
// directory and read arbitrary files from the host filesystem.
//
// The returned file must be closed by the caller.
func openAsset(dir, name string) (*os.File, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	// Closing the root does not affect the file it returned; the file carries
	// its own descriptor.
	defer root.Close()

	return root.Open(name)
}
