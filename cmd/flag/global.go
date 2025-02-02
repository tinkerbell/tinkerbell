package flag

import (
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	ntip "github.com/tinkerbell/tinkerbell/cmd/flag/netip"
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
	EnableHegel          bool
	EnableTinkServer     bool
	EnableTinkController bool
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
	fs.Register(EnabledSmee, ffval.NewValueDefault(&gc.EnableSmee, gc.EnableSmee))
	fs.Register(EnabledHegel, ffval.NewValueDefault(&gc.EnableHegel, gc.EnableHegel))
	fs.Register(EnabledTinkServer, ffval.NewValueDefault(&gc.EnableTinkServer, gc.EnableTinkServer))
	fs.Register(EnabledTinkController, ffval.NewValueDefault(&gc.EnableTinkController, gc.EnableTinkController))
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

var EnabledSmee = Config{
	Name:  "enable-smee",
	Usage: "enable Smee service",
}

var EnabledHegel = Config{
	Name:  "enable-hegel",
	Usage: "enable Hegel service",
}

var EnabledTinkServer = Config{
	Name:  "enable-tink-server",
	Usage: "enable Tink Server service",
}

var EnabledTinkController = Config{
	Name:  "enable-tink-controller",
	Usage: "enable Tink Controller service",
}
