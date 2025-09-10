package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
)

func TestCreateHandler(t *testing.T) {
	tests := map[string]struct {
		otelOperation  string
		trustedProxies []string
		handlers       HandlerMapping
		wantErr        bool
	}{
		"basic handlers": {
			otelOperation: "test",
			handlers: HandlerMapping{
				"/test": func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			},
			trustedProxies: nil,
			wantErr:        false,
		},
		"with health check": {
			otelOperation: "test",
			handlers: HandlerMapping{
				healthCheckURI: func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			},
			trustedProxies: nil,
			wantErr:        false,
		},
		"with metrics": {
			otelOperation: "test",
			handlers: HandlerMapping{
				metricsURI: func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			},
			trustedProxies: nil,
			wantErr:        false,
		},
		"with valid trusted proxies": {
			otelOperation: "test",
			handlers: HandlerMapping{
				"/test": func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			},
			trustedProxies: []string{"192.168.1.1/32"},
			wantErr:        false,
		},
		"with invalid trusted proxies": {
			otelOperation: "test",
			handlers: HandlerMapping{
				"/test": func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				},
			},
			trustedProxies: []string{"invalid"},
			wantErr:        true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			handler, err := createHandler(logr.Discard(), tt.otelOperation, tt.trustedProxies, tt.handlers)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if handler == nil {
				t.Fatal("Handler should not be nil")
			}

			// Test each handler
			for pattern := range tt.handlers {
				req := httptest.NewRequest("GET", pattern, nil)
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, req)
				if recorder.Code != http.StatusOK {
					t.Errorf("Expected status code %d, got %d for pattern %s", http.StatusOK, recorder.Code, pattern)
				}
			}
		})
	}
}

func TestHandlerMapping(t *testing.T) {
	handlers := HandlerMapping{
		"/path1": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"/path2": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		},
	}

	// Check the handlers map
	if len(handlers) != 2 {
		t.Errorf("Expected handlers length to be 2, got %d", len(handlers))
	}
	if handlers["/path1"] == nil {
		t.Error("Expected handler for /path1 to not be nil")
	}
	if handlers["/path2"] == nil {
		t.Error("Expected handler for /path2 to not be nil")
	}
}
