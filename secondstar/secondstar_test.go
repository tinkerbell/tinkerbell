package secondstar

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/listener"
)

// Start must return once its context is cancelled. gliderlabs/ssh resets the
// server's done channel when Serve registers a listener, so a Shutdown that
// arrives first leaves Accept with nothing to stop it and hangs the process.
func TestStartReturnsOnContextCancel(t *testing.T) {
	hostKey, err := generateHostKey()
	if err != nil {
		t.Fatal(err)
	}
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).AddrPort().Port()
	probe.Close()

	c := &Config{
		V4:      listener.Bind{Addr: netip.MustParseAddr("127.0.0.1"), Port: port, Enabled: true},
		HostKey: hostKey,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx, logr.Discard()) }()

	// Give Serve time to register its listener, which is the race being guarded.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start() = %v, want nil", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Start did not return after its context was cancelled")
	}
}

func TestHostKeyFrom(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "valid private key",
			wantErr: false,
		},
		{
			name:    "non-existent file",
			wantErr: true,
		},
		{
			name:    "invalid private key format",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keyBytes []byte
			var nonExistentFile bool
			switch tt.name {
			case "valid private key":
				privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatal(err)
				}
				keyBytes = pem.EncodeToMemory(
					&pem.Block{
						Type:  "RSA PRIVATE KEY",
						Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
					},
				)
			case "non-existent file":
				nonExistentFile = true
			case "invalid private key format":
				keyBytes = []byte("invalid-key-content")
			}

			tmpFile, err := os.CreateTemp("", "test-key-*")
			if err != nil {
				t.Fatal(err)
			}
			defer tmpFile.Close()

			if _, err := tmpFile.Write(keyBytes); err != nil {
				t.Fatal(err)
			}

			cleanup := func() { os.Remove(tmpFile.Name()) }
			filePath := tmpFile.Name()
			if nonExistentFile {
				filePath = "non-existent-file"
			}

			defer cleanup()

			signer, err := HostKeyFrom(filePath)

			if (err != nil) != tt.wantErr {
				t.Errorf("HostKeyFrom() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err == nil {
				t.Errorf("HostKeyFrom() did not return an error")
			}

			if !tt.wantErr && signer == nil {
				t.Errorf("HostKeyFrom() returned nil signer unexpectedly")
			}
		})
	}
}
