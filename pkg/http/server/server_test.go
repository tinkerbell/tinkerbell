package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestNewConfig_Defaults(t *testing.T) {
	cfg := NewConfig()

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"ReadTimeout", cfg.ReadTimeout, DefaultReadTimeout},
		{"ReadHeaderTimeout", cfg.ReadHeaderTimeout, DefaultReadHeaderTimeout},
		{"WriteTimeout", cfg.WriteTimeout, DefaultWriteTimeout},
		{"IdleTimeout", cfg.IdleTimeout, DefaultIdleTimeout},
		{"MaxHeaderBytes", cfg.MaxHeaderBytes, DefaultMaxHeaderBytes},
		{"ShutdownTimeout", cfg.ShutdownTimeout, DefaultShutdownTimeout},
		{"BindAddr", cfg.BindAddr, ""},
		{"BindPort", cfg.BindPort, 0},
		{"HTTPSPort", cfg.HTTPSPort, 0},
	}
	for _, c := range checks {
		if fmt.Sprint(c.got) != fmt.Sprint(c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestNewConfig_WithOptions(t *testing.T) {
	cfg := NewConfig(
		func(c *Config) { c.BindAddr = "10.0.0.1" },
		func(c *Config) { c.BindPort = 9999 },
		func(c *Config) { c.ReadTimeout = 5 * time.Second },
	)

	if cfg.BindAddr != "10.0.0.1" {
		t.Errorf("BindAddr = %q, want %q", cfg.BindAddr, "10.0.0.1")
	}
	if cfg.BindPort != 9999 {
		t.Errorf("BindPort = %d, want %d", cfg.BindPort, 9999)
	}
	if cfg.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 5*time.Second)
	}
	// Non-overridden fields keep defaults.
	if cfg.WriteTimeout != DefaultWriteTimeout {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, DefaultWriteTimeout)
	}
}

func TestSetDefaults_FillsZeroValues(t *testing.T) {
	cfg := &Config{}
	cfg.setDefaults()

	if cfg.ReadTimeout != DefaultReadTimeout {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, DefaultReadTimeout)
	}
	if cfg.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, want %v", cfg.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	}
	if cfg.WriteTimeout != DefaultWriteTimeout {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, DefaultWriteTimeout)
	}
	if cfg.IdleTimeout != DefaultIdleTimeout {
		t.Errorf("IdleTimeout = %v, want %v", cfg.IdleTimeout, DefaultIdleTimeout)
	}
	if cfg.MaxHeaderBytes != DefaultMaxHeaderBytes {
		t.Errorf("MaxHeaderBytes = %v, want %v", cfg.MaxHeaderBytes, DefaultMaxHeaderBytes)
	}
	if cfg.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, DefaultShutdownTimeout)
	}
}

func TestSetDefaults_PreservesExistingValues(t *testing.T) {
	cfg := &Config{
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 2 * time.Second,
	}
	cfg.setDefaults()

	if cfg.ReadTimeout != 1*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 1*time.Second)
	}
	if cfg.WriteTimeout != 2*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 2*time.Second)
	}
	// Zero fields get defaults.
	if cfg.IdleTimeout != DefaultIdleTimeout {
		t.Errorf("IdleTimeout = %v, want %v", cfg.IdleTimeout, DefaultIdleTimeout)
	}
}

// freePort finds an available TCP port on localhost.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// waitForPort polls until a TCP connection to addr succeeds or the timeout expires.
func waitForPort(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s did not start within %v", addr, timeout)
}

func TestServe_HTTPOnly(t *testing.T) {
	port := freePort(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	cfg := NewConfig(func(c *Config) {
		c.BindAddr = "127.0.0.1"
		c.BindPort = port
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- cfg.Serve(ctx, logr.Discard(), handler, nil)
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	waitForPort(t, addr, 3*time.Second)

	resp, err := http.Get(fmt.Sprintf("http://%s/", addr))
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Cancel context to initiate graceful shutdown.
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}
}

// selfSignedCert generates a self-signed TLS certificate for testing.
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}

func TestServe_HTTPS(t *testing.T) {
	httpPort := freePort(t)
	httpsPort := freePort(t)
	cert := selfSignedCert(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "secure")
	})

	cfg := NewConfig(func(c *Config) {
		c.BindAddr = "127.0.0.1"
		c.BindPort = httpPort
		c.HTTPSPort = httpsPort
		c.TLSCerts = []tls.Certificate{cert}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- cfg.Serve(ctx, logr.Discard(), nil, handler)
	}()

	httpsAddr := fmt.Sprintf("127.0.0.1:%d", httpsPort)
	waitForPort(t, httpsAddr, 3*time.Second)

	// Create a client that skips TLS verification for the self-signed cert.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Get(fmt.Sprintf("https://%s/", httpsAddr))
	if err != nil {
		t.Fatalf("HTTPS request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("got status %d, want %d", resp.StatusCode, http.StatusOK)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after context cancellation")
	}
}

func TestServe_GracefulShutdownWaitsForInflight(t *testing.T) {
	port := freePort(t)
	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseHandler
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "done")
	})

	cfg := NewConfig(func(c *Config) {
		c.BindAddr = "127.0.0.1"
		c.BindPort = port
		c.ShutdownTimeout = 10 * time.Second
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- cfg.Serve(ctx, logr.Discard(), handler, nil)
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	waitForPort(t, addr, 3*time.Second)

	// Start an in-flight request.
	type result struct {
		resp *http.Response
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		r, err := http.Get(fmt.Sprintf("http://%s/", addr)) //nolint:bodyclose // closed below after receiving from channel
		resCh <- result{resp: r, err: err}
	}()

	// Wait for the handler to start processing.
	<-requestStarted

	// Cancel context to start shutdown while request is in-flight.
	cancel()

	// Give the server a moment to enter shutdown.
	time.Sleep(100 * time.Millisecond)

	// Release the handler so the in-flight request can complete.
	close(releaseHandler)

	res := <-resCh
	if res.err != nil {
		t.Fatalf("in-flight request failed: %v", res.err)
	}
	defer res.resp.Body.Close()

	if res.resp.StatusCode != http.StatusOK {
		t.Errorf("in-flight request got status %d, want %d", res.resp.StatusCode, http.StatusOK)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after graceful shutdown")
	}
}

func TestServe_BindError(t *testing.T) {
	// Bind a listener to claim a port.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	// Try to start the server on the same port.
	cfg := NewConfig(func(c *Config) {
		c.BindAddr = "127.0.0.1"
		c.BindPort = port
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = cfg.Serve(ctx, logr.Discard(), handler, nil)
	if err == nil {
		t.Fatal("expected error when binding to an already-used port, got nil")
	}
}
