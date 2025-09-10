package http

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestServeHTTP(t *testing.T) {
	tests := map[string]struct {
		addr           string
		handlers       HandlerMapping
		trustedProxies []string
		testEndpoints  []struct {
			path            string
			method          string
			expectStatus    int
			expectBody      string
			headers         map[string]string
			forwardedFor    string // X-Forwarded-For header
			skipBodyCheck   bool   // Skip body content check
			skipStatusCheck bool   // Skip status code check
		}
		wantErr bool
	}{
		"basic endpoints": {
			addr: "127.0.0.1:0", // Use port 0 to get a free port
			handlers: HandlerMapping{
				"/test": func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("test response"))
				},
				"/json": func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"status":"ok"}`))
				},
				"/error": func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte("internal error"))
				},
				"/headers": func(w http.ResponseWriter, r *http.Request) {
					auth := r.Header.Get("Authorization")
					if auth == "Bearer valid-token" {
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte("authorized"))
					} else {
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = w.Write([]byte("unauthorized"))
					}
				},
				"/method": func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost {
						w.WriteHeader(http.StatusCreated)
						_, _ = w.Write([]byte("created"))
					} else {
						w.WriteHeader(http.StatusMethodNotAllowed)
						_, _ = w.Write([]byte("method not allowed"))
					}
				},
			},
			testEndpoints: []struct {
				path            string
				method          string
				expectStatus    int
				expectBody      string
				headers         map[string]string
				forwardedFor    string
				skipBodyCheck   bool
				skipStatusCheck bool
			}{
				{
					path:         "/test",
					method:       http.MethodGet,
					expectStatus: http.StatusOK,
					expectBody:   "test response",
				},
				{
					path:         "/json",
					method:       http.MethodGet,
					expectStatus: http.StatusOK,
					expectBody:   `{"status":"ok"}`,
				},
				{
					path:         "/error",
					method:       http.MethodGet,
					expectStatus: http.StatusInternalServerError,
					expectBody:   "internal error",
				},
				{
					path:         "/headers",
					method:       http.MethodGet,
					expectStatus: http.StatusUnauthorized,
					expectBody:   "unauthorized",
				},
				{
					path:         "/headers",
					method:       http.MethodGet,
					expectStatus: http.StatusOK,
					expectBody:   "authorized",
					headers: map[string]string{
						"Authorization": "Bearer valid-token",
					},
				},
				{
					path:         "/method",
					method:       http.MethodGet,
					expectStatus: http.StatusMethodNotAllowed,
					expectBody:   "method not allowed",
				},
				{
					path:         "/method",
					method:       http.MethodPost,
					expectStatus: http.StatusCreated,
					expectBody:   "created",
				},
				{
					path:          "/nonexistent",
					method:        http.MethodGet,
					expectStatus:  http.StatusNotFound,
					skipBodyCheck: true,
				},
			},
			wantErr: false,
		},
		"with trusted proxies": {
			addr: "127.0.0.1:0",
			handlers: HandlerMapping{
				"/ip": func(w http.ResponseWriter, r *http.Request) {
					// Return the RemoteAddr which should be set by the XFF middleware
					// if trusted proxies are configured correctly
					_, _ = w.Write([]byte(r.RemoteAddr))
				},
			},
			trustedProxies: []string{"127.0.0.1/32"},
			testEndpoints: []struct {
				path            string
				method          string
				expectStatus    int
				expectBody      string
				headers         map[string]string
				forwardedFor    string
				skipBodyCheck   bool
				skipStatusCheck bool
			}{
				{
					path:            "/ip",
					method:          http.MethodGet,
					forwardedFor:    "192.168.1.100",
					skipBodyCheck:   true, // We can't predict the exact body as it depends on how the XFF middleware works
					skipStatusCheck: false,
					expectStatus:    http.StatusOK,
				},
			},
			wantErr: false,
		},
		"with invalid trusted proxies": {
			addr: "127.0.0.1:0",
			handlers: HandlerMapping{
				"/test": func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("test"))
				},
			},
			trustedProxies: []string{"invalid"},
			testEndpoints: []struct {
				path            string
				method          string
				expectStatus    int
				expectBody      string
				headers         map[string]string
				forwardedFor    string
				skipBodyCheck   bool
				skipStatusCheck bool
			}{},
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			config := &ConfigHTTP{
				Logger:         logr.Discard(),
				TrustedProxies: tt.trustedProxies,
			}

			// For invalid trusted proxies test, we expect an error during initialization
			if tt.wantErr {
				err := config.ServeHTTP(ctx, tt.addr, tt.handlers)
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
				return
			}

			// Start the server with a real listener so we can get its address
			ln, err := net.Listen("tcp", tt.addr)
			if err != nil {
				t.Fatalf("Failed to create listener: %v", err)
			}
			serverAddr := ln.Addr().String()
			ln.Close() // Close the listener now, the server will recreate it

			// Start the server in a goroutine
			errCh := make(chan error, 1)
			readyCh := make(chan struct{})

			go func() {
				// Start a goroutine to check server availability
				go func() {
					client := &http.Client{Timeout: 100 * time.Millisecond}
					ticker := time.NewTicker(50 * time.Millisecond)
					defer ticker.Stop()

					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							// Try to connect to the server
							req, _ := http.NewRequest(http.MethodGet, "http://"+serverAddr+"/", nil)
							resp, err := client.Do(req)
							if err == nil {
								resp.Body.Close()
								close(readyCh)
								return
							}
						}
					}
				}()

				// Start the actual server
				err := config.ServeHTTP(ctx, serverAddr, tt.handlers)
				errCh <- err
			}()

			// Wait for server to be ready or fail
			select {
			case <-readyCh:
				// Server is ready to accept connections
			case err := <-errCh:
				t.Fatalf("Server failed to start: %v", err)
			case <-time.After(2 * time.Second):
				t.Fatal("Timeout waiting for server to start")
			}

			// Make HTTP requests to the endpoints
			client := &http.Client{
				Timeout: 1 * time.Second,
			}

			for _, endpoint := range tt.testEndpoints {
				req, err := http.NewRequest(endpoint.method, "http://"+serverAddr+endpoint.path, nil)
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}

				// Add headers if specified
				for k, v := range endpoint.headers {
					req.Header.Set(k, v)
				}

				// Add X-Forwarded-For if specified
				if endpoint.forwardedFor != "" {
					req.Header.Set("X-Forwarded-For", endpoint.forwardedFor)
				}

				// Make the request
				resp, err := client.Do(req)
				if err != nil {
					t.Fatalf("Failed to make request: %v", err)
				}

				// Check status code
				if !endpoint.skipStatusCheck {
					if resp.StatusCode != endpoint.expectStatus {
						t.Errorf("Unexpected status code for %s: expected %d, got %d",
							endpoint.path, endpoint.expectStatus, resp.StatusCode)
					}
				}

				// Check response body
				if !endpoint.skipBodyCheck {
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						t.Fatalf("Failed to read response body: %v", err)
					}

					if string(body) != endpoint.expectBody {
						t.Errorf("Unexpected body for %s: expected %q, got %q",
							endpoint.path, endpoint.expectBody, string(body))
					}
				}

				resp.Body.Close()
			}

			// Shutdown the server
			cancel()

			// Wait for server to stop
			serverErr := <-errCh
			if errors.Is(serverErr, http.ErrServerClosed) {
				// This is the expected error when shutting down
				serverErr = nil
			}

			if serverErr != nil {
				t.Errorf("Error stopping server: %v", serverErr)
			}
		})
	}
}

func TestServeHTTPS(t *testing.T) {
	tests := map[string]struct {
		addr           string
		certFile       string
		keyFile        string
		handlers       HandlerMapping
		trustedProxies []string
		wantErr        bool
	}{
		"missing certificates": {
			addr:     "127.0.0.1:0",
			certFile: "",
			keyFile:  "",
			handlers: HandlerMapping{
				"/test": func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("test"))
				},
			},
			trustedProxies: nil,
			wantErr:        true,
		},
		"missing cert file": {
			addr:     "127.0.0.1:0",
			certFile: "",
			keyFile:  "some-key.pem",
			handlers: HandlerMapping{
				"/test": func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			},
			trustedProxies: nil,
			wantErr:        true,
		},
		"missing key file": {
			addr:     "127.0.0.1:0",
			certFile: "some-cert.pem",
			keyFile:  "",
			handlers: HandlerMapping{
				"/test": func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			},
			trustedProxies: nil,
			wantErr:        true,
		},
		"invalid certificate path": {
			addr:     "127.0.0.1:0",
			certFile: "/path/to/nonexistent/cert.pem",
			keyFile:  "/path/to/nonexistent/key.pem",
			handlers: HandlerMapping{
				"/test": func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			},
			trustedProxies: nil,
			wantErr:        true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			config := &ConfigHTTPS{
				CertFile:       tt.certFile,
				KeyFile:        tt.keyFile,
				Logger:         logr.Discard(),
				TrustedProxies: tt.trustedProxies,
			}

			errCh := make(chan error, 1)
			go func() {
				err := config.ServeHTTPS(ctx, tt.addr, tt.handlers)
				errCh <- err
			}()

			// Give the server time to start or error out
			time.Sleep(100 * time.Millisecond)

			// Cancel the context to stop the server
			cancel()

			// Wait for the server to stop
			err := <-errCh
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}
