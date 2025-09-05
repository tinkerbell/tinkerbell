package http

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

func TestProtocolCheckMiddleware(t *testing.T) {
	// Create test handler that returns 200 OK with a specific message
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test handler executed"))
	})

	// Create config
	cfg := &Config{
		Logger: logr.Discard(),
	}

	testCases := []struct {
		name            string
		useTLS          bool
		allowedProtocol Protocol
		expectedStatus  int
	}{
		// HTTP tests
		{
			name:            "HTTP request allowed on HTTP-only handler",
			useTLS:          false, // HTTP request
			allowedProtocol: ProtocolHTTP,
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "HTTP request allowed on dual protocol handler",
			useTLS:          false, // HTTP request
			allowedProtocol: ProtocolBoth,
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "HTTP request not allowed on HTTPS-only handler",
			useTLS:          false, // HTTP request
			allowedProtocol: ProtocolHTTPS,
			expectedStatus:  http.StatusNotFound, // Should get 404
		},

		// HTTPS tests
		{
			name:            "HTTPS request allowed on HTTPS-only handler",
			useTLS:          true, // HTTPS request
			allowedProtocol: ProtocolHTTPS,
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "HTTPS request allowed on dual protocol handler",
			useTLS:          true, // HTTPS request
			allowedProtocol: ProtocolBoth,
			expectedStatus:  http.StatusOK,
		},
		{
			name:            "HTTPS request not allowed on HTTP-only handler",
			useTLS:          true, // HTTPS request
			allowedProtocol: ProtocolHTTP,
			expectedStatus:  http.StatusNotFound, // Should get 404
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create middleware-wrapped handler
			handler := cfg.protocolCheckMiddleware(testHandler, tc.allowedProtocol)

			// Create test request
			req := httptest.NewRequest("GET", "/test-path", nil)

			// Set TLS connection state if needed
			if tc.useTLS {
				req.TLS = &tls.ConnectionState{
					Version:           tls.VersionTLS12,
					HandshakeComplete: true,
				}
			}

			// Record response
			rec := httptest.NewRecorder()

			// Execute handler
			handler.ServeHTTP(rec, req)

			// Verify status code
			if rec.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, rec.Code)
			}

			// For successful responses, verify content
			if tc.expectedStatus == http.StatusOK {
				if rec.Body.String() != "test handler executed" {
					t.Errorf("Expected 'test handler executed', got '%s'", rec.Body.String())
				}
			}
		})
	}
}

// No need for a custom logger implementation, using logr.Discard() instead
