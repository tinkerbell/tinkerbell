package kubecerts

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"k8s.io/client-go/tools/clientcmd"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
)

func assertP256(t *testing.T, name string, data []byte) {
	t.Helper()
	k, err := keyutil.ParsePrivateKeyPEM(data)
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return
	}
	ek, ok := k.(*ecdsa.PrivateKey)
	if !ok || ek.Curve != elliptic.P256() {
		t.Errorf("%s is %T, want an ECDSA P-256 key", name, k)
	}
}

// privateDir returns a new directory that only the current user can write to.
// t.TempDir follows the umask, which may make it group-writable.
func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readAll(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, n := range fileNames {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		out[n] = b
	}
	return out
}

func TestEnsureGenerates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pki")
	res, err := Ensure(context.Background(), Config{
		Dir:  dir,
		Host: "localhost",
		Port: 6443,
		SANs: []string{"192.168.2.50", "", "Tink.Example.com", "fd00::1"},
	})
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if !res.Generated {
		t.Fatal("Generated = false, want true")
	}
	if res.Dir != dir {
		t.Fatalf("Dir = %q, want %q", res.Dir, dir)
	}
	files := readAll(t, dir)
	for n, want := range map[string]os.FileMode{
		CACertFile: 0o644, CAKeyFile: 0o600, ServerCertFile: 0o644, ServerKeyFile: 0o600,
		ServiceAccountKeyFile: 0o600, KubeconfigFile: 0o600,
	} {
		fi, err := os.Stat(res.Path(n))
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", n, got, want)
		}
	}
	if fi, err := os.Stat(dir); err != nil {
		t.Error(err)
	} else if got := fi.Mode().Perm(); got != 0o700 {
		t.Errorf("directory mode = %o, want 700", got)
	}

	cas, err := certutil.ParseCertsPEM(files[CACertFile])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cas[0])

	server, err := tls.X509KeyPair(files[ServerCertFile], files[ServerKeyFile])
	if err != nil {
		t.Fatalf("server key pair: %v", err)
	}
	serverCert, err := x509.ParseCertificate(server.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"localhost", "kubernetes.default.svc", "tink.example.com", "127.0.0.1", "::1", "192.168.2.50", "fd00::1"} {
		if _, err := serverCert.Verify(x509.VerifyOptions{Roots: roots, DNSName: name, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			t.Errorf("server certificate not valid for %q: %v", name, err)
		}
	}

	for _, n := range []string{CAKeyFile, ServerKeyFile, ServiceAccountKeyFile} {
		assertP256(t, n, files[n])
	}

	kc, err := clientcmd.Load(files[KubeconfigFile])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kc.Clusters[kubeconfigCluster].Server, "https://localhost:6443"; got != want {
		t.Errorf("kubeconfig server = %q, want %q", got, want)
	}
	if !slices.Equal(kc.Clusters[kubeconfigCluster].CertificateAuthorityData, files[CACertFile]) {
		t.Error("kubeconfig does not trust ca.crt")
	}
	user := kc.AuthInfos[kubeconfigUser]
	assertP256(t, "kubeconfig client key", user.ClientKeyData)
	client, err := tls.X509KeyPair(user.ClientCertificateData, user.ClientKeyData)
	if err != nil {
		t.Fatalf("client key pair: %v", err)
	}
	clientCert, err := x509.ParseCertificate(client.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clientCert.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Errorf("client certificate: %v", err)
	}
	if got := clientCert.Subject.Organization; !slices.Equal(got, []string{clientGroup}) {
		t.Errorf("client organization = %v, want [%s]", got, clientGroup)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(fileNames) {
		t.Errorf("directory has %d entries, want %d: temporary files were left behind", len(entries), len(fileNames))
	}
}

func TestEnsureIPv6Host(t *testing.T) {
	dir := privateDir(t)
	if _, err := Ensure(context.Background(), Config{Dir: dir, Host: "::ffff:192.168.2.50", Port: 7443}); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	kc, err := clientcmd.LoadFromFile(filepath.Join(dir, KubeconfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kc.Clusters[kubeconfigCluster].Server, "https://192.168.2.50:7443"; got != want {
		t.Errorf("kubeconfig server = %q, want %q", got, want)
	}

	dir = privateDir(t)
	if _, err := Ensure(context.Background(), Config{Dir: dir, Host: "fd00::5", Port: 6443}); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	kc, err = clientcmd.LoadFromFile(filepath.Join(dir, KubeconfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := kc.Clusters[kubeconfigCluster].Server, "https://[fd00::5]:6443"; got != want {
		t.Errorf("kubeconfig server = %q, want %q", got, want)
	}
}

func TestEnsureKeepsExistingFiles(t *testing.T) {
	dir := privateDir(t)
	if _, err := Ensure(context.Background(), Config{Dir: dir, Host: "localhost", Port: 6443}); err != nil {
		t.Fatal(err)
	}
	before := readAll(t, dir)

	// A different config must not regenerate, so restarts with changed
	// settings never rotate the CA.
	res, err := Ensure(context.Background(), Config{Dir: dir, Host: "other.example.com", Port: 1})
	if err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	if res.Generated {
		t.Fatal("Generated = true, want false")
	}
	for n, b := range readAll(t, dir) {
		if !slices.Equal(b, before[n]) {
			t.Errorf("%s changed", n)
		}
	}
}

func TestEnsureIncomplete(t *testing.T) {
	dir := privateDir(t)
	if _, err := Ensure(context.Background(), Config{Dir: dir, Host: "localhost", Port: 6443}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{ServerCertFile, KubeconfigFile} {
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Ensure(context.Background(), Config{Dir: dir, Host: "localhost", Port: 6443})
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("Ensure() error = %v, want ErrIncomplete", err)
	}
	if want := "missing server.crt, kubeconfig"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err, want)
	}
	if _, err := os.Stat(filepath.Join(dir, ServerCertFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("server.crt was written: %v", err)
	}
}

func TestEnsureRefusesSymlink(t *testing.T) {
	dir := privateDir(t)
	if err := os.Symlink(filepath.Join(t.TempDir(), "elsewhere"), filepath.Join(dir, CAKeyFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(context.Background(), Config{Dir: dir, Host: "localhost", Port: 6443}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Ensure() error = %v, want symbolic link error", err)
	}
}

func TestEnsureRefusesUnusablePaths(t *testing.T) {
	tests := map[string]struct {
		// setup returns the directory to pass to Ensure.
		setup   func(t *testing.T) string
		wantErr string
	}{
		"directory is a symbolic link": {
			setup: func(t *testing.T) string {
				t.Helper()
				link := filepath.Join(privateDir(t), "pki")
				if err := os.Symlink(privateDir(t), link); err != nil {
					t.Fatal(err)
				}
				return link
			},
			wantErr: "is a symbolic link; use the directory it points to",
		},
		"directory is a file": {
			setup: func(t *testing.T) string {
				t.Helper()
				p := filepath.Join(privateDir(t), "pki")
				if err := os.WriteFile(p, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr: "is not a directory",
		},
		"file is a directory": {
			setup: func(t *testing.T) string {
				t.Helper()
				dir := privateDir(t)
				if err := os.Mkdir(filepath.Join(dir, KubeconfigFile), 0o700); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantErr: "kubeconfig: it is not a regular file",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := tt.setup(t)
			_, err := Ensure(context.Background(), Config{Dir: dir, Host: "localhost", Port: 6443})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Ensure() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEnsureInvalidConfig(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr string
	}{
		"empty dir":   {Config{Dir: " ", Host: "localhost", Port: 6443}, "directory must not be empty"},
		"empty host":  {Config{Host: " ", Port: 6443}, "host must not be empty"},
		"bad host":    {Config{Host: "not_a_host", Port: 6443}, `invalid SAN "not_a_host"`},
		"zoned host":  {Config{Host: "fe80::1%eth0", Port: 6443}, "IPv6 zones are not allowed"},
		"bad port":    {Config{Host: "localhost", Port: 0}, "invalid port 0"},
		"large port":  {Config{Host: "localhost", Port: 65536}, "invalid port 65536"},
		"invalid SAN": {Config{Host: "localhost", Port: 6443, SANs: []string{"bad name"}}, `invalid SAN "bad name"`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "pki")
			if tt.cfg.Dir == "" {
				tt.cfg.Dir = dir
			}
			_, err := Ensure(context.Background(), tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Ensure() error = %v, want error containing %q", err, tt.wantErr)
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("directory was created: %v", err)
			}
		})
	}
}

func TestEnsureCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := filepath.Join(t.TempDir(), "pki")
	if _, err := Ensure(ctx, Config{Dir: dir, Host: "localhost", Port: 6443}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ensure() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("directory was created: %v", err)
	}
}

func TestEnsureRefusesWritableByOthers(t *testing.T) {
	tests := map[string]struct {
		// setup returns the directory to pass to Ensure.
		setup func(t *testing.T) string
	}{
		"group-writable directory": {
			setup: func(t *testing.T) string {
				t.Helper()
				dir := privateDir(t)
				if err := os.Chmod(dir, 0o770); err != nil {
					t.Fatal(err)
				}
				return dir
			},
		},
		"world-writable sticky directory": {
			setup: func(t *testing.T) string {
				t.Helper()
				dir := privateDir(t)
				if err := os.Chmod(dir, 0o777|fs.ModeSticky); err != nil {
					t.Fatal(err)
				}
				return dir
			},
		},
		"group-writable file": {
			setup: func(t *testing.T) string {
				t.Helper()
				dir := privateDir(t)
				if _, err := Ensure(context.Background(), Config{Dir: dir, Host: "localhost", Port: 6443}); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(filepath.Join(dir, KubeconfigFile), 0o660); err != nil {
					t.Fatal(err)
				}
				return dir
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := tt.setup(t)
			_, err := Ensure(context.Background(), Config{Dir: dir, Host: "localhost", Port: 6443})
			if !errors.Is(err, ErrUnsafe) {
				t.Fatalf("Ensure() error = %v, want ErrUnsafe", err)
			}
		})
	}
}

// fileInfo is an fs.FileInfo with a chosen owner.
type fileInfo struct {
	mode fs.FileMode
	uid  uint32
}

func (f fileInfo) Name() string       { return "f" }
func (f fileInfo) Size() int64        { return 0 }
func (f fileInfo) Mode() fs.FileMode  { return f.mode }
func (f fileInfo) ModTime() time.Time { return time.Time{} }
func (f fileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fileInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid} }

func TestCheckOwner(t *testing.T) {
	euid := os.Geteuid()
	if euid == 0 {
		t.Skip("every owner other than root is another user only when not running as root")
	}
	tests := map[string]struct {
		fi       fileInfo
		wantSafe bool
	}{
		"current user":      {fi: fileInfo{mode: 0o600, uid: uint32(euid)}, wantSafe: true},
		"root":              {fi: fileInfo{mode: 0o600, uid: 0}, wantSafe: true},
		"another user":      {fi: fileInfo{mode: 0o600, uid: uint32(euid) + 1}},
		"group-writable":    {fi: fileInfo{mode: 0o620, uid: uint32(euid)}},
		"others-writable":   {fi: fileInfo{mode: 0o602, uid: uint32(euid)}},
		"group-readable":    {fi: fileInfo{mode: 0o640, uid: uint32(euid)}, wantSafe: true},
		"private dir":       {fi: fileInfo{mode: fs.ModeDir | 0o700, uid: uint32(euid)}, wantSafe: true},
		"world-traversable": {fi: fileInfo{mode: fs.ModeDir | 0o755, uid: uint32(euid)}, wantSafe: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkOwner("/x", tt.fi)
			if tt.wantSafe && err != nil {
				t.Fatalf("checkOwner() error = %v, want nil", err)
			}
			if !tt.wantSafe && !errors.Is(err, ErrUnsafe) {
				t.Fatalf("checkOwner() error = %v, want ErrUnsafe", err)
			}
		})
	}
}
