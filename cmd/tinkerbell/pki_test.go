package main

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/spf13/pflag"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/pkg/kubecerts"
	"k8s.io/client-go/tools/clientcmd"
)

// pkiTestArrayFlags are the flags that kube-apiserver defines as string arrays.
var pkiTestArrayFlags = []string{"service-account-key-file", "service-account-issuer"}

// pkiTestFlagNames returns the names of the string and string array flags
// that newPKITestFlags defines.
func pkiTestFlagNames() []string {
	names := []string{"service-account-issuer", "kube-apiserver-bind-address"}
	for _, pf := range pkiFlags {
		names = append(names, pf.name)
	}
	return names
}

// newPKITestFlags returns the kube-apiserver and backend flags that
// ensureKubeAPIServerPKI reads and sets, parsed from args and from TINKERBELL_
// environment variables. Flags that are string arrays in kube-apiserver are
// string arrays here too, wrapped like the embedded kube-apiserver flags.
func newPKITestFlags(t *testing.T, args ...string) *ff.FlagSet {
	t.Helper()
	fs := ff.NewFlagSet("test")
	pfs := pflag.NewFlagSet("kube-apiserver", pflag.ContinueOnError)
	for _, name := range pkiTestFlagNames() {
		if !slices.Contains(pkiTestArrayFlags, name) {
			fs.StringLong(name, "", "")
			continue
		}
		pfs.StringArray(name, nil, "")
		if _, err := fs.AddFlag(ff.FlagConfig{LongName: name, Value: newResettableSlice(pfs.Lookup(name).Value)}); err != nil {
			t.Fatal(err)
		}
	}
	fs.IntLong("secure-port", 6443, "")
	if err := ff.Parse(fs, args, ff.WithEnvVarPrefix(flag.EnvVarPrefix)); err != nil {
		t.Fatal(err)
	}
	return fs
}

