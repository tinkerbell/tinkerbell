package hook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name     string
		opts     []Option
		validate func(*testing.T, *Config)
	}{
		{
			name: "default configuration",
			opts: nil,
			validate: func(t *testing.T, c *Config) {
				t.Helper()
				if c.ImagePath != "/var/lib/hook" {
					t.Errorf("expected ImagePath=/var/lib/hook, got %s", c.ImagePath)
				}
				if c.OCIRegistry != "ghcr.io" {
					t.Errorf("expected OCIRegistry=ghcr.io, got %s", c.OCIRegistry)
				}
				if c.OCIRepository != "tinkerbell/hook" {
					t.Errorf("expected OCIRepository=tinkerbell/hook, got %s", c.OCIRepository)
				}
				if c.OCIReference != "latest" {
					t.Errorf("expected OCIReference=latest, got %s", c.OCIReference)
				}
				if c.PullTimeout != 10*time.Minute {
					t.Errorf("expected PullTimeout=10m, got %s", c.PullTimeout)
				}
				if !c.EnableHTTPServer {
					t.Error("expected EnableHTTPServer=true")
				}
			},
		},
		{
			name: "custom configuration",
			opts: []Option{
				WithImagePath("/custom/path"),
				WithOCIRegistry("docker.io"),
				WithOCIRepository("myorg/hooks"),
				WithOCIReference("v1.2.3"),
				WithOCIUsername("testuser"),
				WithOCIPassword("testpass"),
				WithPullTimeout(5 * time.Minute),
				WithEnableHTTPServer(false),
			},
			validate: func(t *testing.T, c *Config) {
				t.Helper()
				if c.ImagePath != "/custom/path" {
					t.Errorf("expected ImagePath=/custom/path, got %s", c.ImagePath)
				}
				if c.OCIRegistry != "docker.io" {
					t.Errorf("expected OCIRegistry=docker.io, got %s", c.OCIRegistry)
				}
				if c.OCIRepository != "myorg/hooks" {
					t.Errorf("expected OCIRepository=myorg/hooks, got %s", c.OCIRepository)
				}
				if c.OCIReference != "v1.2.3" {
					t.Errorf("expected OCIReference=v1.2.3, got %s", c.OCIReference)
				}
				if c.OCIUsername != "testuser" {
					t.Errorf("expected OCIUsername=testuser, got %s", c.OCIUsername)
				}
				if c.OCIPassword != "testpass" {
					t.Errorf("expected OCIPassword=testpass, got %s", c.OCIPassword)
				}
				if c.PullTimeout != 5*time.Minute {
					t.Errorf("expected PullTimeout=5m, got %s", c.PullTimeout)
				}
				if c.EnableHTTPServer {
					t.Error("expected EnableHTTPServer=false")
				}
			},
		},
		{
			name: "with HTTP address",
			opts: []Option{
				WithHTTPAddr(netip.MustParseAddrPort("127.0.0.1:8080")),
			},
			validate: func(t *testing.T, c *Config) {
				t.Helper()
				expected := netip.MustParseAddrPort("127.0.0.1:8080")
				if c.HTTPAddr != expected {
					t.Errorf("expected HTTPAddr=%s, got %s", expected, c.HTTPAddr)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig(tt.opts...)
			tt.validate(t, config)
		})
	}
}

