package docker

import (
	"testing"
)

func TestShouldUseAuth(t *testing.T) {
	tests := map[string]struct {
		imageRef     string
		registryHost string
		expected     bool
		description  string
	}{
		// Positive cases - should use auth
		"exact match": {
			imageRef:     "registry.example.com/namespace/image:tag",
			registryHost: "registry.example.com",
			expected:     true,
			description:  "Exact hostname match should use auth",
		},
		"exact match with port": {
			imageRef:     "registry.example.com:5000/image:tag",
			registryHost: "registry.example.com:5000",
			expected:     true,
			description:  "Exact hostname and port match should use auth",
		},
		"localhost with port": {
			imageRef:     "localhost:5000/image:tag",
			registryHost: "localhost:5000",
			expected:     true,
			description:  "Localhost with port should match exactly",
		},
		"docker hub image with configured host": {
			imageRef:     "ubuntu:20.04",
			registryHost: "docker.io",
			expected:     true,
			description:  "Docker Hub image should match when explicitly configured",
		},
		"docker hub image with namespace": {
			imageRef:     "library/ubuntu:20.04",
			registryHost: "docker.io",
			expected:     true,
			description:  "Docker Hub image with namespace should match when configured",
		},
		"docker hub user image": {
			imageRef:     "username/image:tag",
			registryHost: "docker.io",
			expected:     true,
			description:  "Docker Hub user image should match when configured",
		},
		"ip address registry": {
			imageRef:     "192.168.1.100:5000/image:tag",
			registryHost: "192.168.1.100:5000",
			expected:     true,
			description:  "IP address registry should match exactly",
		},
		"ipv6 address without port": {
			imageRef:     "[::1]/image:tag",
			registryHost: "[::1]",
			expected:     true,
			description:  "IPv6 address without port should match exactly",
		},
		"ipv6 full address without port": {
			imageRef:     "[2001:db8::1]/image:tag",
			registryHost: "[2001:db8::1]",
			expected:     true,
			description:  "Full IPv6 address without port should match exactly",
		},
		"complex path": {
			imageRef:     "registry.example.com/deep/nested/path/image:tag",
			registryHost: "registry.example.com",
			expected:     true,
			description:  "Complex path should match when registry is exact",
		},
		"registry with https scheme": {
			imageRef:     "registry.example.com/image:tag",
			registryHost: "https://registry.example.com",
			expected:     false,
			description:  "registry with https scheme is not a valid registry reference",
		},
		"registry with http scheme": {
			imageRef:     "registry.example.com:5000/image:tag",
			registryHost: "http://registry.example.com:5000",
			expected:     false,
			description:  "registry with http scheme is not a valid registry reference",
		},

		// Security test cases - should NOT use auth (prevent exploitation)
		"substring attack - malicious registry": {
			imageRef:     "malicious-registry.example.com.evil.com/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when target registry is substring of malicious hostname",
		},
		"substring attack - path injection": {
			imageRef:     "evil.com/registry.example.com/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when target registry appears in path",
		},
		"substring attack - domain prefix": {
			imageRef:     "sub.registry.example.com/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match subdomains",
		},
		"substring attack - port manipulation": {
			imageRef:     "registry.example.com.evil:443/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when target is substring with malicious port",
		},
		"substring attack - different port": {
			imageRef:     "registry.example.com:9999/image:tag",
			registryHost: "registry.example.com:5000",
			expected:     false,
			description:  "Should not match when ports are different",
		},
		"substring attack - unicode normalization": {
			imageRef:     "registrу.example.com/image:tag", // Contains Cyrillic 'у' instead of 'y'
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when Unicode characters are used to obfuscate registry",
		},
		"typosquatting attack": {
			imageRef:     "registr.example.com/image:tag", // Similar to registry.example.com
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match similar domain names to prevent typosquatting",
		},
		"subdomain hijack attempt": {
			imageRef:     "evil.registry.example.com/image:tag", // Attacker controls subdomain
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when attacker controls subdomain",
		},
		"port confusion attack": {
			imageRef:     "registry.example.com:80/image:tag", // Attacker uses different port
			registryHost: "registry.example.com:443",
			expected:     false,
			description:  "Should not match when attacker uses different port to bypass auth",
		},
		"path traversal attempt": {
			imageRef:     "evil.com/../registry.example.com/image:tag", // Attacker attempts path traversal
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when attacker attempts path traversal in hostname",
		},
		"invalid registry format": {
			imageRef:     "registry.example.com/image:tag",
			registryHost: "registry.example.com:abc", // Invalid port format
			expected:     false,
			description:  "Should not match when registry host has invalid port format",
		},
		"invalid ipv6 address": {
			imageRef:     "[::1]/image:tag",
			registryHost: "[::1:5000", // Malformed IPv6 address
			expected:     false,
			description:  "Should not match when registry host has malformed IPv6 address",
		},
		"homograph attack simulation": {
			imageRef:     "registrу.example.com/image:tag", // Contains Cyrillic 'у' instead of 'y'
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when attacker uses similar-looking Unicode characters",
		},

		// Edge cases
		"docker hub image - no auth configured": {
			imageRef:     "ubuntu:20.04",
			registryHost: "docker.io",
			expected:     true,
			description:  "Should match Docker Hub when explicitly configured",
		},
		"docker hub image - private registry configured": {
			imageRef:     "ubuntu:20.04",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not use private registry auth for Docker Hub images",
		},
		"empty registry host": {
			imageRef:     "registry.example.com/image:tag",
			registryHost: "",
			expected:     false,
			description:  "Should not use auth when no registry is configured",
		},
		"empty image ref": {
			imageRef:     "",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not use auth for empty image reference",
		},
		"case sensitivity": {
			imageRef:     "Registry.Example.Com/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should be case sensitive for security",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := useAuth(tt.imageRef, tt.registryHost)
			if result != tt.expected {
				t.Errorf("shouldUseAuth(%q, %q) = %v, expected %v - %s",
					tt.imageRef, tt.registryHost, result, tt.expected, tt.description)
			}
		})
	}
}

