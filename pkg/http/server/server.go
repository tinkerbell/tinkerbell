// Package server provides an HTTP/HTTPS server for Tinkerbell.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"
)

const (
	// DefaultReadTimeout is the maximum duration for reading the entire request.
	DefaultReadTimeout = 30 * time.Second
	// DefaultReadHeaderTimeout is the maximum duration for reading request headers.
	DefaultReadHeaderTimeout = 10 * time.Second
	// DefaultWriteTimeout is the maximum duration before timing out writes of the response.
	DefaultWriteTimeout = 30 * time.Second
	// DefaultIdleTimeout is the maximum duration for keep-alive connections.
	DefaultIdleTimeout = 120 * time.Second
	// DefaultShutdownTimeout is the maximum duration for graceful shutdown.
	DefaultShutdownTimeout = 30 * time.Second
	// DefaultMaxHeaderBytes is the maximum size of request headers.
	DefaultMaxHeaderBytes = 1 << 20 // 1 MB
)

// Config is the configuration for the HTTP/HTTPS server.
type Config struct {
	// BindAddr is the IP address to bind to.
	BindAddr string
	// BindPort is the port for the HTTP server.
	BindPort int
	// TLSCerts are in-memory TLS certificates. Must be provided to enable the HTTPS server.
	TLSCerts []tls.Certificate
	// HTTPSPort is the optional port for an HTTPS server.
	HTTPSPort int
	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration
	// ReadHeaderTimeout is the maximum duration for reading request headers.
	ReadHeaderTimeout time.Duration
	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration
	// IdleTimeout is the maximum duration for keep-alive connections.
	IdleTimeout time.Duration
	// MaxHeaderBytes is the maximum size of request headers.
	MaxHeaderBytes int
	// ShutdownTimeout is the maximum duration for graceful shutdown.
	ShutdownTimeout time.Duration
}

// Option configures a Config.
type Option func(*Config)

// NewConfig returns a Config with sensible defaults, modified by the given options.
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		ReadTimeout:       DefaultReadTimeout,
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		WriteTimeout:      DefaultWriteTimeout,
		IdleTimeout:       DefaultIdleTimeout,
		MaxHeaderBytes:    DefaultMaxHeaderBytes,
		ShutdownTimeout:   DefaultShutdownTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

func (c *Config) setDefaults() {
	if c.ReadTimeout == 0 {
		c.ReadTimeout = DefaultReadTimeout
	}
	if c.ReadHeaderTimeout == 0 {
		c.ReadHeaderTimeout = DefaultReadHeaderTimeout
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = DefaultWriteTimeout
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = DefaultIdleTimeout
	}
	if c.MaxHeaderBytes == 0 {
		c.MaxHeaderBytes = DefaultMaxHeaderBytes
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = DefaultShutdownTimeout
	}
}

// Serve starts the HTTP server (and optionally an HTTPS server) and blocks until ctx is cancelled.
// It performs a graceful shutdown when ctx is cancelled.
func (c *Config) Serve(ctx context.Context, log logr.Logger, httpHandler http.Handler, httpsHandler http.Handler) error {
	c.setDefaults()
	g, ctx := errgroup.WithContext(ctx)

	// HTTP server
	g.Go(func() error {
		if httpHandler == nil {
			log.Info("no HTTP handler, skipping HTTP server")
			return nil
		}
		httpAddr := fmt.Sprintf("%s:%d", c.BindAddr, c.BindPort)

		return c.doServe(ctx, log, httpAddr, httpHandler, nil)
	})

	// HTTPS server (optional)
	if len(c.TLSCerts) > 0 {
		if httpsHandler == nil {
			log.Info("no HTTPS handler, skipping HTTPS server")
			return nil
		}
		httpsAddr := fmt.Sprintf("%s:%d", c.BindAddr, c.HTTPSPort)
		tlsCfg := &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: c.TLSCerts,
		}
		g.Go(func() error {
			return c.doServe(ctx, log.WithValues("server", "https"), httpsAddr, httpsHandler, tlsCfg)
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("http server error: %w", err)
	}

	return nil
}

func (c *Config) doServe(ctx context.Context, log logr.Logger, addr string, handler http.Handler, tlsCfg *tls.Config) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       c.ReadTimeout,
		ReadHeaderTimeout: c.ReadHeaderTimeout,
		WriteTimeout:      c.WriteTimeout,
		IdleTimeout:       c.IdleTimeout,
		MaxHeaderBytes:    c.MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logr.ToSlogHandler(log), slog.Level(log.GetV())),
		TLSConfig:         tlsCfg,
	}

	errCh := make(chan error, 1)
	go func() {
		var err error
		if tlsCfg != nil {
			// Use in-memory certificates from TLSConfig.
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down http server", "addr", addr)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			server.Close()
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timed out waiting for graceful shutdown: %w", err)
			}
			return fmt.Errorf("server shutdown error: %w", err)
		}
		return nil
	}
}
