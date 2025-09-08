package http

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

// ConfigHTTP is the configuration for the http server.
type ConfigHTTP struct {
	Logger         logr.Logger
	TrustedProxies []string
}

// ConfigHTTPS is the configuration for the HTTPS server.
type ConfigHTTPS struct {
	CertFile       string
	KeyFile        string
	Logger         logr.Logger
	TrustedProxies []string
}

// ServeHTTP sets up all the HTTP routes using a stdlib mux and starts the http
// server, which will block. App functionality is instrumented in Prometheus and OpenTelemetry.
func (c *ConfigHTTP) ServeHTTP(ctx context.Context, addr string, handlers HandlerMapping) error {
	hdler, err := createHandler(c.Logger, "smee-http", c.TrustedProxies, handlers)
	if err != nil {
		return fmt.Errorf("failed to create new serve mux: %w", err)
	}

	server := http.Server{
		Addr:    addr,
		Handler: hdler,

		// Mitigate Slowloris attacks. 30 seconds is based on Apache's recommended 20-40
		// recommendation. Smee doesn't really have many headers so 20s should be plenty of time.
		// https://en.wikipedia.org/wiki/Slowloris_(computer_security)
		ReadHeaderTimeout: 20 * time.Second,
		ErrorLog:          slog.NewLogLogger(logr.ToSlogHandler(c.Logger), slog.Level(c.Logger.GetV())),
	}

	go func() {
		<-ctx.Done()
		c.Logger.Info("shutting down http server")
		_ = server.Shutdown(ctx)
	}()
	if err := server.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		c.Logger.Error(err, "listen and serve http")
		return err
	}

	return nil
}

// ServeHTTPS sets up all the HTTP routes using a stdlib mux and starts the https
// server, which will block. App functionality is instrumented in Prometheus and OpenTelemetry.
func (c *ConfigHTTPS) ServeHTTPS(ctx context.Context, addrPort string, handlers HandlerMapping) error {
	hdler, err := createHandler(c.Logger, "smee-https", c.TrustedProxies, handlers)
	if err != nil {
		return fmt.Errorf("failed to create new serve mux: %w", err)
	}

	// If CertFile and KeyFile are not provided, can't be read, or don't exist
	// return error
	if c.CertFile == "" || c.KeyFile == "" {
		return fmt.Errorf("CertFile and KeyFile are not provided")
	}

	// Load the certificates with extensive logging
	// This key must be of type RSA. iPXE does not support ECDSA keys.
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to load TLS certificates: %w", err)
	}

	// Define cipher suites with more permissive settings to help resolve handshake issues
	server := http.Server{
		Addr:    addrPort,
		Handler: hdler,

		// Mitigate Slowloris attacks. 30 seconds is based on Apache's recommended 20-40
		// recommendation. Smee doesn't really have many headers so 20s should be plenty of time.
		// https://en.wikipedia.org/wiki/Slowloris_(computer_security)
		ReadHeaderTimeout: 20 * time.Second,
		ErrorLog:          slog.NewLogLogger(logr.ToSlogHandler(c.Logger.WithValues("server", "https")), slog.Level(c.Logger.GetV())),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		},
	}

	go func() {
		<-ctx.Done()
		c.Logger.Info("shutting down https server")
		_ = server.Shutdown(ctx)
	}()
	// The empty CertFile and KeyFile is because we defined the certs in the server.TLSConfig above.
	if err := server.ListenAndServeTLS(c.CertFile, c.KeyFile); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		c.Logger.Error(err, "listen and serve https")
		return err
	}

	return nil
}
