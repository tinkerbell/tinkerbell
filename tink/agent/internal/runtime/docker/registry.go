package docker

import (
	"net/url"
	"strings"
)

// shouldUseAuth determines if authentication should be used for pulling the given image.
// It compares the registry hostname extracted from the image reference against the
// configured registry hostname to ensure exact matching and prevent security vulnerabilities
// from substring matching attacks.
func shouldUseAuth(imageRef, registryHost string) bool {
	if registryHost == "" {
		return false
	}

	imageHost := extractRegistryHostname(imageRef)
	configHost := normalizeRegistryHostname(registryHost)

	return imageHost == configHost
}

// extractRegistryHostname extracts the registry hostname from an image reference.
// Examples:
//   - "registry.example.com/namespace/image:tag" -> "registry.example.com"
//   - "registry.example.com:5000/image" -> "registry.example.com:5000"
//   - "localhost:5000/image" -> "localhost:5000"
//   - "image" -> "docker.io" (Docker Hub default)
//   - "ubuntu:20.04" -> "docker.io" (Docker Hub default)
func extractRegistryHostname(imageRef string) string {
	if imageRef == "" {
		return ""
	}

	// Split the image reference by '/' to get the potential registry part
	parts := strings.Split(imageRef, "/")
	if len(parts) == 1 {
		// Single part means it's a Docker Hub image (e.g., "ubuntu", "ubuntu:20.04")
		return "docker.io"
	}

	// The first part might be the registry hostname
	firstPart := parts[0]

	// Check if the first part looks like a hostname (contains '.' or ':')
	// This helps distinguish between registry hostnames and simple usernames
	if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
		return firstPart
	}

	// If the first part doesn't look like a hostname, assume it's Docker Hub
	// Examples: "library/ubuntu", "username/image"
	return "docker.io"
}

// normalizeRegistryHostname normalizes a registry hostname for comparison.
// It handles various formats that might be provided in configuration.
// Examples:
//   - "https://registry.example.com" -> "registry.example.com"
//   - "http://localhost:5000" -> "localhost:5000"
//   - "registry.example.com:443" -> "registry.example.com:443"
//   - "registry.example.com" -> "registry.example.com"
func normalizeRegistryHostname(registryHost string) string {
	if registryHost == "" {
		return ""
	}

	// Handle URL schemes (https:// or http://)
	if strings.HasPrefix(registryHost, "https://") || strings.HasPrefix(registryHost, "http://") {
		parsed, err := url.Parse(registryHost)
		if err != nil {
			// If parsing fails, strip the scheme manually
			registryHost = strings.TrimPrefix(registryHost, "https://")
			registryHost = strings.TrimPrefix(registryHost, "http://")
		} else {
			registryHost = parsed.Host
		}
	}

	// Remove any trailing path components
	if idx := strings.Index(registryHost, "/"); idx != -1 {
		registryHost = registryHost[:idx]
	}

	return registryHost
}
