package tftp

import "errors"

// Common TFTP errors
var (
	ErrNotFound = errors.New("file not found")
)
