package flag

import (
	"fmt"
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	ntip "github.com/tinkerbell/tinkerbell/pkg/flag/netip"
	"github.com/tinkerbell/tinkerbell/tootles"
)

type TootlesConfig struct {
	Config   *tootles.Config
	BindAddr netip.Addr
	BindPort int
	LogLevel int
}

var KubeIndexesTootles = map[kube.IndexType]kube.Index{
	kube.IndexTypeIPAddr:     kube.Indexes[kube.IndexTypeIPAddr],
	kube.IndexTypeInstanceID: kube.Indexes[kube.IndexTypeInstanceID],
}

func RegisterTootlesFlags(fs *Set, h *TootlesConfig) {
	fs.Register(TootlesBindAddr, &ntip.Addr{Addr: &h.BindAddr})
	fs.Register(TootlesBindPort, ffval.NewValueDefault(&h.BindPort, h.BindPort))
	fs.Register(TootlesDebugMode, ffval.NewValueDefault(&h.Config.DebugMode, h.Config.DebugMode))
	fs.Register(TootlesLogLevel, ffval.NewValueDefault(&h.LogLevel, h.LogLevel))
	fs.Register(TootlesInstanceEndpoint, ffval.NewValueDefault(&h.Config.InstanceEndpoint, h.Config.InstanceEndpoint))
}

var TootlesBindAddr = Config{
	Name:  "tootles-bind-addr",
	Usage: "ip address on which the Tootles service will listen",
}

var TootlesBindPort = Config{
	Name:  "tootles-bind-port",
	Usage: "port on which the Tootles service will listen",
}

var TootlesDebugMode = Config{
	Name:  "tootles-debug-mode",
	Usage: "whether to run Tootles in debug mode",
}

var TootlesLogLevel = Config{
	Name:  "tootles-log-level",
	Usage: "the higher the number the more verbose, level 0 inherits the global log level",
}

var TootlesInstanceEndpoint = Config{
	Name:  "tootles-instance-endpoint",
	Usage: "whether to enable /tootles/instanceID/<instanceID> endpoint that is independent from client IP address",
}

// Convert converts TootlesConfig data types to tootles.Config data types.
func (h *TootlesConfig) Convert(trustedProxies *[]netip.Prefix, bindAddr netip.Addr) {
	// Convert h.BindAddr and h.BindPort to h.Config.BindAddrPort
	addr, port := splitHostPort(h.Config.BindAddrPort)
	if h.BindAddr.IsValid() {
		addr = h.BindAddr.String()
	}
	if bindAddr.IsValid() {
		addr = bindAddr.String()
	}
	if h.BindPort != 0 {
		port = fmt.Sprintf("%d", h.BindPort)
	}
	if port != "" {
		h.Config.BindAddrPort = fmt.Sprintf("%s:%s", addr, port)
	} else {
		h.Config.BindAddrPort = addr
	}

	if trustedProxies != nil && len(*trustedProxies) > 0 {
		h.Config.TrustedProxies = ntip.ToPrefixList(trustedProxies).String()
	}
}
