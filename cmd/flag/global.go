package flag

import (
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	ntip "github.com/tinkerbell/tinkerbell/cmd/flag/netip"
)

type GlobalConfig struct {
	LogLevel             string
	Backend              string
	BackendFilePath      string
	BackendKubeConfig    string
	BackendKubeNamespace string
	OTELEndpoint         string
	OTELInsecure         bool
	TrustedProxies       []netip.Prefix
	PublicIP             netip.Addr
}

func RegisterGlobal(fs *Set, gc *GlobalConfig) {
	fs.Register(LogLevelConfig, ffval.NewEnum(&gc.LogLevel, "info", "debug"))
	fs.Register(BackendConfig, ffval.NewEnum(&gc.Backend, "kube", "file", "none"))
	fs.Register(BackendFilePath, ffval.NewValueDefault(&gc.BackendFilePath, gc.BackendFilePath))
	fs.Register(BackendKubeConfig, ffval.NewValueDefault(&gc.BackendKubeConfig, gc.BackendKubeConfig))
	fs.Register(BackendKubeNamespace, ffval.NewValueDefault(&gc.BackendKubeNamespace, gc.BackendKubeNamespace))
	fs.Register(OTELEndpoint, ffval.NewValueDefault(&gc.OTELEndpoint, gc.OTELEndpoint))
	fs.Register(OTELInsecure, ffval.NewValueDefault(&gc.OTELInsecure, gc.OTELInsecure))
	fs.Register(TrustedProxies, &ntip.PrefixList{PrefixList: &gc.TrustedProxies})
	fs.Register(PublicIP, &ntip.Addr{Addr: &gc.PublicIP})
}

// All these flags are used by at least two services or
// are used to create objects that are used by multiple services.
var LogLevelConfig = Config{
	Name:  "log-level",
	Usage: "log level",
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
