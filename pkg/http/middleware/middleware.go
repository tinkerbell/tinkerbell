// Package middleware provides common HTTP middleware for the Tinkerbell HTTP server.
package middleware

import (
	"context"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"

	"github.com/felixge/httpsnoop"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tinkerbell/tinkerbell/pkg/xff"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Log-level constants for use with [WithLogLevel].
const (
	// LogLevelNever suppresses all logging for a handler (except 5xx responses).
	LogLevelNever = -1
	// LogLevelAlways logs every request at V(0), useful for low-noise
	// endpoints where visibility matters (e.g. cloud-init / Tootles).
	LogLevelAlways = 0
	// LogLevelDebug logs requests at V(1), the default for handlers that
	// don't specify a level. Suitable for high-noise routes like the web UI.
	LogLevelDebug = 1
)

// logLevelKey is the context key for the shared [logConfig].
type logLevelKeyType struct{}

var logLevelKey = logLevelKeyType{}

// sourceIPKeyType is the context key for the original remote address captured
// by the [SourceIP] middleware before XFF can modify r.RemoteAddr.
type sourceIPKeyType struct{}

var sourceIPKey = sourceIPKeyType{}

// logConfig holds the mutable log level for a request. The [Logging]
// middleware creates this and injects a pointer into the request context.
// [WithLogLevel] mutates the pointed-to value so the outer middleware sees
// the change after the inner handler returns.
type logConfig struct {
	level int
}

// WithLogLevel wraps handler so that the [Logging] middleware uses the given
// logr V-level for non-error responses. Use the LogLevel* constants.
//
//	LogLevelNever  (-1): suppress all logging, except 5xx responses
//	LogLevelAlways ( 0): log every request at V(0)
//	LogLevelDebug  ( 1): log at V(1), the default
//
// 5xx responses are always logged at V(0) regardless of the configured level.
func WithLogLevel(level int, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg, ok := r.Context().Value(logLevelKey).(*logConfig); ok {
			cfg.level = level
		}
		handler.ServeHTTP(w, r)
	})
}

// SourceIP returns middleware that captures the original r.RemoteAddr (the TCP
// connection IP) into the request context. This must be applied as the outermost
// middleware — before [XFF] — so that the value is recorded before any header-
// based rewriting of RemoteAddr. The [Logging] middleware reads this value to
// log it as "sourceIP".
func SourceIP() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(context.WithValue(r.Context(), sourceIPKey, clientIP(r.RemoteAddr)))
			next.ServeHTTP(w, r)
		})
	}
}

// XFF returns middleware that processes X-Forwarded-For headers.
// If trustedProxies is empty, the middleware is a no-op.
func XFF(trustedProxies []string) (func(http.Handler) http.Handler, error) {
	if len(trustedProxies) == 0 {
		return func(next http.Handler) http.Handler {
			return next
		}, nil
	}
	xffmw, err := xff.NewXFF(xff.Options{AllowedSubnets: trustedProxies})
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return xffmw.Handler(next)
	}, nil
}

// Logging returns middleware that logs HTTP requests using the provided logger.
// It logs method, URI, client IP, duration, and response status code.
//
// The log verbosity for each request is determined by the context value set via
// [WithLogLevel]. If no level is set the default is [LogLevelDebug] (V(1)).
// Responses with status >= 500 are always logged at V(0) regardless of the
// configured level. A level of [LogLevelNever] (-1) suppresses all output.
func Logging(logger logr.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := r.Method
			uri := r.RequestURI
			client := clientIP(r.RemoteAddr)
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}

			// sourceIP is the original TCP connection IP captured by the
			// SourceIP middleware before XFF rewrites r.RemoteAddr.
			sourceIP, _ := r.Context().Value(sourceIPKey).(string)

			// Inject a mutable logConfig so inner handlers wrapped with
			// WithLogLevel can set the level visible to this middleware.
			cfg := &logConfig{level: LogLevelDebug}
			r = r.WithContext(context.WithValue(r.Context(), logLevelKey, cfg))

			m := httpsnoop.CaptureMetrics(next, w, r)

			level := cfg.level
			// 5xx errors are always surfaced at V(0), even for LogLevelNever.
			if m.Code >= http.StatusInternalServerError {
				level = LogLevelAlways
			}
			if level == LogLevelNever {
				return
			}

			logger.V(level).Info("response", "scheme", scheme, "method", method, "uri", uri, "client", client, "sourceIP", sourceIP, "duration", m.Duration.String(), "code", m.Code)
		})
	}
}

// Recovery returns middleware that recovers from panics, logs the panic value
// and stack trace, and returns a 500 status code.
func Recovery(log logr.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error(nil, "panic recovered in HTTP handler", "panic", rec, "stack", string(debug.Stack()))
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// OTel returns middleware that wraps an http.Handler with OpenTelemetry instrumentation.
func OTel(operationName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, operationName)
	}
}

// RequestMetrics returns middleware that instruments HTTP requests with Prometheus metrics.
// Metrics are registered on the default Prometheus registry so they appear alongside
// application metrics (e.g. Smee DHCP counters) on the /metrics endpoint.
// It is safe to call more than once; metrics are registered exactly once.
func RequestMetrics() func(http.Handler) http.Handler {
	requestMetricsOnce.Do(func() {
		requestCount = promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_server_requests_total",
				Help: "Count of HTTP requests",
			},
			[]string{"method", "status_code"},
		)

		requestDuration = promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_server_request_duration_seconds",
				Help:    "Histogram of response time for HTTP requests in seconds",
				Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"route", "method", "status_code"},
		)
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m := httpsnoop.CaptureMetrics(next, w, r)

			status := strconv.Itoa(m.Code)
			requestCount.WithLabelValues(r.Method, status).Inc()
			// Use the registered mux pattern (Go 1.22+) instead of the raw
			// URL path to avoid unbounded cardinality from dynamic path
			// segments (instance IDs, MAC addresses, etc.).
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			requestDuration.WithLabelValues(route, r.Method, status).Observe(m.Duration.Seconds())
		})
	}
}

var (
	requestMetricsOnce sync.Once
	requestCount       *prometheus.CounterVec
	requestDuration    *prometheus.HistogramVec
)

func clientIP(str string) string {
	host, _, err := net.SplitHostPort(str)
	if err != nil {
		return str
	}
	return host
}
