package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/pkg/kubecerts"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	defaultKubeAPIServerPKIDir = "./tinkerbell-pki"
	defaultKubeAPIServerPort   = 6443
	// defaultServiceAccountIssuer fills in --service-account-issuer, which
	// kube-apiserver requires whenever a service account signing key is set.
	// It is the value the documentation used before the PKI was generated.
	defaultServiceAccountIssuer = "api"
)

// pkiFlags are the kube-apiserver and backend flags that point the embedded
// kube-apiserver and its clients at PKI files, and the file in the PKI
// directory each one is set to. Setting any of them means the operator manages
// the PKI, and a mix of operator-provided and generated files would not trust
// each other.
var pkiFlags = []struct{ name, file string }{
	{"kube-apiserver-tls-cert-file", kubecerts.ServerCertFile},
	{"kube-apiserver-tls-private-key-file", kubecerts.ServerKeyFile},
	{"client-ca-file", kubecerts.CACertFile},
	{"service-account-key-file", kubecerts.ServiceAccountKeyFile},
	{"service-account-signing-key-file", kubecerts.ServiceAccountKeyFile},
	{flag.BackendKubeConfig.Name, kubecerts.KubeconfigFile},
}

// ensureKubeAPIServerPKI makes sure the PKI that the embedded kube-apiserver
// and its clients need exists in dir, generating it on first use, and sets the
// kube-apiserver and backend flags in fs to use it. sans are extra names and
// addresses for the serving certificate. An empty dir means
// defaultKubeAPIServerPKIDir.
//
// It does nothing if any of pkiFlags has a value. A flag set to an empty value
// counts as unset.
func ensureKubeAPIServerPKI(ctx context.Context, log logr.Logger, fs ff.Flags, dir string, sans []string) error {
	var set []string
	for _, pf := range pkiFlags {
		if f, ok := fs.GetFlag(pf.name); ok && hasValue(f) {
			set = append(set, "--"+pf.name)
		}
	}
	if len(set) > 0 {
		log.Info("not generating the embedded kube-apiserver PKI because PKI flags are set", "setFlags", set)
		return nil
	}
	if strings.TrimSpace(dir) == "" {
		dir = defaultKubeAPIServerPKIDir
	}

	port := defaultKubeAPIServerPort
	if f, ok := fs.GetFlag("secure-port"); ok {
		p, err := strconv.Atoi(f.GetValue())
		if err != nil {
			return fmt.Errorf("invalid --secure-port %q: %w", f.GetValue(), err)
		}
		port = p
	}
	// The kubeconfig must reach the kube-apiserver from this host, which
	// localhost cannot when it only listens on one specific address.
	host := "localhost"
	if f, ok := fs.GetFlag("kube-apiserver-bind-address"); ok {
		if a, err := netip.ParseAddr(f.GetValue()); err == nil && !a.IsUnspecified() {
			host = a.String()
		}
	}

	res, err := kubecerts.Ensure(ctx, kubecerts.Config{Dir: dir, Host: host, Port: port, SANs: sans})
	if errors.Is(err, kubecerts.ErrUnsafe) {
		err = fmt.Errorf("%w; make the directory and its files owned by the user running Tinkerbell and not writable "+
			"by its group or others, or set --%s to another directory", err, flag.KubeAPIServerPKIDir.Name)
	}
	if err != nil {
		return fmt.Errorf("embedded kube-apiserver PKI: %w", err)
	}

	for _, pf := range pkiFlags {
		if err := setFlag(fs, pf.name, res.Path(pf.file)); err != nil {
			return err
		}
	}
	if f, ok := fs.GetFlag("service-account-issuer"); ok && !hasValue(f) {
		if err := setFlag(fs, "service-account-issuer", defaultServiceAccountIssuer); err != nil {
			return err
		}
	}

	msg := "using the existing embedded kube-apiserver PKI"
	if res.Generated {
		msg = "generated the embedded kube-apiserver PKI"
	}
	log.Info(msg, "dir", res.Dir, "kubeconfig", res.Path(kubecerts.KubeconfigFile))
	return nil
}

// setFlag replaces the value of the flag name in fs with value.
func setFlag(fs ff.Flags, name, value string) error {
	f, ok := fs.GetFlag(name)
	if !ok {
		return fmt.Errorf("flag --%s is not defined", name)
	}
	// A slice flag that is already set would append value instead of
	// replacing the empty value it holds.
	if r, ok := f.(interface{ Reset() error }); ok {
		if err := r.Reset(); err != nil {
			return fmt.Errorf("reset --%s: %w", name, err)
		}
	}
	if err := f.SetValue(value); err != nil {
		return fmt.Errorf("set --%s: %w", name, err)
	}
	return nil
}

// hasValue reports whether f was set to a non-empty value. ff counts an
// environment variable that is set but empty as setting its flag, and pflag
// slice flags print both no values and a single empty value as "[]".
func hasValue(f ff.Flag) bool {
	v := f.GetValue()
	return f.IsSet() && v != "" && v != "[]"
}

// kubeAPIServerSANs returns the public addresses and host name that clients on
// other hosts may use to reach the embedded kube-apiserver.
func kubeAPIServerSANs(publicIPs ...netip.Addr) []string {
	var sans []string
	for _, a := range publicIPs {
		if a.IsValid() {
			sans = append(sans, a.String())
		}
	}
	// A host name that is not a valid DNS name cannot be a SAN, and it's not
	// worth failing startup over.
	if h, err := os.Hostname(); err == nil && len(validation.IsDNS1123Subdomain(strings.ToLower(h))) == 0 {
		sans = append(sans, h)
	}
	return sans
}
