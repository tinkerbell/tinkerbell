package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// TestServe_DualStackSharedPort covers the configuration an operator is most
// likely to write for dual stack: both wildcards on the same port. It only
// works if each listener is restricted to its own family.
func TestServe_DualStackSharedPort(t *testing.T) {
	port := freePort(t)
	if _, err := net.Listen("tcp6", "[::1]:0"); err != nil {
		t.Skipf("IPv6 localhost is unavailable: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	cfg := NewConfig(func(c *Config) {
		c.V4 = Listener{Addr: netip.MustParseAddr("0.0.0.0"), HTTPPort: port}
		c.V6 = Listener{Addr: netip.MustParseAddr("::"), HTTPPort: port}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- cfg.Serve(ctx, logr.Discard(), handler, nil)
	}()

	for _, addr := range []string{
		fmt.Sprintf("127.0.0.1:%d", port),
		fmt.Sprintf("[::1]:%d", port),
	} {
		waitForPort(t, addr)
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			t.Fatalf("GET %s failed: %v", addr, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want %d", addr, resp.StatusCode, http.StatusOK)
		}
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

// A family with no address must not be served.
func TestServe_SingleFamilyOnly(t *testing.T) {
	port := freePort(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cfg := NewConfig(func(c *Config) {
		c.V4 = Listener{Addr: netip.MustParseAddr("127.0.0.1"), HTTPPort: port}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = cfg.Serve(ctx, logr.Discard(), handler, nil) }()
	waitForPort(t, fmt.Sprintf("127.0.0.1:%d", port))

	// The IPv6 loopback must be refused because no IPv6 listener was configured.
	if c, err := net.DialTimeout("tcp", fmt.Sprintf("[::1]:%d", port), time.Second); err == nil {
		c.Close()
		t.Error("IPv6 connection succeeded, want refused")
	}
}

// Serve returns rather than blocking when no family is configured.
func TestServe_NoListeners(t *testing.T) {
	cfg := NewConfig()
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := cfg.Serve(ctx, logr.Discard(), handler, nil); err != nil {
		t.Errorf("Serve = %v, want nil", err)
	}
}
