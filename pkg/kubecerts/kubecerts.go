// Package kubecerts generates the certificates, keys, and kubeconfig that a
// kube-apiserver, and the clients that talk to it, need to run with TLS and
// client certificate authentication.
package kubecerts

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/keyutil"
)

// File names in Config.Dir.
const (
	CACertFile            = "ca.crt"
	CAKeyFile             = "ca.key"
	ServerCertFile        = "server.crt"
	ServerKeyFile         = "server.key"
	ServiceAccountKeyFile = "service-account-key.pem"
	KubeconfigFile        = "kubeconfig"
)

const (
	caCommonName     = "tinkerbell-ca"
	serverCommonName = "kube-apiserver"
	clientCommonName = "tinkerbell-admin"
	// clientGroup is a superuser group that kube-apiserver authorizes
	// unconditionally, so the generated kubeconfig keeps working when RBAC is
	// enabled.
	clientGroup = "system:masters"

	kubeconfigCluster = "local-apiserver"
	kubeconfigUser    = "admin"
	kubeconfigContext = "default"

	validity = 10 * 365 * 24 * time.Hour
	// clockSkew backdates NotBefore so that hosts with slightly slower clocks
	// don't reject freshly generated certificates.
	clockSkew = time.Hour
)

// fileNames are the names of every file in Config.Dir.
var fileNames = []string{CACertFile, CAKeyFile, ServerCertFile, ServerKeyFile, ServiceAccountKeyFile, KubeconfigFile}

// defaultSANs are the names and addresses in-cluster and local clients
// commonly use to reach a kube-apiserver. 10.0.0.1 is the "kubernetes"
// Service address under kube-apiserver's default --service-cluster-ip-range.
var defaultSANs = []string{
	"kubernetes",
	"kubernetes.default",
	"kubernetes.default.svc",
	"kubernetes.default.svc.cluster",
	"kubernetes.default.svc.cluster.local",
	"kube-apiserver",
	"tinkerbell",
	"localhost",
	"127.0.0.1",
	"::1",
	"10.0.0.1",
}

// ErrIncomplete is returned by Ensure when only some of the files exist.
var ErrIncomplete = errors.New("incomplete set of kube-apiserver PKI files")

// ErrUnsafe is returned by Ensure when a user other than the current one or
// root could change the directory or one of its files, and so could replace
// the CA or the kubeconfig to take control of the kube-apiserver.
var ErrUnsafe = errors.New("kube-apiserver PKI directory is not safe to use")

// Config describes the files that Ensure generates.
type Config struct {
	// Dir is the directory holding the files. It is created if missing.
	Dir string
	// Host is the kube-apiserver name or IP address that the kubeconfig points
	// to. It is also added to the server certificate's SANs.
	Host string
	// Port is the kube-apiserver port that the kubeconfig points to.
	Port int
	// SANs are additional DNS names or IP addresses for the server
	// certificate. Empty values are ignored.
	SANs []string
}

// Result describes the files that Ensure found or generated.
type Result struct {
	// Dir is the absolute path of the directory holding the files.
	Dir string
	// Generated is true when Ensure wrote the files, and false when they
	// already existed.
	Generated bool
}

// Path returns the absolute path of the named file.
func (r *Result) Path(name string) string {
	return filepath.Join(r.Dir, name)
}

// Ensure makes sure that c.Dir holds a CA, a kube-apiserver serving
// certificate signed by it, a service account signing key, and a kubeconfig
// that authenticates as a member of the system:masters group. Every key is
// ECDSA P-256 and every certificate is valid for 10 years.
//
// If none of the files exist, Ensure generates all of them. If all of them
// exist, Ensure leaves them untouched and ignores every field of c except Dir,
// so changes to the other fields only take effect once the files are removed.
// If only some exist, Ensure returns an error wrapping [ErrIncomplete] rather
// than generate a new CA that the existing files do not trust.
//
// c.Dir must be a directory, not a symbolic link, and its files must be
// regular files. Ensure returns an error wrapping [ErrUnsafe] if c.Dir or one
// of its files is owned by a user other than the current one or root, or is
// writable by its group or others.
//
// Every file is staged before any is written, so a failure while generating or
// staging leaves c.Dir unchanged.
func Ensure(ctx context.Context, c Config) (*Result, error) {
	if strings.TrimSpace(c.Dir) == "" {
		return nil, errors.New("directory must not be empty")
	}
	dir, err := filepath.Abs(c.Dir)
	if err != nil {
		return nil, fmt.Errorf("resolve directory: %w", err)
	}
	res := &Result{Dir: dir}

	if err := checkDir(dir); err != nil {
		return nil, err
	}
	missing, err := missingFiles(dir)
	if err != nil {
		return nil, err
	}
	switch len(missing) {
	case 0:
		return res, nil
	case len(fileNames):
	default:
		return nil, fmt.Errorf("%w in %s: missing %s; restore them, or remove the directory to generate a new PKI, "+
			"which invalidates existing kubeconfigs and service account tokens", ErrIncomplete, dir, strings.Join(missing, ", "))
	}

	server, dns, ips, err := c.validate()
	if err != nil {
		return nil, err
	}
	files, err := generate(ctx, time.Now(), server, dns, ips)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}
	// Another user may have created dir after it was first checked.
	if err := checkDir(dir); err != nil {
		return nil, err
	}
	if err := writeAll(dir, files); err != nil {
		return nil, err
	}
	res.Generated = true
	return res, nil
}

