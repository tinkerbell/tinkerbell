package flag

import (
	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/tink/controller"
)

type TinkControllerConfig struct {
	Config   *controller.Config
	LogLevel int
}

func RegisterTinkControllerFlags(fs *Set, t *TinkControllerConfig) {
	fs.Register(TinkControllerEnableLeaderElection, ffval.NewValueDefault(&t.Config.EnableLeaderElection, t.Config.EnableLeaderElection))
	fs.Register(TinkControllerLeaderElectionNamespace, ffval.NewValueDefault(&t.Config.LeaderElectionNamespace, t.Config.LeaderElectionNamespace))
	fs.Register(TinkControllerMaxConcurrentReconciles, ffval.NewValueDefault(&t.Config.MaxConcurrentReconciles, t.Config.MaxConcurrentReconciles))
	fs.Register(TinkControllerLogLevel, ffval.NewValueDefault(&t.LogLevel, t.LogLevel))
}

var TinkControllerEnableLeaderElection = Config{
	Name:  "tink-controller-enable-leader-election",
	Usage: "enable leader election for controller manager",
}

var TinkControllerLeaderElectionNamespace = Config{
	Name:  "tink-controller-leader-election-namespace",
	Usage: "namespace in which the leader election lease will be created; empty means the backend namespace when that is set",
}

var TinkControllerLogLevel = Config{
	Name:  "tink-controller-log-level",
	Usage: logLevelUsage,
}

var TinkControllerMaxConcurrentReconciles = Config{
	Name:  "tink-controller-max-concurrent-reconciles",
	Usage: "maximum number of concurrent reconciles for tink controller",
}
