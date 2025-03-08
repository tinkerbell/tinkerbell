//go:build embedded
// +build embedded

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
}

func RegisterEtcd(fs *Set, ec *EmbeddedEtcdConfig) {
	fs.Register(EtcdDir, ffval.NewValueDefault(&ec.Config.Dir, ec.Config.Dir))
	fs.Register(EtcdWaitHealthyTimeout, ffval.NewValueDefault(&ec.WaitHealthyTimeout, ec.WaitHealthyTimeout))
	fs.Register(EtcdLogLevel, ffval.NewValueDefault(&ec.LogLevel, ec.LogLevel))
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