// checkDir returns an error if dir exists but is a symbolic link, is not a
// directory, or is not safe to use. A missing dir is not an error.
func checkDir(dir string) error {
	fi, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check %s: %w", dir, err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("refusing to use %s: it is a symbolic link; use the directory it points to", dir)
	}
	if !fi.IsDir() {
		return fmt.Errorf("refusing to use %s: it is not a directory", dir)
	}
	return checkOwner(dir, fi)
}

// missingFiles returns the names of the files that are not in dir. It returns
// an error if any file that is in dir is a symbolic link, is not a regular
// file, or is not safe to use.
func missingFiles(dir string) ([]string, error) {
	var missing []string
	for _, n := range fileNames {
		p := filepath.Join(dir, n)
		fi, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			missing = append(missing, n)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("check %s: %w", p, err)
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to use %s: it is a symbolic link", p)
		}
		if !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("refusing to use %s: it is not a regular file", p)
		}
		if err := checkOwner(p, fi); err != nil {
			return nil, err
		}
	}
	return missing, nil
}

// checkOwner returns an error wrapping [ErrUnsafe] if a user other than the
// current one or root could change path: if fi, the result of Lstat on path,
// shows that path is writable by its group or others, or is owned by another
// user. Files owned by root are accepted because root can change any file.
func checkOwner(path string, fi fs.FileInfo) error {
	if perm := fi.Mode().Perm(); perm&0o022 != 0 {
		return fmt.Errorf("%w: %s is writable by its group or others (mode %#o)", ErrUnsafe, path, perm)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("check owner of %s: unsupported file information %T", path, fi.Sys())
	}
	if uid := int64(os.Geteuid()); int64(st.Uid) != 0 && int64(st.Uid) != uid {
		return fmt.Errorf("%w: %s is owned by uid %d, not uid %d", ErrUnsafe, path, st.Uid, uid)
	}
	return nil
}

// validate returns the kubeconfig server URL and the server certificate's
// DNS and IP SANs.
func (c Config) validate() (server string, dns []string, ips []net.IP, err error) {
	if c.Port < 1 || c.Port > 65535 {
		return "", nil, nil, fmt.Errorf("invalid port %d: must be between 1 and 65535", c.Port)
	}
	host := strings.TrimSpace(c.Host)
	if host == "" {
		return "", nil, nil, errors.New("host must not be empty")
	}
	names := slices.Concat(defaultSANs, []string{host})
	for _, v := range c.SANs {
		if v = strings.TrimSpace(v); v != "" {
			names = append(names, v)
		}
	}
	if dns, ips, err = parseSANs(names); err != nil {
		return "", nil, nil, err
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		host = addr.Unmap().String()
	} else {
		host = strings.ToLower(host)
	}
	return "https://" + net.JoinHostPort(host, strconv.Itoa(c.Port)), dns, ips, nil
}

// parseSANs splits names into DNS names and IP addresses without duplicates.
// IPv4-mapped IPv6 addresses become IPv4 addresses, so they match clients
// that connect over IPv4.
func parseSANs(names []string) (dns []string, ips []net.IP, err error) {
	for _, v := range names {
		if addr, err := netip.ParseAddr(v); err == nil {
			if addr.Zone() != "" {
				return nil, nil, fmt.Errorf("invalid SAN %q: IPv6 zones are not allowed", v)
			}
			ip := net.IP(addr.Unmap().AsSlice())
			if !slices.ContainsFunc(ips, ip.Equal) {
				ips = append(ips, ip)
			}
			continue
		}
		name := strings.ToLower(v)
		if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
			return nil, nil, fmt.Errorf("invalid SAN %q: must be an IP address or DNS name: %s", v, strings.Join(errs, "; "))
		}
		if !slices.Contains(dns, name) {
			dns = append(dns, name)
		}
	}
	return dns, ips, nil
}

// file is a generated file waiting to be written.
type file struct {
	name string
	data []byte
	perm fs.FileMode
}

