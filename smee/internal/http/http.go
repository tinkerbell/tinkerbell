// package http is the http server for smee.
package http

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tinkerbell/tinkerbell/pkg/xff"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Config is the configuration for the http server.
type Config struct {
	HealthCheck    HealthCheck
	Logger         logr.Logger
	TrustedProxies []string
	// TLS configuration
	EnableTLS bool
	CertFile  string
	KeyFile   string
}

// HandlerConfig represents a handler with protocol specification
type HandlerConfig struct {
	Pattern   string
	Handler   http.HandlerFunc
	Protocols Protocol // HTTP, HTTPS, or Both
}

// HandlerMapping is a slice of handler configurations.
type HandlerMapping []HandlerConfig

// HealthCheck contains health check configuration
type HealthCheck struct {
	StartTime time.Time
	GitRev    string
}

// ServeHTTP sets up all the HTTP routes using the new multiplexer and starts the server.
// App functionality is instrumented in Prometheus and OpenTelemetry.
func (s *Config) ServeHTTP(ctx context.Context, addr string, handlers HandlerMapping) error {
	// Create the HTTP server with routes and middleware
	httpServer := s.createHTTPServer(handlers)

	// Create multiplexer config
	multiplexerConfig := []MultiplexerOption{
		WithHTTPServer(httpServer),
		WithLogger(s.Logger),
	}

	// Add TLS config if enabled
	if s.EnableTLS {
		// If CertFile and KeyFile are not provided, can't be read, or don't exist
		// return error
		if s.CertFile == "" || s.KeyFile == "" {
			return fmt.Errorf("TLS is enabled but CertFile and KeyFile are not provided")
		}
		// Load the certificates
		cert, err := tls.LoadX509KeyPair(s.CertFile, s.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS certificates: %w", err)
		}

		// Add TLS config to multiplexer options
		multiplexerConfig = append(multiplexerConfig, WithTLSConfig(&tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}))
	}

	// Create the protocol multiplexer
	multiplexer, err := NewMultiplexer(multiplexerConfig...)
	if err != nil {
		return fmt.Errorf("failed to create protocol multiplexer: %w", err)
	}

	// Determine which protocols to serve
	hasHTTP, hasHTTPS := s.analyzeProtocols(handlers)

	// Start the appropriate server based on protocols needed
	switch {
	case hasHTTP && hasHTTPS && s.EnableTLS:
		// Serve both protocols on the same port
		s.Logger.Info("Starting dual protocol server", "addr", addr, "protocols", ProtocolBoth.String())
		err = multiplexer.ListenAndServeBoth(ctx, addr)
	case hasHTTP && !hasHTTPS:
		// Serve only HTTPS
		s.Logger.Info("Starting HTTPS server", "addr", addr, "protocol", ProtocolHTTPS.String())
		err = multiplexer.ListenAndServeTLS(ctx, addr)
	case !hasHTTP && hasHTTPS:
		// Serve only HTTP (default)
		s.Logger.Info("Starting HTTP server", "addr", addr, "protocol", ProtocolHTTP.String())
		err = multiplexer.ListenAndServe(ctx, addr)
	}

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		s.Logger.Error(err, "multiplexer server error")
		return err
	}

	return nil
}

// createHTTPServer creates and configures the HTTP server with all routes and middleware
func (s *Config) createHTTPServer(handlers HandlerMapping) *http.Server {
	// Create the main HTTP mux
	httpMux := http.NewServeMux()

	// Add common handlers (metrics, health check)
	s.addCommonHandlers(httpMux)

	// Add user-provided handlers with middleware
	s.addUserHandlers(httpMux, handlers)

	// Create the HTTP server
	return &http.Server{
		Handler:           s.wrapWithMiddleware(httpMux),
		ReadHeaderTimeout: 20 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logr.ToSlogHandler(s.Logger), slog.Level(s.Logger.GetV())),
	}
}

// addCommonHandlers adds system handlers like metrics and health checks
func (s *Config) addCommonHandlers(mux *http.ServeMux) {
	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Health check endpoint
	mux.HandleFunc("/healthcheck", s.HealthCheck.serveHealthchecker(s.Logger))
}

