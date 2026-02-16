package flag

import (
	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/pkg/flag/netip"
	"github.com/tinkerbell/tinkerbell/rufio"
)

type RufioConfig struct {
	Config   *rufio.Config
	LogLevel int
}

func RegisterRufioFlags(fs *Set, t *RufioConfig) {
	fs.Register(RufioControllerEnableLeaderElection, ffval.NewValueDefault(&t.Config.EnableLeaderElection, t.Config.EnableLeaderElection))
	fs.Register(RufioControllerLeaderElectionNamespace, ffval.NewValueDefault(&t.Config.LeaderElectionNamespace, t.Config.LeaderElectionNamespace))
	fs.Register(RufioControllerMetricsAddr, &netip.AddrPort{AddrPort: &t.Config.MetricsAddr})
	fs.Register(RufioControllerProbeAddr, &netip.AddrPort{AddrPort: &t.Config.ProbeAddr})
	fs.Register(RufioBMCConnectTimeout, ffval.NewValueDefault(&t.Config.BMCConnectTimeout, t.Config.BMCConnectTimeout))
	fs.Register(RufioPowerCheckInterval, ffval.NewValueDefault(&t.Config.PowerCheckInterval, t.Config.PowerCheckInterval))
	fs.Register(RufioMaxConcurrentReconciles, ffval.NewValueDefault(&t.Config.MaxConcurrentReconciles, t.Config.MaxConcurrentReconciles))
	fs.Register(RufioLogLevel, ffval.NewValueDefault(&t.LogLevel, t.LogLevel))
}

var RufioControllerEnableLeaderElection = Config{
	Name:  "rufio-controller-enable-leader-election",
	Usage: "enable leader election for controller manager",
}

var RufioControllerMetricsAddr = Config{
	Name:  "rufio-controller-metrics-addr",
	Usage: "address on which to expose metrics",
}

var RufioControllerProbeAddr = Config{
	Name:  "rufio-controller-probe-addr",
	Usage: "address on which to expose health probes",
}

var RufioControllerLeaderElectionNamespace = Config{
	Name:  "rufio-controller-leader-election-namespace",
	Usage: "namespace in which the leader election lease will be created",
}

var RufioBMCConnectTimeout = Config{
	Name:  "rufio-bmc-connect-timeout",
	Usage: "timeout for BMC connection",
}

var RufioPowerCheckInterval = Config{
	Name:  "rufio-power-check-interval",
	Usage: "interval at which the machine's power state is reconciled",
}

var RufioLogLevel = Config{
	Name:  "rufio-log-level",
	Usage: "the higher the number the more verbose, level 0 inherits the global log level, a negative number disables logging",
}

var RufioMaxConcurrentReconciles = Config{
	Name:  "rufio-max-concurrent-reconciles",
	Usage: "maximum number of concurrent reconciles for rufio controllers",
}
