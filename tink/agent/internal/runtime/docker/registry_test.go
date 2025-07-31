package docker

import (
	"testing"
)

func TestShouldUseAuth(t *testing.T) {
	tests := []struct {
		name         string
		imageRef     string
		registryHost string
		expected     bool
		description  string
	}{
		// Positive cases - should use auth
		{
			name:         "exact match",
			imageRef:     "registry.example.com/namespace/image:tag",
			registryHost: "registry.example.com",
			expected:     true,
			description:  "Exact hostname match should use auth",
		},
		{
			name:         "exact match with port",
			imageRef:     "registry.example.com:5000/image:tag",
			registryHost: "registry.example.com:5000",
			expected:     true,
			description:  "Exact hostname and port match should use auth",
		},
		{
			name:         "localhost with port",
			imageRef:     "localhost:5000/image:tag",
			registryHost: "localhost:5000",
			expected:     true,
			description:  "Localhost with port should match exactly",
		},
		{
			name:         "registry with https scheme",
			imageRef:     "registry.example.com/image:tag",
			registryHost: "https://registry.example.com",
			expected:     true,
			description:  "Should normalize https scheme and match",
		},
		{
			name:         "registry with http scheme",
			imageRef:     "registry.example.com:5000/image:tag",
			registryHost: "http://registry.example.com:5000",
			expected:     true,
			description:  "Should normalize http scheme and match",
		},

		// Security test cases - should NOT use auth (prevent exploitation)
		{
			name:         "substring attack - malicious registry",
			imageRef:     "malicious-registry.example.com.evil.com/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when target registry is substring of malicious hostname",
		},
		{
			name:         "substring attack - path injection",
			imageRef:     "evil.com/registry.example.com/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when target registry appears in path",
		},
		{
			name:         "substring attack - domain prefix",
			imageRef:     "sub.registry.example.com/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match subdomains",
		},
		{
			name:         "substring attack - port manipulation",
			imageRef:     "registry.example.com.evil:443/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not match when target is substring with malicious port",
		},
		{
			name:         "substring attack - different port",
			imageRef:     "registry.example.com:9999/image:tag",
			registryHost: "registry.example.com:5000",
			expected:     false,
			description:  "Should not match when ports are different",
		},

		// Edge cases
		{
			name:         "docker hub image - no auth configured",
			imageRef:     "ubuntu:20.04",
			registryHost: "docker.io",
			expected:     true,
			description:  "Should match Docker Hub when explicitly configured",
		},
		{
			name:         "docker hub image - private registry configured",
			imageRef:     "ubuntu:20.04",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not use private registry auth for Docker Hub images",
		},
		{
			name:         "empty registry host",
			imageRef:     "registry.example.com/image:tag",
			registryHost: "",
			expected:     false,
			description:  "Should not use auth when no registry is configured",
		},
		{
			name:         "empty image ref",
			imageRef:     "",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should not use auth for empty image reference",
		},
		{
			name:         "case sensitivity",
			imageRef:     "Registry.Example.Com/image:tag",
			registryHost: "registry.example.com",
			expected:     false,
			description:  "Should be case sensitive for security",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldUseAuth(tt.imageRef, tt.registryHost)
			if result != tt.expected {
				t.Errorf("shouldUseAuth(%q, %q) = %v, expected %v - %s",
					tt.imageRef, tt.registryHost, result, tt.expected, tt.description)
			}
		})
	}
}

func TestExtractRegistryHostname(t *testing.T) {
	tests := []struct {
		name     string
		imageRef string
		expected string
	}{
		{
			name:     "full registry with namespace and tag",
			imageRef: "registry.example.com/namespace/image:tag",
			expected: "registry.example.com",
		},
		{
			name:     "registry with port",
			imageRef: "registry.example.com:5000/image:tag",
			expected: "registry.example.com:5000",
		},
		{
			name:     "localhost with port",
			imageRef: "localhost:5000/image:tag",
			expected: "localhost:5000",
		},
		{
			name:     "docker hub official image",
			imageRef: "ubuntu:20.04",
			expected: "docker.io",
		},
		{
			name:     "docker hub image without tag",
			imageRef: "ubuntu",
			expected: "docker.io",
		},
		{
			name:     "docker hub with namespace",
			imageRef: "library/ubuntu:20.04",
			expected: "docker.io",
		},
		{
			name:     "docker hub user image",
			imageRef: "username/image:tag",
			expected: "docker.io",
		},
		{
			name:     "ip address registry",
			imageRef: "192.168.1.100:5000/image:tag",
			expected: "192.168.1.100:5000",
		},
		{
			name:     "empty image",
			imageRef: "",
			expected: "",
		},
		{
			name:     "complex path",
			imageRef: "registry.example.com/deep/nested/path/image:tag",
			expected: "registry.example.com",
		},
		{
			name:     "docker hub image with complex tag",
			imageRef: "myapp:v1.2.3",
			expected: "docker.io",
		},
		{
			name:     "docker hub image with timestamp tag",
			imageRef: "myapp:2023-12-25",
			expected: "docker.io",
		},
		{
			name:     "ipv6 address with port",
			imageRef: "[::1]:5000/image:tag",
			expected: "[::1]:5000",
		},
		{
			name:     "ipv6 full address with port",
			imageRef: "[2001:db8::1]:5000/image:tag",
			expected: "[2001:db8::1]:5000",
		},
		{
			name:     "localhost without port",
			imageRef: "localhost/image:tag",
			expected: "localhost",
		},
		{
			name:     "ipv4 address without port",
			imageRef: "192.168.1.100/image:tag",
			expected: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRegistryHostname(tt.imageRef)
			if result != tt.expected {
				t.Errorf("extractRegistryHostname(%q) = %q, expected %q",
					tt.imageRef, result, tt.expected)
			}
		})
	}
}

