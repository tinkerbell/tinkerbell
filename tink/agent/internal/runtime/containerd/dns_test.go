package containerd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseResolvConf(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantNS     []string
		wantSearch []string
		wantOpts   []string
	}{
		{
			name: "standard resolv.conf",
			content: `nameserver 8.8.8.8
nameserver 8.8.4.4
search example.com corp.example.com
options ndots:5 timeout:2
`,
			wantNS:     []string{"8.8.8.8", "8.8.4.4"},
			wantSearch: []string{"example.com", "corp.example.com"},
			wantOpts:   []string{"ndots:5", "timeout:2"},
		},
		{
			name: "with comments and empty lines",
			content: `# This is a comment
; Another comment

nameserver 10.0.0.1
search local
`,
			wantNS:     []string{"10.0.0.1"},
			wantSearch: []string{"local"},
			wantOpts:   nil,
		},
		{
			name: "systemd-resolved",
			content: `nameserver 127.0.0.53
options edns0 trust-ad
search .
`,
			wantNS:     []string{"127.0.0.53"},
			wantSearch: []string{"."},
			wantOpts:   []string{"edns0", "trust-ad"},
		},
		{
			name:       "empty",
			content:    "",
			wantNS:     nil,
			wantSearch: nil,
			wantOpts:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, search, opts := parseResolvConf(tt.content)
			if !slices.Equal(ns, tt.wantNS) {
				t.Errorf("nameservers = %v, want %v", ns, tt.wantNS)
			}
			if !slices.Equal(search, tt.wantSearch) {
				t.Errorf("search = %v, want %v", search, tt.wantSearch)
			}
			if !slices.Equal(opts, tt.wantOpts) {
				t.Errorf("options = %v, want %v", opts, tt.wantOpts)
			}
		})
	}
}

func TestIsLocalhost(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.53", true},
		{"127.0.1.1", true},
		{"::1", true},
		{"8.8.8.8", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"not-an-ip", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got := isLocalhost(tt.ip)
			if got != tt.want {
				t.Errorf("isLocalhost(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestBuildResolvConf(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "resolv.conf")

	err := buildResolvConf(dst, false)
	if err != nil {
		t.Fatalf("buildResolvConf() error = %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading generated resolv.conf: %v", err)
	}

	s := string(content)

	// Verify it has at least one nameserver line
	if !strings.Contains(s, "nameserver ") {
		t.Error("generated resolv.conf has no nameserver lines")
	}

	// Verify no localhost nameservers
	ns, _, _ := parseResolvConf(s)
	for _, n := range ns {
		if isLocalhost(n) {
			t.Errorf("generated resolv.conf contains localhost nameserver: %s", n)
		}
	}
}

func TestBuildResolvConfAllowLocalhost(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "resolv.conf")

	err := buildResolvConf(dst, true)
	if err != nil {
		t.Fatalf("buildResolvConf(allowLocalhostDNS=true) error = %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading generated resolv.conf: %v", err)
	}

	s := string(content)

	// Verify it has at least one nameserver line
	if !strings.Contains(s, "nameserver ") {
		t.Error("generated resolv.conf has no nameserver lines")
	}
}

func TestWriteResolvConf(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "resolv.conf")

	err := writeResolvConf(dst,
		[]string{"1.1.1.1", "8.8.8.8"},
		[]string{"example.com", "local"},
		[]string{"ndots:5"},
	)
	if err != nil {
		t.Fatalf("writeResolvConf() error = %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "nameserver 1.1.1.1\n") {
		t.Error("missing nameserver 1.1.1.1")
	}
	if !strings.Contains(s, "nameserver 8.8.8.8\n") {
		t.Error("missing nameserver 8.8.8.8")
	}
	if !strings.Contains(s, "search example.com local\n") {
		t.Error("missing search domains")
	}
	if !strings.Contains(s, "options ndots:5\n") {
		t.Error("missing options")
	}
}

func TestBuildHosts(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "hosts")

	// buildHosts no longer truncates; callers pass the final hostname.
	err := buildHosts(dst, "my-container")
	if err != nil {
		t.Fatalf("buildHosts() error = %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "127.0.0.1\tlocalhost\n") {
		t.Error("missing localhost entry")
	}
	if !strings.Contains(s, "::1\t\tlocalhost") {
		t.Error("missing ipv6 localhost entry")
	}
	if !strings.Contains(s, "127.0.0.1\tmy-container\n") {
		t.Errorf("missing or incorrect IPv4 hostname entry, got:\n%s", s)
	}
	if !strings.Contains(s, "::1\t\tmy-container\n") {
		t.Errorf("missing or incorrect IPv6 hostname entry, got:\n%s", s)
	}
}

func TestPrepareDNSFiles(t *testing.T) {
	df, err := prepareDNSFiles(false)
	if err != nil {
		t.Fatalf("prepareDNSFiles() error = %v", err)
	}
	defer func() {
		if err := df.cleanup(); err != nil {
			t.Errorf("cleanup error: %v", err)
		}
	}()

	// Verify all files exist
	for _, f := range []string{df.resolvConf, df.hosts, df.hostname} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected file %s to exist: %v", f, err)
		}
	}

	// Before setHostname, files should be empty placeholders
	content, err := os.ReadFile(df.hosts)
	if err != nil {
		t.Fatalf("reading hosts: %v", err)
	}
	if len(content) != 0 {
		t.Errorf("hosts should be empty before setHostname, got %q", string(content))
	}

	// After setHostname with isolated networking, hostname is truncated to 12 chars
	if err := df.setHostname("test-container-id-12345", false); err != nil {
		t.Fatalf("setHostname() error = %v", err)
	}
	hostContent, err := os.ReadFile(df.hostname)
	if err != nil {
		t.Fatalf("reading hostname: %v", err)
	}
	if got := strings.TrimSpace(string(hostContent)); got != "test-contain" {
		t.Errorf("hostname = %q, want %q", got, "test-contain")
	}
}

