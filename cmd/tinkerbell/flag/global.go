package flag

import (
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	ntip "github.com/tinkerbell/tinkerbell/pkg/flag/netip"
)

type GlobalConfig struct {
	LogLevel             int
	Backend              string
	BackendFilePath      string
	BackendKubeConfig    string
	BackendKubeNamespace string
	OTELEndpoint         string
	OTELInsecure         bool
	TrustedProxies       []netip.Prefix
	PublicIP             netip.Addr
	BindAddr             netip.Addr
	EnableSmee           bool
	EnableTootles        bool
	EnableTinkServer     bool
	EnableTinkController bool
	EnableRufio          bool
	EnableSecondStar     bool
	EnableUI             bool
	EnableCRDMigrations  bool
	EmbeddedGlobalConfig EmbeddedGlobalConfig
	BackendKubeOptions   BackendKubeOptions
	TLS                  TLSConfig
}

type EmbeddedGlobalConfig struct {
	EnableKubeAPIServer bool
	EnableETCD          bool
}

type BackendKubeOptions struct {
	QPS   float32
	Burst int
}

type TLSConfig struct {
	CertFile string
	KeyFile  string
}

func RegisterGlobal(fs *Set, gc *GlobalConfig) {
	fs.Register(BackendConfig, ffval.NewEnum(&gc.Backend, "kube", "file", "none"))
	fs.Register(BackendFilePath, ffval.NewValueDefault(&gc.BackendFilePath, gc.BackendFilePath))
	fs.Register(KubeBurst, ffval.NewValueDefault(&gc.BackendKubeOptions.Burst, gc.BackendKubeOptions.Burst))
	fs.Register(BackendKubeConfig, ffval.NewValueDefault(&gc.BackendKubeConfig, gc.BackendKubeConfig))
	fs.Register(BackendKubeNamespace, ffval.NewValueDefault(&gc.BackendKubeNamespace, gc.BackendKubeNamespace))
	fs.Register(KubeQPS, ffval.NewValueDefault(&gc.BackendKubeOptions.QPS, gc.BackendKubeOptions.QPS))
	fs.Register(BindAddr, &ntip.Addr{Addr: &gc.BindAddr})
	fs.Register(EnableSmee, ffval.NewValueDefault(&gc.EnableSmee, gc.EnableSmee))
	fs.Register(EnableTootles, ffval.NewValueDefault(&gc.EnableTootles, gc.EnableTootles))
	fs.Register(EnableTinkServer, ffval.NewValueDefault(&gc.EnableTinkServer, gc.EnableTinkServer))
	fs.Register(EnableTinkController, ffval.NewValueDefault(&gc.EnableTinkController, gc.EnableTinkController))
	fs.Register(EnableRufioController, ffval.NewValueDefault(&gc.EnableRufio, gc.EnableRufio))
	fs.Register(EnableSecondStar, ffval.NewValueDefault(&gc.EnableSecondStar, gc.EnableSecondStar))
	fs.Register(EnableUI, ffval.NewValueDefault(&gc.EnableUI, gc.EnableUI))
	fs.Register(EnableCRDMigrations, ffval.NewValueDefault(&gc.EnableCRDMigrations, gc.EnableCRDMigrations))
	fs.Register(LogLevelConfig, ffval.NewValueDefault(&gc.LogLevel, gc.LogLevel))
	fs.Register(OTELEndpoint, ffval.NewValueDefault(&gc.OTELEndpoint, gc.OTELEndpoint))
	fs.Register(OTELInsecure, ffval.NewValueDefault(&gc.OTELInsecure, gc.OTELInsecure))
	fs.Register(PublicIP, &ntip.Addr{Addr: &gc.PublicIP})
	fs.Register(TLSCertFile, ffval.NewValueDefault(&gc.TLS.CertFile, gc.TLS.CertFile))
	fs.Register(TLSKeyFile, ffval.NewValueDefault(&gc.TLS.KeyFile, gc.TLS.KeyFile))
	fs.Register(TrustedProxies, &ntip.PrefixList{PrefixList: &gc.TrustedProxies})
}