// TestIsRegistryHostname tests the new hostname detection logic.
func TestIsRegistryHostname(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Should be recognized as registry hostnames
		{
			name:     "registry with domain",
			input:    "registry.example.com",
			expected: true,
		},
		{
			name:     "registry with domain and port",
			input:    "registry.example.com:5000",
			expected: true,
		},
		{
			name:     "localhost",
			input:    "localhost",
			expected: true,
		},
		{
			name:     "localhost with port",
			input:    "localhost:5000",
			expected: true,
		},
		{
			name:     "ipv4 address",
			input:    "192.168.1.100",
			expected: true,
		},
		{
			name:     "ipv4 address with port",
			input:    "192.168.1.100:5000",
			expected: true,
		},
		{
			name:     "ipv6 address with port",
			input:    "[::1]:5000",
			expected: true,
		},
		{
			name:     "ipv6 full address with port",
			input:    "[2001:db8::1]:5000",
			expected: true,
		},
		{
			name:     "subdomain registry",
			input:    "sub.registry.example.com",
			expected: true,
		},

		// Should NOT be recognized as registry hostnames (Docker Hub images)
		{
			name:     "simple image name",
			input:    "ubuntu",
			expected: false,
		},
		{
			name:     "image with simple tag",
			input:    "ubuntu:20.04",
			expected: false,
		},
		{
			name:     "image with complex tag",
			input:    "myapp:v1.2.3",
			expected: false,
		},
		{
			name:     "image with timestamp tag",
			input:    "myapp:2023-12-25",
			expected: false,
		},
		{
			name:     "username format",
			input:    "username",
			expected: false,
		},
		{
			name:     "image with hash tag",
			input:    "image:sha256-abcd1234",
			expected: false,
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "just colon",
			input:    ":",
			expected: false,
		},
		{
			name:     "just dot",
			input:    ".",
			expected: false,
		},
		{
			name:     "malformed ipv6",
			input:    "[::1",
			expected: false,
		},
		{
			name:     "invalid port",
			input:    "registry.example.com:abc",
			expected: false,
		},
		{
			name:     "port too large",
			input:    "registry.example.com:999999",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRegistryHostname(tt.input)
			if result != tt.expected {
				t.Errorf("isRegistryHostname(%q) = %v, expected %v",
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeRegistryHostname(t *testing.T) {
	tests := []struct {
		name         string
		registryHost string
		expected     string
	}{
		{
			name:         "plain hostname",
			registryHost: "registry.example.com",
			expected:     "registry.example.com",
		},
		{
			name:         "hostname with port",
			registryHost: "registry.example.com:5000",
			expected:     "registry.example.com:5000",
		},
		{
			name:         "https scheme",
			registryHost: "https://registry.example.com",
			expected:     "registry.example.com",
		},
		{
			name:         "http scheme",
			registryHost: "http://registry.example.com:5000",
			expected:     "registry.example.com:5000",
		},
		{
			name:         "https with path",
			registryHost: "https://registry.example.com/v2",
			expected:     "registry.example.com",
		},
		{
			name:         "localhost",
			registryHost: "localhost:5000",
			expected:     "localhost:5000",
		},
		{
			name:         "ip address",
			registryHost: "192.168.1.100:5000",
			expected:     "192.168.1.100:5000",
		},
		{
			name:         "empty string",
			registryHost: "",
			expected:     "",
		},
		{
			name:         "malformed url",
			registryHost: "https://[invalid-url",
			expected:     "[invalid-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeRegistryHostname(tt.registryHost)
			if result != tt.expected {
				t.Errorf("normalizeRegistryHostname(%q) = %q, expected %q",
					tt.registryHost, result, tt.expected)
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
			result := shouldUseAuth(scenario.imageRef, scenario.registryHost)
			if result != scenario.shouldAuth {
				t.Errorf("Security test failed: %s\n"+
					"shouldUseAuth(%q, %q) = %v, expected %v",
					scenario.description, scenario.imageRef, scenario.registryHost, result, scenario.shouldAuth)
			}
		})
	}
}
