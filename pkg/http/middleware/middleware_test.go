package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

func TestLogging_CapturesStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
	}{
		{
			name: "200 implicit",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintln(w, "ok")
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "201 explicit",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "404 explicit",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "double WriteHeader records first",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()
			mw := Logging(logger)
			wrapped := mw(tt.handler)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			wrapped.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestLogging_WithLogLevel(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("LogLevelNever suppresses logging", func(t *testing.T) {
		sink := &spySink{}
		logger := logr.New(sink)

		inner := WithLogLevel(LogLevelNever, okHandler)
		wrapped := Logging(logger)(inner)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		wrapped.ServeHTTP(rec, req)

		if sink.called {
			t.Error("expected logging to be suppressed for LogLevelNever")
		}
	})

	t.Run("LogLevelNever suppresses even with query string", func(t *testing.T) {
		sink := &spySink{}
		logger := logr.New(sink)

		inner := WithLogLevel(LogLevelNever, okHandler)
		wrapped := Logging(logger)(inner)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/metrics?format=prometheus", nil)
		wrapped.ServeHTTP(rec, req)

		if sink.called {
			t.Error("expected logging to be suppressed for LogLevelNever with query string")
		}
	})

	t.Run("LogLevelAlways logs at V0", func(t *testing.T) {
		sink := &spySink{}
		logger := logr.New(sink)

		inner := WithLogLevel(LogLevelAlways, okHandler)
		wrapped := Logging(logger)(inner)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/tootles/", nil)
		wrapped.ServeHTTP(rec, req)

		if !sink.called {
			t.Error("expected logging for LogLevelAlways")
		}
		if sink.lastLevel != 0 {
			t.Errorf("expected V-level 0, got %d", sink.lastLevel)
		}
	})

	t.Run("LogLevelDebug logs at V1", func(t *testing.T) {
		sink := &spySink{}
		logger := logr.New(sink)

		inner := WithLogLevel(LogLevelDebug, okHandler)
		wrapped := Logging(logger)(inner)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
		wrapped.ServeHTTP(rec, req)

		if !sink.called {
			t.Error("expected logging for LogLevelDebug")
		}
		if sink.lastLevel != 1 {
			t.Errorf("expected V-level 1, got %d", sink.lastLevel)
		}
	})

	t.Run("default (no wrapper) logs at V1", func(t *testing.T) {
		sink := &spySink{}
		logger := logr.New(sink)

		wrapped := Logging(logger)(okHandler)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		wrapped.ServeHTTP(rec, req)

		if !sink.called {
			t.Error("expected logging for unwrapped handler")
		}
		if sink.lastLevel != 1 {
			t.Errorf("expected V-level 1, got %d", sink.lastLevel)
		}
	})

	t.Run("5xx overrides to V0 regardless of configured level", func(t *testing.T) {
		sink := &spySink{}
		logger := logr.New(sink)

		errHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		inner := WithLogLevel(LogLevelDebug, errHandler)
		wrapped := Logging(logger)(inner)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		wrapped.ServeHTTP(rec, req)

		if !sink.called {
			t.Error("expected logging for 5xx response")
		}
		if sink.lastLevel != 0 {
			t.Errorf("expected V-level 0 for 5xx, got %d", sink.lastLevel)
		}
	})

	t.Run("5xx is logged even with LogLevelNever", func(t *testing.T) {
		sink := &spySink{}
		logger := logr.New(sink)

		errHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		})

		inner := WithLogLevel(LogLevelNever, errHandler)
		wrapped := Logging(logger)(inner)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		wrapped.ServeHTTP(rec, req)

		if !sink.called {
			t.Error("expected 5xx to be logged even with LogLevelNever")
		}
		if sink.lastLevel != 0 {
			t.Errorf("expected V-level 0 for 5xx, got %d", sink.lastLevel)
		}
	})
}

func TestRecovery_ReturnsFiveHundredOnPanic(t *testing.T) {
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	})

	mw := Recovery(logr.Discard())
	wrapped := mw(handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRecovery_NoPanic(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := Recovery(logr.Discard())
	wrapped := mw(handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequestMetrics_RecordsMetrics(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	mw := RequestMetrics()
	wrapped := mw(handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusAccepted)
	}

	// Metrics are registered on the default Prometheus registry via promauto.
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	var foundCount, foundDuration bool
	for _, f := range families {
		switch f.GetName() {
		case "http_server_requests_total":
			foundCount = true
			if len(f.GetMetric()) == 0 {
				t.Error("http_server_requests_total has no metric points")
			}
		case "http_server_request_duration_seconds":
			foundDuration = true
			if len(f.GetMetric()) == 0 {
				t.Error("http_server_request_duration_seconds has no metric points")
			}
		}
	}
	if !foundCount {
		t.Error("http_server_requests_total metric not found")
	}
	if !foundDuration {
		t.Error("http_server_request_duration_seconds metric not found")
	}
}

func TestXFF_EmptyProxies(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw, err := XFF(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wrapped := mw(handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.1:8080", "192.168.1.1"},
		{"[::1]:8080", "::1"},
		{"invalid", "invalid"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := clientIP(tt.input)
			if got != tt.want {
				t.Errorf("clientIP(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLogging_SkipWithQueryString(t *testing.T) {
	// Verify that WithLogLevel(LogLevelNever, ...) suppresses logging
	// even when the request has a query string.
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	sink := &spySink{}
	logger := logr.New(sink)

	inner := WithLogLevel(LogLevelNever, handler)
	wrapped := Logging(logger)(inner)

	// Request with query string — should be skipped.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics?format=prometheus", nil)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if sink.called {
		t.Error("expected logging to be skipped for /metrics?format=prometheus")
	}

	// Request without query string — should also be skipped.
	sink.called = false
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
	wrapped.ServeHTTP(rec, req)

	if sink.called {
		t.Error("expected logging to be skipped for /healthcheck")
	}

	// Request NOT wrapped with LogLevelNever — should be logged.
	sink.called = false
	notSkipped := Logging(logger)(handler)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	notSkipped.ServeHTTP(rec, req)

	if !sink.called {
		t.Error("expected logging for /api/test")
	}
}

// spySink is a minimal logr.LogSink that records whether Info was called
// and at what V-level.
type spySink struct {
	called    bool
	lastLevel int
}

func (s *spySink) Init(logr.RuntimeInfo)              {}
func (s *spySink) Enabled(int) bool                   { return true }
func (s *spySink) Info(level int, _ string, _ ...any) { s.called = true; s.lastLevel = level }
func (s *spySink) Error(_ error, _ string, _ ...any)  { s.called = true }
func (s *spySink) WithValues(_ ...any) logr.LogSink   { return s }
func (s *spySink) WithName(_ string) logr.LogSink     { return s }

func TestHTTPSnoopPreservesInterfaces(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Error("expected http.Flusher to be preserved through httpsnoop")
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := Logging(logr.Discard())
	wrapped := mw(handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	wrapped.ServeHTTP(rec, req)
}