func RegisterEmbeddedGlobals(fs *Set, gc *GlobalConfig) {
	fs.Register(EnableKubeAPIServer, ffval.NewValueDefault(&gc.EmbeddedGlobalConfig.EnableKubeAPIServer, gc.EmbeddedGlobalConfig.EnableKubeAPIServer))
	fs.Register(EnableETCD, ffval.NewValueDefault(&gc.EmbeddedGlobalConfig.EnableETCD, gc.EmbeddedGlobalConfig.EnableETCD))
}

// All these flags are used by at least two services or
// are used to create objects that are used by multiple services.
var LogLevelConfig = Config{
	Name:  "log-level",
	Usage: "the higher the number the more verbose",
}

// BackendConfig flags.
var BackendConfig = Config{
	Name:  "backend",
	Usage: "backend to use (kube, file, none)",
}

var BackendFilePath = Config{
	Name:  "backend-file-path",
	Usage: "[file] path to the file backend, this is only implemented when running only the Smee service",
}

// Kube backend flags.
var BackendKubeConfig = Config{
	Name:  "backend-kube-config",
	Usage: "[kube] path to the kubeconfig file",
}

var BackendKubeNamespace = Config{
	Name:  "backend-kube-namespace",
	Usage: "[kube] namespace to watch for resources",
}

var KubeQPS = Config{
	Name:  "backend-kube-qps",
	Usage: "[kube] maximum queries per second to the Kubernetes API server. A 0 value equates to 5 (client sdk constraint). A negative value disables client-side ratelimiting.",
}

var KubeBurst = Config{
	Name:  "backend-kube-burst",
	Usage: "[kube] maximum burst for throttle in the Kubernetes client. A 0 value equates to 10 (client sdk constraint). A negative value disables client-side burst limiting.",
}

// OTEL flags.
var OTELEndpoint = Config{
	Name:  "otel-endpoint",
	Usage: "[otel] OpenTelemetry collector endpoint",
}

var OTELInsecure = Config{
	Name:  "otel-insecure",
	Usage: "[otel] OpenTelemetry collector insecure",
}

// Shared flags.
var TrustedProxies = Config{
	Name:  "trusted-proxies",
	Usage: "list of trusted proxies in CIDR notation",
}

var PublicIP = Config{
	Name:  "public-ipv4",
	Usage: "public IPv4 address to use for all enabled services",
}

var EnableSmee = Config{
	Name:  "enable-smee",
	Usage: "enable Smee service",
}

var EnableTootles = Config{
	Name:  "enable-tootles",
	Usage: "enable Tootles service",
}

var EnableTinkServer = Config{
	Name:  "enable-tink-server",
	Usage: "enable Tink Server service",
}

var EnableTinkController = Config{
	Name:  "enable-tink-controller",
	Usage: "enable Tink Controller service",
}

var EnableRufioController = Config{
	Name:  "enable-rufio-controller",
	Usage: "enable Rufio Controller service",
}

var EnableSecondStar = Config{
	Name:  "enable-secondstar",
	Usage: "enable SecondStar service",
}

var EnableUI = Config{
	Name:  "enable-ui",
	Usage: "enable UI service",
}

var EnableKubeAPIServer = Config{
	Name:  "enable-embedded-kube-apiserver",
	Usage: "enables the embedded kube-apiserver",
}

var EnableETCD = Config{
	Name:  "enable-embedded-etcd",
	Usage: "enables the embedded etcd",
}

var EnableCRDMigrations = Config{
	Name:  "enable-crd-migrations",
	Usage: "create CRDs in the cluster",
}

var BindAddr = Config{
	Name:  "bind-address",
	Usage: "IP address to which to bind all services",
}

// TLS flags
var TLSCertFile = Config{
	Name:  "tls-cert-file",
	Usage: "[tls] path to the TLS certificate file",
}

var TLSKeyFile = Config{
	Name:  "tls-key-file",
	Usage: "[tls] path to the TLS key file",
}
