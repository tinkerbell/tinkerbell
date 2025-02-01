package flag

import (
	"fmt"
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/backend/kube"
	ntip "github.com/tinkerbell/tinkerbell/cmd/flag/netip"
	"github.com/tinkerbell/tinkerbell/hegel"
)

type HegelConfig struct {
	Config   *hegel.Config
	BindAddr netip.Addr
	BindPort int
}

var KubeIndexesHegel = map[kube.IndexType]kube.Index{
	kube.IndexTypeIPAddr: kube.Indexes[kube.IndexTypeIPAddr],
}

func RegisterHegelFlags(fs *Set, h *HegelConfig) {
	fs.Register(HegelBindAddr, &ntip.Addr{Addr: &h.BindAddr})
	fs.Register(HegelBindPort, ffval.NewValueDefault(&h.BindPort, h.BindPort))
	fs.Register(HegelDebugMode, ffval.NewValueDefault(&h.Config.DebugMode, h.Config.DebugMode))
}

var HegelBindAddr = Config{
	Name:  "hegel-bind-addr",
	Usage: "ip address on which the Hegel service will listen",
}

var HegelBindPort = Config{
	Name:  "hegel-bind-port",
	Usage: "port on which the Hegel service will listen",
}

var HegelDebugMode = Config{
	Name:  "hegel-debug-mode",
	Usage: "whether to run Hegel in debug mode",
}

// Convert converts HegelConfig data types to hegel.Config data types.
func (h *HegelConfig) Convert(trustedProxies *[]netip.Prefix) {
	// Convert h.BindAddr and h.BindPort to h.Config.BindAddrPort
	addr, port := splitHostPort(h.Config.BindAddrPort)
	if h.BindAddr.IsValid() {
		addr = h.BindAddr.String()
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
