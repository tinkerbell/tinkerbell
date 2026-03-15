package flag

import (
	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/pkg/flag/delimitedlist"
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
	fs.Register(TinkControllerReferenceAllowListRules, delimitedlist.New(&t.Config.ReferenceAllowListRules, '|'))
	fs.Register(TinkControllerReferenceDenyListRules, delimitedlist.New(&t.Config.ReferenceDenyListRules, '|'))
}

var TinkControllerEnableLeaderElection = Config{
	Name:  "tink-controller-enable-leader-election",
	Usage: "enable leader election for controller manager",
}

var TinkControllerLeaderElectionNamespace = Config{
	Name:  "tink-controller-leader-election-namespace",
	Usage: "namespace in which the leader election lease will be created",
}

var TinkControllerLogLevel = Config{
	Name:  "tink-controller-log-level",
	Usage: "the higher the number the more verbose, level 0 inherits the global log level, a negative number disables logging",
}

var TinkControllerReferenceAllowListRules = Config{
	Name:  "tink-controller-reference-allow-list-rules",
	Usage: "rules for which Hardware Reference objects are accessible to Templates",
}

var TinkControllerReferenceDenyListRules = Config{
	Name:  "tink-controller-reference-deny-list-rules",
	Usage: "rules for which Hardware Reference objects are not accessible to Templates, defaults to deny all",
}

var TinkControllerMaxConcurrentReconciles = Config{
	Name:  "tink-controller-max-concurrent-reconciles",
	Usage: "maximum number of concurrent reconciles for tink controller",
}
