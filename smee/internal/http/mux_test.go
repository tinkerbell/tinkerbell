package http

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestNewMultiplexer(t *testing.T) {
	tests := map[string]struct {
		opts        []MultiplexerOption
		expectError bool
		errorMsg    string
		checkServer bool // Whether to check for HTTP server
	}{
		"ValidConfigWithHTTPServer": {
			opts: []MultiplexerOption{
				WithHTTPServer(&http.Server{
					ReadHeaderTimeout: 10 * time.Second,
				}),
			},
			expectError: false,
			checkServer: true,
		},
		"ConfigWithTLSConfig": {
			opts: []MultiplexerOption{
				WithTLSConfig(&tls.Config{
					Certificates: []tls.Certificate{},
				}),
			},
			expectError: false,
			checkServer: false, // Don't check for HTTP server when only TLS config is provided
		},
		"MissingHTTPServer": {
			opts: []MultiplexerOption{
				WithLogger(logr.Discard()),
			},
			expectError: true,
			errorMsg:    "at least one of HTTPServer or TLSConfig must be provided",
		},
		"ValidConfigWithoutLogger": {
			opts: []MultiplexerOption{
				WithHTTPServer(&http.Server{
					ReadHeaderTimeout: 10 * time.Second,
				}),
			},
			expectError: false,
			checkServer: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			multiplexer, err := NewMultiplexer(tt.opts...)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorMsg, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Failed to create multiplexer: %v", err)
			}

			if multiplexer == nil {
				t.Fatal("Multiplexer is nil")
			}

			if tt.checkServer && multiplexer.httpServer == nil {
				t.Error("HTTP server should be set")
			}
		})
	}
}

// TestNewMultiplexerWithCertFiles has been removed since the functional options pattern
// no longer supports direct certificate file loading. TLS config should be created
// and passed to WithTLSConfig instead.

func TestMultiplexerListenAndServeErrors(t *testing.T) {
	tests := map[string]struct {
		opts        []MultiplexerOption
		method      string
		expectError bool
		errorMsg    string
	}{
		"ListenAndServeTLS_NoTLSConfig": {
			opts: []MultiplexerOption{
				WithHTTPServer(&http.Server{
					ReadHeaderTimeout: 10 * time.Second,
				}),
				WithLogger(logr.Discard()),
			},
			method:      "ListenAndServeTLS",
			expectError: true,
			errorMsg:    "no TLS configuration provided",
		},
		"ListenAndServeBoth_NoTLSConfig": {
			opts: []MultiplexerOption{
				WithHTTPServer(&http.Server{
					ReadHeaderTimeout: 10 * time.Second,
				}),
				WithLogger(logr.Discard()),
			},
			method:      "ListenAndServeBoth",
			expectError: true,
			errorMsg:    "no TLS configuration provided for dual protocol serving",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			multiplexer, err := NewMultiplexer(tt.opts...)
			if err != nil {
				t.Fatalf("Failed to create multiplexer: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			var testErr error
			switch tt.method {
			case "ListenAndServeTLS":
				testErr = multiplexer.ListenAndServeTLS(ctx, "localhost:0")
			case "ListenAndServeBoth":
				testErr = multiplexer.ListenAndServeBoth(ctx, "localhost:0")
			}

			if tt.expectError {
				if testErr == nil {
					t.Error("Expected error but got none")
				}
				if tt.errorMsg != "" && !strings.Contains(testErr.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorMsg, testErr)
				}
			} else if testErr != nil {
				t.Errorf("Unexpected error: %v", testErr)
			}
		})
	}
}

func TestProtocolConstants(t *testing.T) {
	tests := map[string]struct {
		protocol Protocol
		expected int
	}{
		"HTTP":  {ProtocolHTTP, 0},
		"HTTPS": {ProtocolHTTPS, 1},
		"Both":  {ProtocolBoth, 2},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if int(tt.protocol) != tt.expected {
				t.Errorf("Expected %s to be %d, got %d", name, tt.expected, int(tt.protocol))
			}
		})
	}
}

