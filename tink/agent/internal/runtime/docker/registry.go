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

	// Check if the first part looks like a registry hostname
	// We need to distinguish between:
	// - Registry hostnames (registry.example.com, localhost:5000, 192.168.1.1:5000, [::1]:5000)
	// - Docker Hub images with tags (ubuntu:20.04, myapp:v1.2.3)
	// - Docker Hub usernames (username/image)
	if isRegistryHostname(firstPart) {
		return firstPart
	}

	// If the first part doesn't look like a hostname, assume it's Docker Hub
	// Examples: "library/ubuntu", "username/image", "ubuntu:20.04"
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

// isRegistryHostname determines if a string represents a registry hostname rather than
// a Docker Hub image name with tag. This function handles various edge cases:
// - IPv6 addresses in brackets: [::1]:5000, [2001:db8::1]:5000
// - IPv4 addresses with ports: 192.168.1.1:5000
// - Hostnames with ports: registry.example.com:5000, localhost:5000
// - Hostnames with dots: registry.example.com, sub.domain.com
// - Known registry patterns: localhost, 127.0.0.1
// - Excludes Docker Hub image:tag patterns: ubuntu:20.04, myapp:v1.2.3
func isRegistryHostname(part string) bool {
	if part == "" {
		return false
	}

	// Handle IPv6 addresses in brackets [::1]:5000 or [2001:db8::1]:5000
	if strings.HasPrefix(part, "[") && strings.Contains(part, "]:") {
		return true
	}

	// Check for localhost (with or without port)
	if part == "localhost" || strings.HasPrefix(part, "localhost:") {
		return true
	}

	// Check for IP addresses (IPv4) with optional port
	if isIPv4WithOptionalPort(part) {
		return true
	}

	// Check if it contains a dot (indicating a domain)
	if strings.Contains(part, ".") {
		// Make sure it's not just a single dot or other invalid patterns
		if part == "." || part == ".." || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".") {
			return false
		}

		// Additional check: if it contains a colon, make sure it's likely a port, not a tag
		if strings.Contains(part, ":") {
			return isHostnameWithPort(part)
		}

		// Basic validation: should have at least one character before and after dot
		dotParts := strings.Split(part, ".")
		for _, dotPart := range dotParts {
			if len(dotPart) == 0 {
				return false
			}
		}

		return true
	}

	// If it contains a colon but no dot, it could be:
	// 1. A hostname with port (localhost:5000) - already handled above
	// 2. A Docker image with tag (ubuntu:20.04) - should return false
	// 3. An IPv4 address with port (1.2.3.4:5000) - already handled above
	// At this point, assume it's a Docker image with tag
	return false
}

// isIPv4WithOptionalPort checks if the string is an IPv4 address with optional port
func isIPv4WithOptionalPort(part string) bool {
	// Split by colon to separate potential IP and port
	host := part
	if colonIndex := strings.LastIndex(part, ":"); colonIndex != -1 {
		host = part[:colonIndex]
		portStr := part[colonIndex+1:]
		// Basic port validation (1-65535)
		if portStr == "" || len(portStr) > 5 {
			return false
		}
		// Check if port contains only digits
		for _, r := range portStr {
			if r < '0' || r > '9' {
				return false
			}
		}
	}

	// Basic IPv4 validation: check for pattern like x.x.x.x
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}

	for _, octet := range parts {
		if octet == "" || len(octet) > 3 {
			return false
		}
		// Check if octet contains only digits
		for _, r := range octet {
			if r < '0' || r > '9' {
				return false
			}
		}
	}

	return true
}

// isHostnameWithPort checks if a string with both dots and colons represents
// a hostname with port rather than a Docker image with tag
func isHostnameWithPort(part string) bool {
	// Find the last colon (potential port separator)
	colonIndex := strings.LastIndex(part, ":")
	if colonIndex == -1 {
		return true // No colon, just a hostname with dots
	}

	portStr := part[colonIndex+1:]
	hostname := part[:colonIndex]

	// Port should be numeric and reasonable length
	if len(portStr) == 0 || len(portStr) > 5 {
		return false
	}

	// Check if port contains only digits
	for _, r := range portStr {
		if r < '0' || r > '9' {
			return false
		}
	}

	// Hostname part should still contain dots for this to be a registry hostname
	return strings.Contains(hostname, ".")
}