// TestSecurityScenarios tests specific security scenarios to ensure the implementation
// is resistant to various attack vectors.
func TestSecurityScenarios(t *testing.T) {
	scenarios := []struct {
		name         string
		imageRef     string
		registryHost string
		shouldAuth   bool
		description  string
	}{
		{
			name:         "typosquatting attack",
			imageRef:     "registr.example.com/malware:latest",
			registryHost: "registry.example.com",
			shouldAuth:   false,
			description:  "Attacker uses similar domain name",
		},
		{
			name:         "subdomain hijack attempt",
			imageRef:     "evil.registry.example.com/image:latest",
			registryHost: "registry.example.com",
			shouldAuth:   false,
			description:  "Attacker controls subdomain",
		},
		{
			name:         "homograph attack simulation",
			imageRef:     "registrу.example.com/image:latest", // Contains Cyrillic 'у' instead of 'y'
			registryHost: "registry.example.com",
			shouldAuth:   false,
			description:  "Attacker uses similar-looking Unicode characters",
		},
		{
			name:         "port confusion",
			imageRef:     "registry.example.com:80/image:latest",
			registryHost: "registry.example.com:443",
			shouldAuth:   false,
			description:  "Attacker uses different port to bypass auth",
		},
		{
			name:         "path traversal attempt",
			imageRef:     "evil.com/../registry.example.com/image:latest",
			registryHost: "registry.example.com",
			shouldAuth:   false,
			description:  "Attacker attempts path traversal in hostname",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			result := useAuth(scenario.imageRef, scenario.registryHost)
			if result != scenario.shouldAuth {
				t.Errorf("Security test failed: %s\n"+
					"shouldUseAuth(%q, %q) = %v, expected %v",
					scenario.description, scenario.imageRef, scenario.registryHost, result, scenario.shouldAuth)
			}
		})
	}
}
