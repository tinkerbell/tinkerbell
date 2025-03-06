package secondstar

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

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