// addUserHandlers adds user-provided handlers to the mux
func (s *Config) addUserHandlers(mux *http.ServeMux, handlers HandlerMapping) {
	for _, hc := range handlers {
		// Wrap the handler with OpenTelemetry
		_, otelHandler := otelFuncWrapper(hc.Pattern, hc.Handler)

		// Wrap with protocol-checking middleware
		protocolHandler := s.protocolCheckMiddleware(otelHandler, hc.Protocols)

		// Register the handler
		mux.Handle(hc.Pattern, protocolHandler)

		s.Logger.V(1).Info("Registered handler",
			"pattern", hc.Pattern,
			"protocols", hc.Protocols.String())
	}
}

// protocolCheckMiddleware creates middleware that ensures requests are only handled
// if the protocol (HTTP/HTTPS) matches the handler's protocol specification
func (s *Config) protocolCheckMiddleware(next http.Handler, allowedProtocol Protocol) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isTLS := r.TLS != nil
		requestProto := ProtocolHTTP
		if isTLS {
			requestProto = ProtocolHTTPS
		}

		// Check if this request's protocol is allowed for this handler
		allowed := false
		switch allowedProtocol {
		case ProtocolBoth:
			allowed = true
		case ProtocolHTTP:
			allowed = !isTLS
		case ProtocolHTTPS:
			allowed = isTLS
		}

		if !allowed {
			// Log the protocol mismatch
			s.Logger.V(1).Info("Protocol mismatch",
				"path", r.URL.Path,
				"requestProtocol", requestProto,
				"allowedProtocol", allowedProtocol)

			// Return 404 not found for security reasons (instead of 403 forbidden)
			// This avoids revealing the existence of routes that are available on other protocols
			http.NotFound(w, r)
			return
		}

		// Protocol is allowed, proceed with the request
		next.ServeHTTP(w, r)
	})
}

// wrapWithMiddleware wraps the main handler with middleware chain
func (s *Config) wrapWithMiddleware(handler http.Handler) http.Handler {
	// Wrap with OpenTelemetry
	otelHandler := otelhttp.NewHandler(handler, "smee-http")

	// Add X-Forwarded-For support if trusted proxies are configured
	if len(s.TrustedProxies) > 0 {
		xffmw, err := xff.NewXFF(xff.Options{
			AllowedSubnets: s.TrustedProxies,
		})
		if err != nil {
			s.Logger.Error(err, "failed to create new xff object")
			return &loggingMiddleware{
				handler: otelHandler,
				log:     s.Logger,
			}
		}

		return xffmw.Handler(&loggingMiddleware{
			handler: otelHandler,
			log:     s.Logger,
		})
	}

	return &loggingMiddleware{
		handler: otelHandler,
		log:     s.Logger,
	}
}

// analyzeProtocols determines which protocols are needed based on handler configurations
func (s *Config) analyzeProtocols(handlers HandlerMapping) (hasHTTP, hasHTTPS bool) {
	for _, hc := range handlers {
		switch hc.Protocols {
		case ProtocolHTTP:
			hasHTTP = true
		case ProtocolHTTPS:
			hasHTTPS = true
		case ProtocolBoth:
			hasHTTP = true
			hasHTTPS = true
		}
	}

	// Common handlers are available on both protocols if TLS is enabled
	if s.EnableTLS {
		hasHTTPS = true
	}

	// Always enable HTTP unless all handlers are HTTPS-only and TLS is enabled.
	// This ensures HTTP is available for endpoints unless the configuration is strictly HTTPS.
	if !hasHTTPS {
		hasHTTP = true
	}

	return hasHTTP, hasHTTPS
}

func (h *HealthCheck) serveHealthchecker(log logr.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		res := struct {
			GitRev        string `json:"git_rev"`
			UptimeSeconds string `json:"uptime_seconds"`
			Goroutines    int    `json:"goroutines"`
		}{
			GitRev:        h.GitRev,
			UptimeSeconds: fmt.Sprintf("%.2f", time.Since(h.StartTime).Seconds()),
			Goroutines:    runtime.NumGoroutine(),
		}

		if err := json.NewEncoder(w).Encode(&res); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Error(err, "marshaling healthcheck json")
		}
	}
}

// otelFuncWrapper takes a route and an http handler function, wraps the function
// with otelhttp, and returns the route again and http.Handler all set for Handle().
func otelFuncWrapper(route string, h func(w http.ResponseWriter, req *http.Request)) (string, http.Handler) {
	return route, otelhttp.WithRouteTag(route, http.HandlerFunc(h))
}