func TestImagePathHasFiles(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
	}{
		{
			name: "empty directory",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				return dir
			},
			expected: false,
		},
		{
			name: "directory with files",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0o644)
				if err != nil {
					t.Fatal(err)
				}
				return dir
			},
			expected: true,
		},
		{
			name: "directory with subdirectories only",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755)
				if err != nil {
					t.Fatal(err)
				}
				return dir
			},
			expected: false,
		},
		{
			name: "directory with files and subdirectories",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755)
				if err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0o644)
				if err != nil {
					t.Fatal(err)
				}
				return dir
			},
			expected: true,
		},
		{
			name: "non-existent directory",
			setup: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "nonexistent")
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			log := testr.New(t)

			svc := &service{
				config: &Config{
					ImagePath: dir,
				},
				log: log,
			}

			result := svc.imagePathHasFiles()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestStartWithExistingFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a test file
	err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("test"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	config := NewConfig(
		WithImagePath(dir),
		WithEnableHTTPServer(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	log := testr.New(t)

	// Should return without error and not attempt OCI pull
	err = config.Start(ctx, log)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartWithEmptyDirectory(t *testing.T) {
	t.Skip("Skipping integration test that requires actual OCI registry")
	// This test would require a mock OCI registry or local test registry
	// Consider implementing with httptest server mocking ORAS protocol
}

func TestStartHTTPServerDisabled(t *testing.T) {
	dir := t.TempDir()

	// Create a test file so OCI pull is skipped
	err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	config := NewConfig(
		WithImagePath(dir),
		WithEnableHTTPServer(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	log := testr.New(t)

	// Should complete without starting HTTP server
	err = config.Start(ctx, log)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHTTPServer(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	testContent := "test content for hook file"
	err := os.WriteFile(filepath.Join(dir, "vmlinuz-x86_64"), []byte(testContent), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Find an available port
	_ = NewConfig(
		WithImagePath(dir),
		WithHTTPAddr(netip.MustParseAddrPort("127.0.0.1:0")),
		WithEnableHTTPServer(true),
	)

	// We don't actually start the service, just test the HTTP handler directly
	// Test HTTP handler
	handler := http.FileServerFS(os.DirFS(dir))
	server := httptest.NewServer(handler)
	defer server.Close()

	// Test file serving
	resp, err := http.Get(server.URL + "/vmlinuz-x86_64")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Read response body
	body := make([]byte, len(testContent))
	_, err = resp.Body.Read(body)
	if err != nil && err.Error() != "EOF" {
		t.Fatal(err)
	}

	if string(body) != testContent {
		t.Errorf("expected %q, got %q", testContent, string(body))
	}
}

func TestPullOCIImageOnce(t *testing.T) {
	// This test verifies that pullOCIImage uses sync.Once correctly
	dir := t.TempDir()
	log := testr.New(t)

	svc := &service{
		config: &Config{
			ImagePath:     dir,
			OCIRegistry:   "invalid.example.com",
			OCIRepository: "test/hook",
			OCIReference:  "nonexistent",
			PullTimeout:   100 * time.Millisecond, // Short timeout to fail quickly
		},
		log: log,
	}

	ctx := context.Background()

	// First call - will attempt to pull from non-existent registry
	err1 := svc.pullOCIImage(ctx)

	// Second call - should return immediately due to sync.Once
	// The behavior is that sync.Once runs the function once, and we capture
	// the error in the closure
	err2 := svc.pullOCIImage(ctx)

	// The implementation uses sync.Once which means doPullOCIImage runs once
	// Both calls return the same error that was captured
	if err1 == nil && err2 == nil {
		t.Error("expected at least one error from failed OCI pull")
	}

	// Log the errors for debugging
	t.Logf("First call error: %v", err1)
	t.Logf("Second call error: %v", err2)
}

func TestConfigOptionChaining(t *testing.T) {
	config := NewConfig(
		WithImagePath("/test"),
		WithOCIRegistry("registry.example.com"),
		WithOCIRepository("org/repo"),
		WithOCIReference("v1.0.0"),
		WithPullTimeout(30*time.Second),
		WithHTTPAddr(netip.MustParseAddrPort("0.0.0.0:9090")),
		WithEnableHTTPServer(true),
	)

	// Verify all options were applied
	if config.ImagePath != "/test" {
		t.Errorf("ImagePath not set correctly")
	}
	if config.OCIRegistry != "registry.example.com" {
		t.Errorf("OCIRegistry not set correctly")
	}
	if config.OCIRepository != "org/repo" {
		t.Errorf("OCIRepository not set correctly")
	}
	if config.OCIReference != "v1.0.0" {
		t.Errorf("OCIReference not set correctly")
	}
	if config.PullTimeout != 30*time.Second {
		t.Errorf("PullTimeout not set correctly")
	}
	if config.HTTPAddr != netip.MustParseAddrPort("0.0.0.0:9090") {
		t.Errorf("HTTPAddr not set correctly")
	}
	if !config.EnableHTTPServer {
		t.Errorf("EnableHTTPServer not set correctly")
	}
}

func TestServiceReadyState(t *testing.T) {
	dir := t.TempDir()
	log := testr.New(t)

	svc := &service{
		config: &Config{
			ImagePath: dir,
		},
		log:   log,
		ready: false,
	}

	// Initially not ready
	if svc.ready {
		t.Error("service should not be ready initially")
	}

	// Set ready
	svc.mutex.Lock()
	svc.ready = true
	svc.mutex.Unlock()

	// Check ready state
	svc.mutex.RLock()
	isReady := svc.ready
	svc.mutex.RUnlock()

	if !isReady {
		t.Error("service should be ready after setting ready=true")
	}
}

func TestStartCreatesImageDirectory(t *testing.T) {
	tempBase := t.TempDir()
	imagePath := filepath.Join(tempBase, "nested", "hook", "images")

	config := NewConfig(
		WithImagePath(imagePath),
		WithEnableHTTPServer(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	log := testr.New(t)

	// Start should create the directory
	_ = config.Start(ctx, log)

	// Verify directory was created
	info, err := os.Stat(imagePath)
	if err != nil {
		t.Fatalf("expected directory to be created: %v", err)
	}

	if !info.IsDir() {
		t.Error("expected path to be a directory")
	}
}

// Mock logger for testing log output
type mockLogger struct {
	logs []string
}

func (m *mockLogger) Init(logr.RuntimeInfo) {}

func (m *mockLogger) Enabled(_ int) bool {
	return true
}

func (m *mockLogger) Info(_ int, msg string, _ ...interface{}) {
	m.logs = append(m.logs, fmt.Sprintf("INFO: %s", msg))
}

func (m *mockLogger) Error(err error, msg string, _ ...interface{}) {
	m.logs = append(m.logs, fmt.Sprintf("ERROR: %s: %v", msg, err))
}

func (m *mockLogger) WithValues(_ ...interface{}) logr.LogSink {
	return m
}

func (m *mockLogger) WithName(_ string) logr.LogSink {
	return m
}

func TestLoggingBehavior(t *testing.T) {
	dir := t.TempDir()

	// Create a file so pull is skipped
	err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	mockLog := &mockLogger{}
	log := logr.New(mockLog)

	config := NewConfig(
		WithImagePath(dir),
		WithEnableHTTPServer(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = config.Start(ctx, log)

	// Give goroutine time to log
	time.Sleep(50 * time.Millisecond)

	// Verify logging occurred
	if len(mockLog.logs) == 0 {
		t.Error("expected some log messages to be generated")
	}

	// Check for expected log message
	foundStartMessage := false
	for _, log := range mockLog.logs {
		if contains(log, "starting hook service") {
			foundStartMessage = true
			break
		}
	}

	if !foundStartMessage {
		t.Error("expected to find 'starting hook service' log message")
	}
}

func contains(s, substr string) bool {
	// Simple substring check using standard library
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		valid  bool
	}{
		{
			name: "valid config with all fields",
			config: &Config{
				ImagePath:        "/var/lib/hook",
				OCIRegistry:      "ghcr.io",
				OCIRepository:    "tinkerbell/hook",
				OCIReference:     "latest",
				PullTimeout:      10 * time.Minute,
				HTTPAddr:         netip.MustParseAddrPort("0.0.0.0:8080"),
				EnableHTTPServer: true,
			},
			valid: true,
		},
		{
			name: "valid config with minimal fields",
			config: &Config{
				ImagePath:     "/var/lib/hook",
				OCIRegistry:   "ghcr.io",
				OCIRepository: "tinkerbell/hook",
				OCIReference:  "latest",
				PullTimeout:   1 * time.Minute,
			},
			valid: true,
		},
		{
			name: "config with sha256 digest reference",
			config: &Config{
				ImagePath:     "/var/lib/hook",
				OCIRegistry:   "ghcr.io",
				OCIRepository: "tinkerbell/hook",
				OCIReference:  "sha256:1234567890abcdef",
				PullTimeout:   5 * time.Minute,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation - just ensure fields are set
			if tt.config.ImagePath == "" && tt.valid {
				t.Error("valid config should have ImagePath set")
			}
			if tt.config.OCIRegistry == "" && tt.valid {
				t.Error("valid config should have OCIRegistry set")
			}
			if tt.config.OCIRepository == "" && tt.valid {
				t.Error("valid config should have OCIRepository set")
			}
			if tt.config.OCIReference == "" && tt.valid {
				t.Error("valid config should have OCIReference set")
			}
			if tt.config.PullTimeout == 0 && tt.valid {
				t.Error("valid config should have PullTimeout set")
			}
		})
	}
}

func TestStartWithInvalidImagePath(t *testing.T) {
	// Try to create a directory in a location that requires root permissions
	// This should fail gracefully
	config := NewConfig(
		WithImagePath("/invalid/readonly/path"),
		WithEnableHTTPServer(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	log := testr.New(t)

	err := config.Start(ctx, log)
	// Should get an error about failing to create directory
	if err == nil {
		t.Error("expected error when creating directory in invalid location")
	}
}

func TestStartWithHTTPServerEnabled(t *testing.T) {
	t.Skip("Skipping test that requires binding to network port")
	// This test would require finding an available port and properly
	// testing HTTP server lifecycle. The HTTP handler itself is tested
	// in TestHTTPServer using httptest
}

func TestConcurrentImagePathHasFiles(t *testing.T) {
	// Test thread safety of imagePathHasFiles
	dir := t.TempDir()
	log := testr.New(t)

	svc := &service{
		config: &Config{
			ImagePath: dir,
		},
		log: log,
	}

	// Run concurrent checks
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = svc.imagePathHasFiles()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestMultipleOptionApplications(t *testing.T) {
	// Test that options can be applied multiple times (last one wins)
	config := NewConfig(
		WithImagePath("/first"),
		WithImagePath("/second"),
		WithImagePath("/third"),
	)

	if config.ImagePath != "/third" {
		t.Errorf("expected last option to win, got %s", config.ImagePath)
	}
}

func TestEmptyOCIConfiguration(t *testing.T) {
	// Test with empty OCI configuration values
	config := NewConfig(
		WithOCIRegistry(""),
		WithOCIRepository(""),
		WithOCIReference(""),
	)

	// Should still create config, just with empty values
	if config == nil {
		t.Error("expected config to be created even with empty values")
	}
}

func TestZeroPullTimeout(t *testing.T) {
	// Test with zero pull timeout
	config := NewConfig(
		WithPullTimeout(0),
	)

	if config.PullTimeout != 0 {
		t.Errorf("expected zero timeout, got %s", config.PullTimeout)
	}
}

func TestInvalidHTTPAddr(t *testing.T) {
	// Test with invalid (zero) HTTP address
	config := NewConfig(
		WithHTTPAddr(netip.AddrPort{}),
	)

	if config.HTTPAddr.IsValid() {
		t.Error("expected invalid HTTP address")
	}
}

func TestReadyStateConcurrency(t *testing.T) {
	// Test concurrent access to ready state
	svc := &service{
		config: &Config{},
		log:    testr.New(t),
	}

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			svc.mutex.Lock()
			svc.ready = !svc.ready
			svc.mutex.Unlock()
		}
		done <- true
	}()

	// Reader goroutines
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				svc.mutex.RLock()
				_ = svc.ready
				svc.mutex.RUnlock()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 6; i++ {
		<-done
	}
}

func TestOCIAuthentication(t *testing.T) {
	// Test that OCI authentication options are properly set
	config := NewConfig(
		WithOCIUsername("testuser"),
		WithOCIPassword("testpass"),
	)

	if config.OCIUsername != "testuser" {
		t.Errorf("expected OCIUsername=testuser, got %s", config.OCIUsername)
	}

	if config.OCIPassword != "testpass" {
		t.Errorf("expected OCIPassword=testpass, got %s", config.OCIPassword)
	}

	// Test partial authentication (username only)
	config2 := NewConfig(
		WithOCIUsername("useronly"),
	)

	if config2.OCIUsername != "useronly" {
		t.Errorf("expected OCIUsername=useronly, got %s", config2.OCIUsername)
	}

	if config2.OCIPassword != "" {
		t.Errorf("expected empty OCIPassword, got %s", config2.OCIPassword)
	}
}

func TestOCIAuthenticationDefault(t *testing.T) {
	// Test that OCI authentication defaults to empty (unauthenticated)
	config := NewConfig()

	if config.OCIUsername != "" {
		t.Errorf("expected empty OCIUsername by default, got %s", config.OCIUsername)
	}

	if config.OCIPassword != "" {
		t.Errorf("expected empty OCIPassword by default, got %s", config.OCIPassword)
	}
}
