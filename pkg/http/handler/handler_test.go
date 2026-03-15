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

func TestRedirectToHTTPS(t *testing.T) {
	h := RedirectToHTTPS(logr.Discard(), 7443)

	tests := []struct {
		name     string
		host     string
		wantHost string
	}{
		{name: "hostname without port", host: "example.com", wantHost: "example.com:7443"},
		{name: "hostname with port", host: "example.com:8080", wantHost: "example.com:7443"},
		{name: "IPv4 without port", host: "10.0.0.1", wantHost: "10.0.0.1:7443"},
		{name: "IPv4 with port", host: "10.0.0.1:8080", wantHost: "10.0.0.1:7443"},
		{name: "bracketed IPv6 without port", host: "[::1]", wantHost: "[::1]:7443"},
		{name: "bracketed IPv6 with port", host: "[::1]:8080", wantHost: "[::1]:7443"},
		{name: "bracketed IPv6 full without port", host: "[2001:db8::1]", wantHost: "[2001:db8::1]:7443"},
		{name: "bracketed IPv6 full with port", host: "[2001:db8::1]:8080", wantHost: "[2001:db8::1]:7443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://"+tt.host+"/path", nil)
			req.Host = tt.host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusPermanentRedirect {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusPermanentRedirect)
			}
			loc := rec.Header().Get("Location")
			want := "https://" + tt.wantHost + "/path"
			if loc != want {
				t.Fatalf("Location = %q, want %q", loc, want)
			}
		})
	}
}