// generate returns the files of a new PKI whose server certificate is valid
// for dns and ips, and whose kubeconfig points to server.
func generate(ctx context.Context, now time.Time, server string, dns []string, ips []net.IP) ([]file, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var files []file
	addKey := func(name string) (crypto.Signer, error) {
		k, data, err := newKey()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		files = append(files, file{name: name, data: data, perm: 0o600})
		return k, nil
	}
	notBefore, notAfter := now.Add(-clockSkew), now.Add(validity)

	caKey, err := addKey(CAKeyFile)
	if err != nil {
		return nil, err
	}
	caDER, ca, err := sign(&x509.Certificate{
		Subject:               pkix.Name{CommonName: caCommonName, Organization: []string{"Tinkerbell"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}, nil, caKey.Public(), caKey)
	if err != nil {
		return nil, fmt.Errorf("create CA certificate: %w", err)
	}
	caPEM := certPEM(caDER)
	files = append(files, file{name: CACertFile, data: caPEM, perm: 0o644})

	serverKey, err := addKey(ServerKeyFile)
	if err != nil {
		return nil, err
	}
	serverDER, _, err := sign(&x509.Certificate{
		Subject:               pkix.Name{CommonName: serverCommonName, Organization: []string{"Tinkerbell"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              dns,
		IPAddresses:           ips,
	}, ca, serverKey.Public(), caKey)
	if err != nil {
		return nil, fmt.Errorf("create server certificate: %w", err)
	}
	files = append(files, file{name: ServerCertFile, data: certPEM(serverDER), perm: 0o644})

	if _, err := addKey(ServiceAccountKeyFile); err != nil {
		return nil, err
	}

	clientKey, clientKeyPEM, err := newKey()
	if err != nil {
		return nil, fmt.Errorf("client key: %w", err)
	}
	clientDER, _, err := sign(&x509.Certificate{
		Subject:               pkix.Name{CommonName: clientCommonName, Organization: []string{clientGroup}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}, ca, clientKey.Public(), caKey)
	if err != nil {
		return nil, fmt.Errorf("create client certificate: %w", err)
	}
	kubeconfig, err := encodeKubeconfig(server, caPEM, certPEM(clientDER), clientKeyPEM)
	if err != nil {
		return nil, err
	}
	return append(files, file{name: KubeconfigFile, data: kubeconfig, perm: 0o600}), nil
}

// newKey returns a new ECDSA P-256 private key and its PEM encoding.
func newKey() (*ecdsa.PrivateKey, []byte, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}
	data, err := keyutil.MarshalPrivateKeyToPEM(k)
	if err != nil {
		return nil, nil, fmt.Errorf("encode key: %w", err)
	}
	return k, data, nil
}

// sign creates a certificate from tmpl for pub, signed by parentKey. A nil
// parent self-signs.
func sign(tmpl, parent *x509.Certificate, pub crypto.PublicKey, parentKey crypto.Signer) ([]byte, *x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial number: %w", err)
	}
	tmpl.SerialNumber = serial
	if parent == nil {
		parent = tmpl
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, parentKey)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return der, cert, nil
}

func certPEM(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func encodeKubeconfig(server string, caPEM, certPEM, keyPEM []byte) ([]byte, error) {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[kubeconfigCluster] = &clientcmdapi.Cluster{
		Server:                   server,
		CertificateAuthorityData: caPEM,
	}
	cfg.AuthInfos[kubeconfigUser] = &clientcmdapi.AuthInfo{
		ClientCertificateData: certPEM,
		ClientKeyData:         keyPEM,
	}
	cfg.Contexts[kubeconfigContext] = &clientcmdapi.Context{
		Cluster:  kubeconfigCluster,
		AuthInfo: kubeconfigUser,
	}
	cfg.CurrentContext = kubeconfigContext
	b, err := clientcmd.Write(*cfg)
	if err != nil {
		return nil, fmt.Errorf("encode kubeconfig: %w", err)
	}
	return b, nil
}

// writeAll stages every file as a temporary file in dir before renaming any
// into place, so a failure while staging leaves the directory untouched.
func writeAll(dir string, files []file) (err error) {
	var staged, published []string
	defer func() {
		if err == nil {
			return
		}
		for _, p := range slices.Concat(staged, published) {
			_ = os.Remove(p)
		}
	}()

	for _, f := range files {
		p, err := stage(dir, f)
		if err != nil {
			return fmt.Errorf("write %s: %w", f.name, err)
		}
		staged = append(staged, p)
	}
	for _, f := range files {
		target := filepath.Join(dir, f.name)
		if err := os.Rename(staged[0], target); err != nil {
			return fmt.Errorf("write %s: %w", f.name, err)
		}
		staged = staged[1:]
		published = append(published, target)
	}
	return nil
}

// stage writes f to a new temporary file in dir and returns its path.
func stage(dir string, f file) (path string, err error) {
	tmp, err := os.CreateTemp(dir, "."+f.name+".tmp-*")
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := tmp.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if err := tmp.Chmod(f.perm); err != nil {
		return "", err
	}
	if _, err := tmp.Write(f.data); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}
