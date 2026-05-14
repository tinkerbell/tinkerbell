package containerd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestAddrHash_NerdctlExample reproduces nerdctl's documented example from
// pkg/clientutil.DataStore: the suffix for "/run/containerd/containerd.sock"
// is "1935db59". We can't write to /run in tests, so simulate the address
// via a tempdir + symlink and compute the expected suffix the same way
// nerdctl would, then assert that addrHash matches sha256(realpath)[:8].
func TestAddrHash_StableForResolvedPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "containerd.sock")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.sock")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	// Both paths must resolve to the same hash because EvalSymlinks
	// collapses the symlink first.
	h1, err := addrHash(target)
	if err != nil {
		t.Fatalf("addrHash(target): %v", err)
	}
	h2, err := addrHash(link)
	if err != nil {
		t.Fatalf("addrHash(link): %v", err)
	}
	if h1 != h2 {
		t.Errorf("symlink and target should hash equal, got %q vs %q", h1, h2)
	}

	// And the hash must be sha256(realpath)[:8].
	want := func() string {
		s := sha256.Sum256([]byte(target))
		return hex.EncodeToString(s[:])[:8]
	}()
	if h1 != want {
		t.Errorf("addrHash = %q, want %q", h1, want)
	}
}

func TestAddrHash_StripsUnixScheme(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "containerd.sock")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	plain, err := addrHash(target)
	if err != nil {
		t.Fatalf("addrHash(plain): %v", err)
	}
	scheme, err := addrHash("unix://" + target)
	if err != nil {
		t.Fatalf("addrHash(unix://): %v", err)
	}
	if plain != scheme {
		t.Errorf("unix:// prefix must be stripped, got %q vs %q", plain, scheme)
	}
}

func TestDataStoreDir_CreatesDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data-root")
	sock := filepath.Join(t.TempDir(), "containerd.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := dataStoreDir(root, sock)
	if err != nil {
		t.Fatalf("dataStoreDir: %v", err)
	}
	if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
		t.Fatalf("expected data store dir to exist, got err=%v", err)
	}
	// Must be a 8-hex-char child of root.
	parent, suffix := filepath.Split(got)
	if filepath.Clean(parent) != filepath.Clean(root) {
		t.Errorf("parent = %q, want %q", parent, root)
	}
	if len(suffix) != 8 {
		t.Errorf("suffix %q is not 8 chars", suffix)
	}
}

func TestContainerLogPaths(t *testing.T) {
	dir := containerLogDir("/var/lib/nerdctl/abcd1234", "tinkerbell", "deadbeef")
	if got, want := dir, "/var/lib/nerdctl/abcd1234/containers/tinkerbell/deadbeef"; got != want {
		t.Errorf("containerLogDir = %q, want %q", got, want)
	}
	file := containerLogFile("/var/lib/nerdctl/abcd1234", "tinkerbell", "deadbeef")
	if got, want := file, "/var/lib/nerdctl/abcd1234/containers/tinkerbell/deadbeef/deadbeef-json.log"; got != want {
		t.Errorf("containerLogFile = %q, want %q", got, want)
	}
}

func TestWriteLogConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ctr")
	if err := writeLogConfig(dir, "/run/containerd/containerd.sock"); err != nil {
		t.Fatalf("writeLogConfig: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "log-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got logConfig
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, b)
	}
	if got.Driver != "json-file" {
		t.Errorf("Driver = %q, want json-file", got.Driver)
	}
	if got.Address != "unix:///run/containerd/containerd.sock" {
		t.Errorf("Address = %q, want unix:// prefix added", got.Address)
	}
}

func TestWriteLogConfig_PreservesExistingScheme(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ctr")
	if err := writeLogConfig(dir, "tcp://1.2.3.4:5000"); err != nil {
		t.Fatalf("writeLogConfig: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "log-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got logConfig
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Address != "tcp://1.2.3.4:5000" {
		t.Errorf("Address = %q, want unchanged", got.Address)
	}
}