func TestSetHostnameHostNetwork(t *testing.T) {
	df, err := prepareDNSFiles(true)
	if err != nil {
		t.Fatalf("prepareDNSFiles() error = %v", err)
	}
	defer func() { _ = df.cleanup() }()

	// Host network mode: full hostname, no truncation
	if err := df.setHostname("my-real-hostname", true); err != nil {
		t.Fatalf("setHostname() error = %v", err)
	}

	// Hostname file should have the full hostname, not truncated
	hostContent, err := os.ReadFile(df.hostname)
	if err != nil {
		t.Fatalf("reading hostname: %v", err)
	}
	if got := strings.TrimSpace(string(hostContent)); got != "my-real-hostname" {
		t.Errorf("hostname = %q, want %q", got, "my-real-hostname")
	}
}

func TestDNSFilesCleanup(t *testing.T) {
	df, err := prepareDNSFiles(false)
	if err != nil {
		t.Fatalf("prepareDNSFiles() error = %v", err)
	}

	dir := df.dir
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temp dir should exist: %v", err)
	}

	if err := df.cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("temp dir should have been removed after cleanup")
	}
}

func TestDNSFilesCleanupNil(t *testing.T) {
	// nil dnsFiles should be safe to call cleanup on
	var df *dnsFiles
	if err := df.cleanup(); err != nil {
		t.Errorf("cleanup on nil dnsFiles should not error: %v", err)
	}
}

func TestGenerateID(t *testing.T) {
	id, err := generateID()
	if err != nil {
		t.Fatalf("generateID() error = %v", err)
	}
	// Should be 64 hex characters (32 bytes)
	if len(id) != 64 {
		t.Errorf("generateID() length = %d, want 64", len(id))
	}
	// Should be valid hex
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("generateID() contains non-hex character: %c", c)
		}
	}
	// Should be unique
	id2, err := generateID()
	if err != nil {
		t.Fatalf("generateID() second call error = %v", err)
	}
	if id == id2 {
		t.Error("generateID() should produce unique IDs")
	}
}
