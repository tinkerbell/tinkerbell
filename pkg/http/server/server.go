// Package server provides an HTTP/HTTPS server for Tinkerbell.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/listener"
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

// Listener is the address and ports for one IP family. An invalid Addr leaves
// that family unserved.
type Listener struct {
	Addr      netip.Addr
	HTTPPort  int
	HTTPSPort int
}

// Config is the configuration for the HTTP/HTTPS server.
type Config struct {
	// V4 and V6 are the per-family listen addresses and ports.
	V4 Listener
	V6 Listener
	// TLSCerts are in-memory TLS certificates. Must be provided to enable the HTTPS server.
	TLSCerts []tls.Certificate
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

// Serve starts an HTTP server, and optionally an HTTPS server, for every
// configured address family and blocks until ctx is cancelled. It performs a
// graceful shutdown when ctx is cancelled.
func (c *Config) Serve(ctx context.Context, log logr.Logger, httpHandler http.Handler, httpsHandler http.Handler) error {
	c.setDefaults()
	g, ctx := errgroup.WithContext(ctx)

	serveHTTPS := len(c.TLSCerts) > 0 && httpsHandler != nil
	var started int
	for _, l := range []Listener{c.V4, c.V6} {
		if !l.Addr.IsValid() {
			continue
		}
		if httpHandler != nil {
			g.Go(func() error {
				return c.doServe(ctx, log, l.Addr, l.HTTPPort, httpHandler, nil)
			})
			started++
		}
		if serveHTTPS {
			tlsCfg := &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: c.TLSCerts,
			}
			g.Go(func() error {
				return c.doServe(ctx, log.WithValues("server", "https"), l.Addr, l.HTTPSPort, httpsHandler, tlsCfg)
			})
			started++
		}
	}

	if started == 0 {
		log.Info("no HTTP listeners configured, skipping HTTP server")
		return nil
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("http server error: %w", err)
	}

	return nil
}

func (c *Config) doServe(ctx context.Context, log logr.Logger, bindAddr netip.Addr, port int, handler http.Handler, tlsCfg *tls.Config) error {
	l, err := listener.TCP(ctx, bindAddr, port)
	if err != nil {
		return err
	}
	addr := l.Addr().String()

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       c.ReadTimeout,
		ReadHeaderTimeout: c.ReadHeaderTimeout,
		WriteTimeout:      c.WriteTimeout,
		IdleTimeout:       c.IdleTimeout,
		MaxHeaderBytes:    c.MaxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logr.ToSlogHandler(log), slog.Level(-log.GetV())),
		TLSConfig:         tlsCfg,
	}

	errCh := make(chan error, 1)
	go func() {
		var err error
		if tlsCfg != nil {
			// Use in-memory certificates from TLSConfig.
			err = server.ServeTLS(l, "", "")
		} else {
			err = server.Serve(l)
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
			_ = server.Close()
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timed out waiting for graceful shutdown: %w", err)
			}
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("server shutdown error: %w", err)
		}
		return nil
	}
}
