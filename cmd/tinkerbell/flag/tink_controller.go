package flag

import (
	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/pkg/flag/netip"
	"github.com/tinkerbell/tinkerbell/pkg/flag/spacelist"
	"github.com/tinkerbell/tinkerbell/tink/controller"
)

type TinkControllerConfig struct {
	Config   *controller.Config
	LogLevel int
}

func RegisterTinkControllerFlags(fs *Set, t *TinkControllerConfig) {
	fs.Register(TinkControllerEnableLeaderElection, ffval.NewValueDefault(&t.Config.EnableLeaderElection, t.Config.EnableLeaderElection))
	fs.Register(TinkControllerLeaderElectionNamespace, ffval.NewValueDefault(&t.Config.LeaderElectionNamespace, t.Config.LeaderElectionNamespace))
	fs.Register(TinkControllerMetricsAddr, &netip.AddrPort{AddrPort: &t.Config.MetricsAddr})
	fs.Register(TinkControllerProbeAddr, &netip.AddrPort{AddrPort: &t.Config.ProbeAddr})
	fs.Register(TinkControllerLogLevel, ffval.NewValueDefault(&t.LogLevel, t.LogLevel))
	fs.Register(TinkControllerReferenceAllowListRules, spacelist.New(&t.Config.ReferenceAllowListRules))
	fs.Register(TinkControllerReferenceDenyListRules, spacelist.New(&t.Config.ReferenceDenyListRules))
}

var TinkControllerEnableLeaderElection = Config{
	Name:  "tink-controller-enable-leader-election",
	Usage: "enable leader election for controller manager",
}

var TinkControllerMetricsAddr = Config{
	Name:  "tink-controller-metrics-addr",
	Usage: "address on which to expose metrics",
}

var TinkControllerProbeAddr = Config{
	Name:  "tink-controller-probe-addr",
	Usage: "address on which to expose health probes",
}

var TinkControllerLeaderElectionNamespace = Config{
	Name:  "tink-controller-leader-election-namespace",
	Usage: "namespace in which the leader election lease will be created",
}

var TinkControllerLogLevel = Config{
	Name:  "tink-controller-log-level",
	Usage: "the higher the number the more verbose, level 0 inherits the global log level",
}

var TinkControllerReferenceAllowListRules = Config{
	Name:  "tink-controller-reference-allow-list-rules",
	Usage: "rules for which Hardware Reference objects are accessible to Templates",
}

var TinkControllerReferenceDenyListRules = Config{
	Name:  "tink-controller-reference-deny-list-rules",
	Usage: "rules for which Hardware Reference objects are not accessible to Templates, defaults to deny all",
}
