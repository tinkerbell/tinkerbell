/*
Copyright The Tinkerbell Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package containerd

import (
	"context"
	"crypto/sha256"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const testRegistryHost = "registry.example.test"

func TestResolverUsesRegistryHostsConfiguration(t *testing.T) {
	tests := map[string]struct {
		tls        bool
		writeCA    bool
		hostConfig func(serverURL string) string
	}{
		"plain HTTP": {
			hostConfig: func(serverURL string) string {
				return registryHostConfig(serverURL, "")
			},
		},
		"HTTPS skip verify": {
			tls: true,
			hostConfig: func(serverURL string) string {
				return registryHostConfig(serverURL, "  skip_verify = true")
			},
		},
		"HTTPS custom CA": {
			tls:     true,
			writeCA: true,
			hostConfig: func(serverURL string) string {
				return registryHostConfig(serverURL, `  ca = "ca.crt"`)
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var manifestRequests atomic.Int32
			server := newTestRegistry(t, tt.tls, &manifestRequests)

			root := t.TempDir()
			hostDir := filepath.Join(root, testRegistryHost)
			if err := os.MkdirAll(hostDir, 0o755); err != nil {
				t.Fatalf("create registry host directory: %v", err)
			}

			if tt.writeCA {
				certificate := server.Certificate()
				certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
				if err := os.WriteFile(filepath.Join(hostDir, "ca.crt"), certificatePEM, 0o600); err != nil {
					t.Fatalf("write registry CA: %v", err)
				}
			}

			if err := os.WriteFile(filepath.Join(hostDir, "hosts.toml"), []byte(tt.hostConfig(server.URL)), 0o600); err != nil {
				t.Fatalf("write hosts.toml: %v", err)
			}

			resolver := newResolver(context.Background(), root)
			if _, _, err := resolver.Resolve(context.Background(), testRegistryHost+"/actions/test:latest"); err != nil {
				t.Fatalf("resolve image: %v", err)
			}
			if got := manifestRequests.Load(); got == 0 {
				t.Fatal("configured registry did not receive a manifest request")
			}
		})
	}
}

func TestRegistryHostsFallsBackWithoutConfiguration(t *testing.T) {
	tests := map[string]string{
		"empty path":   "",
		"missing path": filepath.Join(t.TempDir(), "missing"),
	}

	for name, configPath := range tests {
		t.Run(name, func(t *testing.T) {
			hosts, err := newRegistryHosts(context.Background(), configPath)(testRegistryHost)
			if err != nil {
				t.Fatalf("resolve default registry hosts: %v", err)
			}
			if len(hosts) != 1 {
				t.Fatalf("default registry hosts count = %d, want 1", len(hosts))
			}
			if hosts[0].Scheme != "https" {
				t.Errorf("default registry scheme = %q, want https", hosts[0].Scheme)
			}
			if hosts[0].Host != testRegistryHost {
				t.Errorf("default registry host = %q, want %q", hosts[0].Host, testRegistryHost)
			}
		})
	}
}

func registryHostConfig(serverURL, extra string) string {
	return fmt.Sprintf(`server = %q

[host.%q]
  capabilities = ["pull", "resolve"]
%s
`, serverURL, serverURL, extra)
}

func newTestRegistry(t *testing.T, useTLS bool, manifestRequests *atomic.Int32) *httptest.Server {
	t.Helper()

	manifest := []byte(`{"schemaVersion":2}`)
	manifestDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))
	manifestPath := "/v2/actions/test/manifests/latest"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/":
			w.WriteHeader(http.StatusOK)
		case manifestPath:
			manifestRequests.Add(1)
			w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
			w.Header().Set("Content-Length", strconv.Itoa(len(manifest)))
			w.Header().Set("Docker-Content-Digest", manifestDigest)
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write(manifest); err != nil {
				t.Errorf("write manifest response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	})

	var server *httptest.Server
	if useTLS {
		server = httptest.NewTLSServer(handler)
	} else {
		server = httptest.NewServer(handler)
	}
	t.Cleanup(server.Close)

	return server
}
