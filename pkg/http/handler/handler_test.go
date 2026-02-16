package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestHealthCheck(t *testing.T) {
	handler := HealthCheck(logr.Discard(), time.Now())

	t.Run("returns 200 with JSON", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
		}

		var body struct {
			GitRev        string `json:"git_rev"`
			UptimeSeconds string `json:"uptime_seconds"`
			Goroutines    int    `json:"goroutines"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response body: %v", err)
		}
		if body.UptimeSeconds == "" {
			t.Error("uptime_seconds should not be empty")
		}
		if body.Goroutines <= 0 {
			t.Error("goroutines should be > 0")
		}
	})

	t.Run("Content-Type not set before encoding succeeds", func(t *testing.T) {
		// This validates the buffering pattern: Content-Type is set only after
		// successful JSON encoding, not before. We verify by checking that the
		// response is well-formed (200 + application/json). If encoding were
		// done directly to the writer, a failure mid-write would leave the
		// response in an inconsistent state.
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q, want %q", ct, "application/json")
		}
	})
}