func TestMultiplexerServerShutdown(t *testing.T) {
	opts := []MultiplexerOption{
		WithHTTPServer(&http.Server{
			ReadHeaderTimeout: 10 * time.Second,
		}),
		WithLogger(logr.Discard()),
	}

	multiplexer, err := NewMultiplexer(opts...)
	if err != nil {
		t.Fatalf("Failed to create multiplexer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		err := multiplexer.ListenAndServe(ctx, "localhost:0")
		errChan <- err
	}()

	// Cancel context after a brief delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for server to shut down
	select {
	case err := <-errChan:
		// Server should shut down gracefully
		if err != nil && !strings.Contains(err.Error(), "Server closed") &&
			!strings.Contains(err.Error(), "use of closed network connection") &&
			!strings.Contains(err.Error(), "bind: address already in use") {
			t.Errorf("Unexpected error during shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not shut down within timeout")
	}
}

func TestCloneHTTPServer(t *testing.T) {
	originalServer := &http.Server{
		Handler:           http.NewServeMux(),
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	opts := []MultiplexerOption{
		WithHTTPServer(originalServer),
		WithLogger(logr.Discard()),
	}

	multiplexer, err := NewMultiplexer(opts...)
	if err != nil {
		t.Fatalf("Failed to create multiplexer: %v", err)
	}

	cloned := multiplexer.cloneHTTPServer()

	// Verify the clone has the same configuration
	if cloned.Handler != originalServer.Handler {
		t.Error("Handler should be the same")
	}
	if cloned.ReadTimeout != originalServer.ReadTimeout {
		t.Error("ReadTimeout should be the same")
	}
	if cloned.ReadHeaderTimeout != originalServer.ReadHeaderTimeout {
		t.Error("ReadHeaderTimeout should be the same")
	}
	if cloned.WriteTimeout != originalServer.WriteTimeout {
		t.Error("WriteTimeout should be the same")
	}
	if cloned.IdleTimeout != originalServer.IdleTimeout {
		t.Error("IdleTimeout should be the same")
	}
	if cloned.MaxHeaderBytes != originalServer.MaxHeaderBytes {
		t.Error("MaxHeaderBytes should be the same")
	}

	// Verify it's a different instance
	if cloned == originalServer {
		t.Error("Cloned server should be a different instance")
	}
}

func TestHandleConnection(t *testing.T) {
	// Create a multiplexer with TLS config for testing protocol detection
	opts := []MultiplexerOption{
		WithHTTPServer(&http.Server{
			Handler: http.NewServeMux(),
		}),
		WithTLSConfig(&tls.Config{
			Certificates: []tls.Certificate{},
		}),
		WithLogger(logr.Discard()),
	}

	multiplexer, err := NewMultiplexer(opts...)
	if err != nil {
		t.Fatalf("Failed to create multiplexer: %v", err)
	}

	tests := map[string]struct {
		data      []byte
		expectTLS bool
	}{
		"HTTPConnection": {
			data:      []byte("GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			expectTLS: false,
		},
		"TLSConnection": {
			data:      []byte{0x16, 0x03, 0x01, 0x00, 0x01}, // TLS handshake
			expectTLS: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Create a mock connection
			mockConn := newMockConn(tt.data)

			// This test mainly verifies handleConnection doesn't panic
			// Full integration testing requires real network connections
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("handleConnection panicked: %v", r)
				}
			}()

			// Call handleConnection in a goroutine since it may block
			done := make(chan bool, 1)
			go func() {
				defer func() {
					done <- true
				}()
				multiplexer.handleConnection(context.Background(), mockConn)
			}()

			// Wait briefly for the goroutine to process
			select {
			case <-done:
				// Success
			case <-time.After(100 * time.Millisecond):
				// Timeout is okay, just testing that it doesn't panic
			}
		})
	}
}

func TestMultiplexerProtocolDetection(t *testing.T) {
	// Test protocol detection with buffered connections
	tests := map[string]struct {
		data     []byte
		expected byte
	}{
		"HTTPRequest": {
			data:     []byte("GET / HTTP/1.1\r\n"),
			expected: 'G',
		},
		"TLSHandshake": {
			data:     []byte{0x16, 0x03, 0x01, 0x00, 0x10},
			expected: 0x16,
		},
		"POSTRequest": {
			data:     []byte("POST /api HTTP/1.1\r\n"),
			expected: 'P',
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mockConn := newMockConn(tt.data)
			buffered := newBufferedConn(mockConn, logr.Discard())

			firstByte, err := buffered.peekFirstByte()
			if err != nil {
				t.Fatalf("Failed to peek first byte: %v", err)
			}

			if firstByte != tt.expected {
				t.Errorf("Expected first byte 0x%02x, got 0x%02x", tt.expected, firstByte)
			}

			// Verify we can still read the full data after peeking
			data := make([]byte, len(tt.data))
			n, err := buffered.Read(data)
			if err != nil {
				t.Fatalf("Failed to read after peek: %v", err)
			}
			if n != len(tt.data) {
				t.Errorf("Expected to read %d bytes, got %d", len(tt.data), n)
			}
		})
	}
}
