package flag

import (
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/backend/kube"
	ntip "github.com/tinkerbell/tinkerbell/cmd/flag/netip"
	"github.com/tinkerbell/tinkerbell/tink/server"
)

type TinkServerConfig struct {
	Config   *server.Config
	BindAddr netip.Addr
	BindPort uint16
}

var KubeIndexesTinkServer = map[kube.IndexType]kube.Index{
	kube.IndexTypeWorkflowByNonTerminalState: kube.Indexes[kube.IndexTypeWorkflowByNonTerminalState],
}

func RegisterTinkServerFlags(fs *Set, t *TinkServerConfig) {
	fs.Register(TinkServerBindAddr, &ntip.Addr{Addr: &t.BindAddr})
	fs.Register(TinkServerBindPort, ffval.NewValueDefault(&t.BindPort, t.BindPort))
}

// Convert TinkServerConfig data types to tink server server.Config data types.
func (t *TinkServerConfig) Convert() {
	t.Config.BindAddrPort = netip.AddrPortFrom(t.BindAddr, t.BindPort)
}

var TinkServerBindAddr = Config{
	Name:  "tink-server-bind-addr",
	Usage: "ip address on which the Tink server will listen",
}

var TinkServerBindPort = Config{
	Name:  "tink-server-bind-port",
	Usage: "port on which the Tink server will listen",
}
