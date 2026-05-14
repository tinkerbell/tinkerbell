package containerd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultDataRoot mirrors nerdctl's default --data-root. nerdctl stores
// per-container state (including json-file logs) under
// <DataRoot>/<sha256(address)[:8]>/containers/<namespace>/<id>/.
//
// See: https://github.com/containerd/nerdctl/blob/main/pkg/clientutil/client.go
const defaultDataRoot = "/var/lib/nerdctl"

// dataStoreDir reproduces nerdctl's pkg/clientutil.DataStore so that files
// written here are discoverable by the `nerdctl` CLI talking to the same
// containerd socket. The 8-character suffix is the first 8 hex chars of the
// SHA-256 of the (symlink-resolved) socket path; e.g. for the default
// "/run/containerd/containerd.sock" the suffix is "1935db59".
//
// Reproduced (rather than imported) to avoid pulling nerdctl's full
// dependency tree. Mirror of:
// https://github.com/containerd/nerdctl/blob/main/pkg/clientutil/client.go
func dataStoreDir(dataRoot, address string) (string, error) {
	if dataRoot == "" {
		dataRoot = defaultDataRoot
	}
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		return "", fmt.Errorf("create data root %q: %w", dataRoot, err)
	}
	suffix, err := addrHash(address)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(dataRoot, suffix)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create data store %q: %w", dir, err)
	}
	return dir, nil
}

// addrHash returns the first 8 hex chars of sha256(realpath(address)).
// nerdctl strips a leading "unix://" scheme and resolves symlinks before
// hashing, so we do the same. We deliberately fail fast if the path does
// not resolve: silently falling back to hashing the unresolved address
// would diverge from nerdctl's hash whenever the path contains symlinks,
// causing `nerdctl logs` to look in a different per-socket directory than
// the one tink-agent wrote logs to. By the time Execute reaches this
// code, containerd.New(SocketPath) in NewConfig has already dialed the
// socket, so a missing path here indicates a real configuration problem.
func addrHash(address string) (string, error) {
	addr := strings.TrimPrefix(address, "unix://")
	resolved, err := filepath.EvalSymlinks(addr)
	if err != nil {
		return "", fmt.Errorf("eval symlinks for %q: %w", addr, err)
	}
	sum := sha256.Sum256([]byte(resolved))
	return hex.EncodeToString(sum[:])[:8], nil
}

// containerLogDir returns the per-container directory nerdctl expects for
// logs and log-config.json: <dataStore>/containers/<namespace>/<id>.
func containerLogDir(dataStore, namespace, id string) string {
	return filepath.Join(dataStore, "containers", namespace, id)
}

// containerLogFile returns the json-file log path nerdctl reads for
// `nerdctl logs`: <containerLogDir>/<id>-json.log.
//
// Mirror of nerdctl's pkg/logging/jsonfile.Path:
// https://github.com/containerd/nerdctl/blob/main/pkg/logging/jsonfile/jsonfile.go
func containerLogFile(dataStore, namespace, id string) string {
	return filepath.Join(containerLogDir(dataStore, namespace, id), id+"-json.log")
}

// logConfig is the shape nerdctl persists at
// <containerLogDir>/log-config.json. nerdctl's `nerdctl logs` reads this
// file (NOT the nerdctl/log-uri label) to choose a log driver, so it must
// exist for our containers to be readable.
//
// Mirror of nerdctl's pkg/logging.LogConfig:
// https://github.com/containerd/nerdctl/blob/main/pkg/logging/logging.go
type logConfig struct {
	Driver  string            `json:"driver"`
	Opts    map[string]string `json:"opts,omitempty"`
	Address string            `json:"address"`
}

// writeLogConfig writes log-config.json for a container so that
// `nerdctl logs` selects the json-file driver. The address must match
// the containerd socket nerdctl is invoked against; it is normalized to
// include the "unix://" scheme.
func writeLogConfig(dir, address string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create log dir %q: %w", dir, err)
	}
	if !strings.Contains(address, "://") {
		address = "unix://" + address
	}
	cfg := logConfig{
		Driver:  "json-file",
		Opts:    map[string]string{},
		Address: address,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal log-config: %w", err)
	}
	path := filepath.Join(dir, "log-config.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
