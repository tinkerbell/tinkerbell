package http

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

// Protocol represents the supported protocols
type Protocol int

const (
	// ProtocolHTTP represents HTTP traffic
	ProtocolHTTP Protocol = iota
	// ProtocolHTTPS represents HTTPS traffic
	ProtocolHTTPS
	// ProtocolBoth represents both HTTP and HTTPS
	ProtocolBoth
)

// Multiplexer handles protocol-level multiplexing between HTTP and HTTPS
type Multiplexer struct {
	tlsConfig  *tls.Config
	httpServer *http.Server
	log        logr.Logger
}

type MultiplexerOption func(*Multiplexer)

func WithTLSConfig(c *tls.Config) MultiplexerOption {
	return func(m *Multiplexer) {
		m.tlsConfig = c
	}
}

func WithLogger(l logr.Logger) MultiplexerOption {
	return func(m *Multiplexer) {
		m.log = l
	}
}

func WithHTTPServer(s *http.Server) MultiplexerOption {
	return func(m *Multiplexer) {
		m.httpServer = s
	}
}

// NewMultiplexer creates a new protocol multiplexer
func NewMultiplexer(opts ...MultiplexerOption) (*Multiplexer, error) {
	m := &Multiplexer{}
	for _, opt := range opts {
		opt(m)
	}

	// There must be at least one of HTTPServer or TLSConfig
	if m.httpServer == nil && m.tlsConfig == nil {
		return nil, fmt.Errorf("at least one of HTTPServer or TLSConfig must be provided")
	}

	return m, nil
}

// ListenAndServe starts the multiplexer on the specified address
func (m *Multiplexer) ListenAndServe(ctx context.Context, addr string) error {
	return m.listenAndServe(ctx, addr, ProtocolHTTP)
}

// ListenAndServeTLS starts the multiplexer with TLS-only on the specified address
func (m *Multiplexer) ListenAndServeTLS(ctx context.Context, addr string) error {
	if m.tlsConfig == nil {
		return fmt.Errorf("no TLS configuration provided")
	}
	return m.listenAndServe(ctx, addr, ProtocolHTTPS)
}

// ListenAndServeBoth starts the multiplexer with both HTTP and HTTPS on the specified address
func (m *Multiplexer) ListenAndServeBoth(ctx context.Context, addr string) error {
	if m.tlsConfig == nil {
		return fmt.Errorf("no TLS configuration provided for dual protocol serving")
	}
	return m.listenAndServe(ctx, addr, ProtocolBoth)
}

// listenAndServe handles the actual serving logic based on protocol
func (m *Multiplexer) listenAndServe(ctx context.Context, addr string, protocol Protocol) error {
	switch protocol {
	case ProtocolHTTP:
		return m.serveHTTPOnly(ctx, addr)
	case ProtocolHTTPS:
		return m.serveHTTPSOnly(ctx, addr)
	case ProtocolBoth:
		return m.serveDualProtocol(ctx, addr)
	default:
		return fmt.Errorf("unsupported protocol: %v", protocol)
	}
}

// serveHTTPOnly serves only HTTP traffic
func (m *Multiplexer) serveHTTPOnly(ctx context.Context, addr string) error {
	server := m.cloneHTTPServer()
	server.Addr = addr

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	m.log.V(1).Info("Starting HTTP server", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server error: %w", err)
	}
	return nil
}

// serveHTTPSOnly serves only HTTPS traffic
func (m *Multiplexer) serveHTTPSOnly(ctx context.Context, addr string) error {
	server := m.cloneHTTPServer()
	server.Addr = addr
	server.TLSConfig = m.tlsConfig

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	m.log.V(1).Info("Starting HTTPS server", "addr", addr)
	if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTPS server error: %w", err)
	}
	return nil
}

// serveDualProtocol serves both HTTP and HTTPS on the same port using protocol detection
func (m *Multiplexer) serveDualProtocol(ctx context.Context, addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// Setup graceful shutdown
	go func() {
		<-ctx.Done()
		m.log.V(1).Info("Shutting down dual protocol server")
		listener.Close()
	}()

	m.log.V(1).Info("Starting dual protocol server", "addr", addr)

	// Accept connections and route based on protocol detection
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				// Check if it's a network error due to listener being closed
				if netErr, ok := err.(net.Error); ok && !netErr.Temporary() {
					return nil
				}
				m.log.Error(err, "Failed to accept connection")
				continue
			}
		}

		go m.handleConnection(ctx, conn)
	}
}

// handleConnection handles a single connection with protocol detection
func (m *Multiplexer) handleConnection(ctx context.Context, conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			m.log.Error(fmt.Errorf("panic: %v", r), "Recovered from panic in connection handler")
			conn.Close()
		}
	}()

	// Set a deadline for protocol detection
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Create a buffered connection to peek at the first byte
	bufferedConn := newBufferedConn(conn)

	// Peek at the first byte to detect protocol
	firstByte, err := bufferedConn.peekFirstByte()
	if err != nil {
		m.log.V(1).Info("Failed to peek first byte", "error", err)
		conn.Close()
		return
	}

	// Reset the read deadline
	conn.SetReadDeadline(time.Time{})

	// TLS handshake typically starts with 0x16 (22 in decimal)
	if firstByte == 0x16 && m.tlsConfig != nil {
		m.handleHTTPSConnection(ctx, bufferedConn)
	} else {
		m.handleHTTPConnection(ctx, bufferedConn)
	}
}

// handleHTTPConnection handles an HTTP connection
func (m *Multiplexer) handleHTTPConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	server := m.cloneHTTPServer()
	listener := newSingleConnListener(conn)
	defer listener.Close()

	m.log.V(1).Info("Handling HTTP connection", "remote", conn.RemoteAddr())
	server.Serve(listener)
}

// handleHTTPSConnection handles an HTTPS connection
func (m *Multiplexer) handleHTTPSConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// Wrap with TLS
	tlsConn := tls.Server(conn, m.tlsConfig)

	// Perform TLS handshake with timeout
	tlsConn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := tlsConn.Handshake(); err != nil {
		m.log.V(1).Info("TLS handshake failed", "error", err, "remote", conn.RemoteAddr())
		return
	}
	tlsConn.SetDeadline(time.Time{})

	server := m.cloneHTTPServer()
	server.TLSConfig = m.tlsConfig
	listener := newSingleConnListener(tlsConn)
	defer listener.Close()

	m.log.V(1).Info("Handling HTTPS connection", "remote", conn.RemoteAddr())
	server.Serve(listener)
}

// cloneHTTPServer creates a copy of the HTTP server for use in goroutines
func (m *Multiplexer) cloneHTTPServer() *http.Server {
	return &http.Server{
		Handler:           m.httpServer.Handler,
		ReadTimeout:       m.httpServer.ReadTimeout,
		ReadHeaderTimeout: m.httpServer.ReadHeaderTimeout,
		WriteTimeout:      m.httpServer.WriteTimeout,
		IdleTimeout:       m.httpServer.IdleTimeout,
		MaxHeaderBytes:    m.httpServer.MaxHeaderBytes,
		ErrorLog:          m.httpServer.ErrorLog,
		ConnState:         m.httpServer.ConnState,
		ConnContext:       m.httpServer.ConnContext,
	}
}
