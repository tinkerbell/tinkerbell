package flag

import (
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	ntip "github.com/tinkerbell/tinkerbell/pkg/flag/netip"
	"github.com/tinkerbell/tinkerbell/tink/server"
)

type TinkServerConfig struct {
	Config   *server.Config
	BindAddr netip.Addr
	BindPort uint16
	LogLevel int
}

var KubeIndexesTinkServer = map[kube.IndexType]kube.Index{
	kube.IndexTypeWorkflowAgentID: kube.Indexes[kube.IndexTypeWorkflowAgentID],
}

func RegisterTinkServerFlags(fs *Set, t *TinkServerConfig) {
	fs.Register(TinkServerBindAddr, &ntip.Addr{Addr: &t.BindAddr})
	fs.Register(TinkServerBindPort, ffval.NewValueDefault(&t.BindPort, t.BindPort))
	fs.Register(TinkServerLogLevel, ffval.NewValueDefault(&t.LogLevel, t.LogLevel))
	fs.Register(TinkServerAutoEnrollmentEnabled, ffval.NewValueDefault(&t.Config.AutoEnrollmentEnabled, t.Config.AutoEnrollmentEnabled))
	fs.Register(TinkerbellAutoDiscoveryNamespace, ffval.NewValueDefault(&t.Config.AutoDiscoveryNamespace, t.Config.AutoDiscoveryNamespace))
	fs.Register(TinkerbellAutoDiscoveryEnabled, ffval.NewValueDefault(&t.Config.AutoDiscoveryEnabled, t.Config.AutoDiscoveryEnabled))
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

var TinkServerLogLevel = Config{
	Name:  "tink-server-log-level",
	Usage: "the higher the number the more verbose, level 0 inherits the global log level",
}

var TinkServerAutoEnrollmentEnabled = Config{
	Name:  "tink-server-auto-enrollment-enabled",
	Usage: "enable auto enrollment capabilities for the Tink server",
}

var TinkerbellAutoDiscoveryEnabled = Config{
	Name:  "tink-server-auto-discovery-enabled",
	Usage: "enable auto discovery capabilities for the Tink server",
}

var TinkerbellAutoDiscoveryNamespace = Config{
	Name:  "tink-server-auto-discovery-namespace",
	Usage: "namespace in which the Tink server will create auto discovered Hardware objects",
}