// flagValue returns the value of the flag name in fs, without the brackets
// around string arrays, so a string array holding more than one value never
// equals a single path.
func flagValue(t *testing.T, fs ff.Flags, name string) string {
	t.Helper()
	f, ok := fs.GetFlag(name)
	if !ok {
		t.Fatalf("flag --%s is not defined", name)
	}
	if slices.Contains(pkiTestArrayFlags, name) {
		return strings.TrimSuffix(strings.TrimPrefix(f.GetValue(), "["), "]")
	}
	return f.GetValue()
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

// envKey returns the TINKERBELL_ environment variable for the flag name.
func envKey(name string) string {
	return flag.EnvVarPrefix + "_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// clearPKITestEnv unsets the environment variables for the flags that
// newPKITestFlags defines for the duration of t, so the environment running
// the tests can't change their results.
func clearPKITestEnv(t *testing.T) {
	t.Helper()
	for _, name := range append(pkiTestFlagNames(), "secure-port") {
		// Setenv restores the original value when t ends.
		t.Setenv(envKey(name), "")
		if err := os.Unsetenv(envKey(name)); err != nil {
			t.Fatal(err)
		}
	}
}

// assertGeneratedPKIFlags checks that every PKI flag in fs holds exactly the
// absolute path of its file in dir, and that the issuer is the default.
func assertGeneratedPKIFlags(t *testing.T, fs ff.Flags, dir string) {
	t.Helper()
	dir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, pf := range pkiFlags {
		want := filepath.Join(dir, pf.file)
		if got := flagValue(t, fs, pf.name); got != want {
			t.Errorf("--%s = %q, want %q", pf.name, got, want)
		}
		if _, err := os.Stat(want); err != nil {
			t.Errorf("--%s: %v", pf.name, err)
		}
	}
	if got := flagValue(t, fs, "service-account-issuer"); got != defaultServiceAccountIssuer {
		t.Errorf("--service-account-issuer = %q, want %q", got, defaultServiceAccountIssuer)
	}
}

// kubeconfigServer returns the server URL of the current context of the
// kubeconfig file at path.
func kubeconfigServer(t *testing.T, path string) string {
	t.Helper()
	kc, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, ok := kc.Contexts[kc.CurrentContext]
	if !ok {
		t.Fatalf("kubeconfig has no current context %q", kc.CurrentContext)
	}
	cluster, ok := kc.Clusters[ctx.Cluster]
	if !ok {
		t.Fatalf("kubeconfig has no cluster %q", ctx.Cluster)
	}
	return cluster.Server
}

func TestEnsureKubeAPIServerPKIGenerates(t *testing.T) {
	clearPKITestEnv(t)
	dir := filepath.Join(t.TempDir(), "pki")
	fs := newPKITestFlags(t, "--secure-port=7443")

	if err := ensureKubeAPIServerPKI(context.Background(), logr.Discard(), fs, dir, []string{"192.168.2.50"}); err != nil {
		t.Fatalf("ensureKubeAPIServerPKI() error = %v", err)
	}
	assertGeneratedPKIFlags(t, fs, dir)
	if got, want := kubeconfigServer(t, flagValue(t, fs, flag.BackendKubeConfig.Name)), "https://localhost:7443"; got != want {
		t.Errorf("kubeconfig server = %q, want %q", got, want)
	}

	// A restart reuses the files.
	before, err := os.ReadFile(filepath.Join(dir, kubecerts.CAKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureKubeAPIServerPKI(context.Background(), logr.Discard(), newPKITestFlags(t), dir, nil); err != nil {
		t.Fatalf("second ensureKubeAPIServerPKI() error = %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, kubecerts.CAKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, after) {
		t.Error("restart replaced the CA key")
	}
}

func TestEnsureKubeAPIServerPKIKeepsIssuerAndUsesBindAddress(t *testing.T) {
	clearPKITestEnv(t)
	dir := privateDir(t)
	fs := newPKITestFlags(t, "--service-account-issuer=https://tink.example.com", "--kube-apiserver-bind-address=192.168.2.50")

	if err := ensureKubeAPIServerPKI(context.Background(), logr.Discard(), fs, dir, nil); err != nil {
		t.Fatalf("ensureKubeAPIServerPKI() error = %v", err)
	}
	if got, want := flagValue(t, fs, "service-account-issuer"), "https://tink.example.com"; got != want {
		t.Errorf("--service-account-issuer = %q, want %q", got, want)
	}
	if got, want := kubeconfigServer(t, filepath.Join(dir, kubecerts.KubeconfigFile)), "https://192.168.2.50:6443"; got != want {
		t.Errorf("kubeconfig server = %q, want %q", got, want)
	}
}

func TestEnsureKubeAPIServerPKISkipsWhenAnyPKIFlagIsSet(t *testing.T) {
	for _, pf := range pkiFlags {
		t.Run(pf.name, func(t *testing.T) {
			clearPKITestEnv(t)
			dir := filepath.Join(t.TempDir(), "pki")
			fs := newPKITestFlags(t, "--"+pf.name+"=/operator/provided")

			if err := ensureKubeAPIServerPKI(context.Background(), logr.Discard(), fs, dir, nil); err != nil {
				t.Fatalf("ensureKubeAPIServerPKI() error = %v", err)
			}
			if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("PKI directory was created: %v", err)
			}
			for _, n := range pkiTestFlagNames() {
				want := ""
				if n == pf.name {
					want = "/operator/provided"
				}
				if got := flagValue(t, fs, n); got != want {
					t.Errorf("--%s = %q, want %q", n, got, want)
				}
			}
		})
	}
}

// TestEnsureKubeAPIServerPKIIgnoresEmptyEnv covers environment variables that
// are set but empty, which ff counts as setting their flags.
func TestEnsureKubeAPIServerPKIIgnoresEmptyEnv(t *testing.T) {
	names := []string{"service-account-issuer"}
	for _, pf := range pkiFlags {
		names = append(names, pf.name)
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			clearPKITestEnv(t)
			t.Setenv(envKey(name), "")
			dir := filepath.Join(t.TempDir(), "pki")
			fs := newPKITestFlags(t)
			if f, ok := fs.GetFlag(name); !ok || !f.IsSet() {
				t.Fatalf("--%s is not set by its empty environment variable", name)
			}

			if err := ensureKubeAPIServerPKI(context.Background(), logr.Discard(), fs, dir, nil); err != nil {
				t.Fatalf("ensureKubeAPIServerPKI() error = %v", err)
			}
			assertGeneratedPKIFlags(t, fs, dir)
		})
	}
}

func TestEnsureKubeAPIServerPKIEmptyDirUsesDefault(t *testing.T) {
	clearPKITestEnv(t)
	t.Chdir(privateDir(t))
	fs := newPKITestFlags(t)

	if err := ensureKubeAPIServerPKI(context.Background(), logr.Discard(), fs, " ", nil); err != nil {
		t.Fatalf("ensureKubeAPIServerPKI() error = %v", err)
	}
	assertGeneratedPKIFlags(t, fs, defaultKubeAPIServerPKIDir)
}

func TestEnsureKubeAPIServerPKIIncomplete(t *testing.T) {
	clearPKITestEnv(t)
	dir := privateDir(t)
	if err := ensureKubeAPIServerPKI(context.Background(), logr.Discard(), newPKITestFlags(t), dir, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, kubecerts.KubeconfigFile)); err != nil {
		t.Fatal(err)
	}
	err := ensureKubeAPIServerPKI(context.Background(), logr.Discard(), newPKITestFlags(t), dir, nil)
	if !errors.Is(err, kubecerts.ErrIncomplete) || !strings.Contains(err.Error(), "remove the directory to generate a new PKI") {
		t.Fatalf("ensureKubeAPIServerPKI() error = %v, want ErrIncomplete with remediation", err)
	}
}

func TestEnsureKubeAPIServerPKIUnsafeDir(t *testing.T) {
	clearPKITestEnv(t)
	dir := privateDir(t)
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	err := ensureKubeAPIServerPKI(context.Background(), logr.Discard(), newPKITestFlags(t), dir, nil)
	if !errors.Is(err, kubecerts.ErrUnsafe) || !strings.Contains(err.Error(), "--"+flag.KubeAPIServerPKIDir.Name) {
		t.Fatalf("ensureKubeAPIServerPKI() error = %v, want ErrUnsafe with remediation", err)
	}
}

func TestResettableSlice(t *testing.T) {
	pfs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	got := pfs.StringArray("array", []string{"default"}, "")
	pfs.String("string", "", "")

	v := newResettableSlice(pfs.Lookup("array").Value)
	for _, s := range []string{"a", "b"} {
		if err := v.Set(s); err != nil {
			t.Fatal(err)
		}
	}
	r, ok := v.(*resettableSlice)
	if !ok {
		t.Fatalf("newResettableSlice() = %T, want *resettableSlice", v)
	}
	if err := r.Reset(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*got, []string{"default"}) {
		t.Fatalf("after Reset = %q, want [default]", *got)
	}
	if got, want := r.GetPlaceholder(), "STRINGARRAY"; got != want {
		t.Errorf("GetPlaceholder() = %q, want %q", got, want)
	}

	if s := pfs.Lookup("string").Value; newResettableSlice(s) != s {
		t.Error("newResettableSlice wrapped a value that is not a slice")
	}
}

func TestKubeAPIServerSANs(t *testing.T) {
	got := kubeAPIServerSANs(netip.MustParseAddr("192.168.2.50"), netip.Addr{}, netip.MustParseAddr("fd00::1"))
	if len(got) < 2 || got[0] != "192.168.2.50" || got[1] != "fd00::1" {
		t.Fatalf("kubeAPIServerSANs() = %v, want the valid public IPs first", got)
	}
}
