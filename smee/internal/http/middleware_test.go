package http

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

func TestLogRequest(t *testing.T) {
	tests := map[string]struct {
		handler        http.Handler
		method         string
		requestURI     string
		remoteAddr     string
		tls            bool
		disableLogging bool
	}{
		"simple request": {
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
			method:     "GET",
			requestURI: "/test",
			remoteAddr: "192.168.1.1:1234",
			tls:        false,
		},
		"https request": {
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
			method:     "POST",
			requestURI: "/api/data",
			remoteAddr: "10.0.0.1:5678",
			tls:        true,
		},
		"disabled logging": {
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Global-Logging", "disabled")
				w.WriteHeader(http.StatusOK)
			}),
			method:         "GET",
			requestURI:     "/api/data",
			remoteAddr:     "10.0.0.1:5678",
			tls:            false,
			disableLogging: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Create middleware with our handler
			middleware := LogRequest(tt.handler, logr.Discard())

			// Create request
			req := httptest.NewRequest(tt.method, tt.requestURI, nil)
			req.RequestURI = tt.requestURI
			req.RemoteAddr = tt.remoteAddr

			// Simulate TLS if needed
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}

			// Create response recorder
			recorder := httptest.NewRecorder()

			// Call middleware
			middleware.ServeHTTP(recorder, req)

			// Check response status
			if recorder.Code != http.StatusOK {
				t.Errorf("Expected status code %d, got %d", http.StatusOK, recorder.Code)
			}

			// Check if X-Global-Logging is set
			if tt.disableLogging {
				if header := recorder.Header().Get("X-Global-Logging"); header != "disabled" {
					t.Errorf("Expected X-Global-Logging header to be 'disabled', got %q", header)
				}
			}
		})
	}
}

func TestResponseWriter(t *testing.T) {
	tests := map[string]struct {
		writeData  string
		statusCode int
	}{
		"default status code": {
			writeData:  "test data",
			statusCode: 0, // Will default to 200
		},
		"custom status code": {
			writeData:  "error data",
			statusCode: http.StatusBadRequest,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Create a recorder to capture output
			rec := httptest.NewRecorder()

			// Create our response writer wrapper
			rw := &responseWriter{ResponseWriter: rec}

			// Set status code if provided
			if tt.statusCode > 0 {
				rw.WriteHeader(tt.statusCode)
			}

			// Write data
			n, err := rw.Write([]byte(tt.writeData))
			if err != nil {
				t.Errorf("Unexpected error when writing: %v", err)
			}
			if n != len(tt.writeData) {
				t.Errorf("Expected to write %d bytes, but wrote %d", len(tt.writeData), n)
			}

			// Check status code
			expectedStatus := tt.statusCode
			if expectedStatus == 0 {
				expectedStatus = http.StatusOK
			}
			if rw.statusCode != expectedStatus {
				t.Errorf("Expected status code %d, got %d", expectedStatus, rw.statusCode)
			}

			// Check written data
			if rec.Body.String() != tt.writeData {
				t.Errorf("Expected body %q, got %q", tt.writeData, rec.Body.String())
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	type testCase struct {
		addr     string
		expected string
	}

	tests := map[string]testCase{
		"valid address": {
			addr:     "192.168.1.1:8080",
			expected: "192.168.1.1",
		},
		"ipv6 address": {
			addr:     "[2001:db8::1]:8080",
			expected: "2001:db8::1",
		},
		"invalid address": {
			addr:     "192.168.1.1", // Missing port
			expected: "?",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := clientIP(tt.addr)
			if result != tt.expected {
				t.Errorf("Expected client IP %q, got %q", tt.expected, result)
			}
		})
	}
}
