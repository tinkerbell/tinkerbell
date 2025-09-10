package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestHealthCheckHandlerFunc(t *testing.T) {
	tests := map[string]struct {
		gitRev   string
		wantCode int
	}{
		"successful response": {
			gitRev:   "abc123",
			wantCode: http.StatusOK,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Create a healthcheck with a known start time
			startTime := time.Now().Add(-10 * time.Second) // 10 seconds ago
			healthCheck := HealthCheck{
				StartTime: startTime,
				GitRev:    tt.gitRev,
			}

			// Create a handler function
			handler := healthCheck.HandlerFunc(logr.Discard())

			// Create a request and response recorder
			req := httptest.NewRequest("GET", "/healthcheck", nil)
			recorder := httptest.NewRecorder()

			// Call the handler function
			handler(recorder, req)

			// Check response
			if recorder.Code != tt.wantCode {
				t.Errorf("Expected status code %d, got %d", tt.wantCode, recorder.Code)
			}
			if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
				t.Errorf("Expected Content-Type %q, got %q", "application/json", contentType)
			}

			// Parse the response
			var response struct {
				GitRev        string `json:"git_rev"`
				UptimeSeconds string `json:"uptime_seconds"`
				Goroutines    int    `json:"goroutines"`
			}
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			// Validate the response
			if response.GitRev != tt.gitRev {
				t.Errorf("Expected GitRev %q, got %q", tt.gitRev, response.GitRev)
			}
			if response.UptimeSeconds == "" {
				t.Error("UptimeSeconds should not be empty")
			}
			if response.Goroutines <= 0 {
				t.Errorf("Expected Goroutines > 0, got %d", response.Goroutines)
			}

			// Parse the uptime to verify it's in the expected range
			var uptimeSeconds float64
			_, err = fmt.Sscanf(response.UptimeSeconds, "%f", &uptimeSeconds)
			if err != nil {
				t.Fatalf("Failed to parse uptime %q: %v", response.UptimeSeconds, err)
			}
			if uptimeSeconds < 10.0 {
				t.Errorf("Expected uptime >= 10.0 seconds, got %f", uptimeSeconds)
			}
		})
	}
}
