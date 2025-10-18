package docker

import (
	"github.com/distribution/reference"
	"golang.org/x/text/unicode/norm"
)

// useAuth determines if authentication should be used for pulling the given image.
// It compares the registry hostname extracted from the image reference against the
// configured registry hostname to ensure exact matching and prevent security vulnerabilities
// from substring matching attacks and homograph attacks using Unicode normalization.
func useAuth(imageRef, registryHost string) bool {
	if registryHost == "" {
		return false
	}

	pnn, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return false
	}
	imageHost := reference.Domain(pnn)

	// Apply Unicode normalization to prevent homograph attacks
	// Use NFC (Canonical Decomposition followed by Canonical Composition)
	// to ensure consistent Unicode representation
	imageH := norm.NFC.String(imageHost)
	registryH := norm.NFC.String(registryHost)

	return imageH == registryH
}
