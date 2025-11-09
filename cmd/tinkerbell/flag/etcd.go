//go:build embedded

package flag

import (
	"time"

	"github.com/peterbourgon/ff/v4/ffval"
	"go.etcd.io/etcd/server/v3/embed"
)

type EmbeddedEtcdConfig struct {
	Config             *embed.Config
	WaitHealthyTimeout time.Duration
	LogLevel           int
	DisableLogging     bool
}

func RegisterEtcd(fs *Set, ec *EmbeddedEtcdConfig) {
	fs.Register(EtcdDir, ffval.NewValueDefault(&ec.Config.Dir, ec.Config.Dir))
	fs.Register(EtcdWaitHealthyTimeout, ffval.NewValueDefault(&ec.WaitHealthyTimeout, ec.WaitHealthyTimeout))
	fs.Register(EtcdLogLevel, ffval.NewValueDefault(&ec.LogLevel, ec.LogLevel))
	fs.Register(EtcdDisableLogging, ffval.NewValueDefault(&ec.DisableLogging, ec.DisableLogging))
}

var EtcdDir = Config{
	Name:  "etcd-dir",
	Usage: "the directory to store etcd data",
}

var EtcdWaitHealthyTimeout = Config{
	Name:  "etcd-wait-healthy-timeout",
	Usage: "the timeout to wait for etcd to become healthy",
}

var EtcdLogLevel = Config{
	Name:  "etcd-log-level",
	Usage: "log level for etcd",
}

var EtcdDisableLogging = Config{
	Name:  "etcd-disable-logging",
	Usage: "disable logging for etcd",
}
