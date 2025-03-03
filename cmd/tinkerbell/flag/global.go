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
	EnableSmee           bool
	EnableTootles        bool
	EnableTinkServer     bool
	EnableTinkController bool
	EnableRufio          bool
	EnableSecondStar     bool
	EmbeddedGlobalConfig EmbeddedGlobalConfig
}

type EmbeddedGlobalConfig struct {
	EnableKubeAPIServer bool
	EnableETCD          bool
}

func RegisterGlobal(fs *Set, gc *GlobalConfig) {
	fs.Register(LogLevelConfig, ffval.NewValueDefault(&gc.LogLevel, gc.LogLevel))
	fs.Register(BackendConfig, ffval.NewEnum(&gc.Backend, "kube", "file", "none"))
	fs.Register(BackendFilePath, ffval.NewValueDefault(&gc.BackendFilePath, gc.BackendFilePath))
	fs.Register(BackendKubeConfig, ffval.NewValueDefault(&gc.BackendKubeConfig, gc.BackendKubeConfig))
	fs.Register(BackendKubeNamespace, ffval.NewValueDefault(&gc.BackendKubeNamespace, gc.BackendKubeNamespace))
	fs.Register(OTELEndpoint, ffval.NewValueDefault(&gc.OTELEndpoint, gc.OTELEndpoint))
	fs.Register(OTELInsecure, ffval.NewValueDefault(&gc.OTELInsecure, gc.OTELInsecure))
	fs.Register(TrustedProxies, &ntip.PrefixList{PrefixList: &gc.TrustedProxies})
	fs.Register(PublicIP, &ntip.Addr{Addr: &gc.PublicIP})
	fs.Register(EnableSmee, ffval.NewValueDefault(&gc.EnableSmee, gc.EnableSmee))
	fs.Register(EnableTootles, ffval.NewValueDefault(&gc.EnableTootles, gc.EnableTootles))
	fs.Register(EnableTinkServer, ffval.NewValueDefault(&gc.EnableTinkServer, gc.EnableTinkServer))
	fs.Register(EnableTinkController, ffval.NewValueDefault(&gc.EnableTinkController, gc.EnableTinkController))
	fs.Register(EnableRufioController, ffval.NewValueDefault(&gc.EnableRufio, gc.EnableRufio))
	fs.Register(EnableSecondStar, ffval.NewValueDefault(&gc.EnableSecondStar, gc.EnableSecondStar))
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
	Usage: "path to the file backend",
}

var BackendKubeConfig = Config{
	Name:  "backend-kube-config",
	Usage: "path to the kubeconfig file",
}

var BackendKubeNamespace = Config{
	Name:  "backend-kube-namespace",
	Usage: "namespace to watch for resources",
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

var EnableKubeAPIServer = Config{
	Name:  "enable-embedded-kube-apiserver",
	Usage: "enables the embedded kube-apiserver",
}

var EnableETCD = Config{
	Name:  "enable-embedded-etcd",
	Usage: "enables the embedded etcd",
}
