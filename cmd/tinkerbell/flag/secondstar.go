package flag

import (
	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/secondstar"
)

type SecondStarConfig struct {
	Config      *secondstar.Config
	HostKeyPath string
	LogLevel    int
}

var KubeIndexesSecondStar = map[kube.IndexType]kube.Index{
	kube.IndexTypeHardwareName: kube.Indexes[kube.IndexTypeHardwareName],
	kube.IndexTypeMachineName:  kube.Indexes[kube.IndexTypeMachineName],
}

func RegisterSecondStarFlags(fs *Set, ssc *SecondStarConfig) {
	fs.Register(SecondStarPort, ffval.NewValueDefault(&ssc.Config.SSHPort, ssc.Config.SSHPort))
	fs.Register(SecondStarHostKey, ffval.NewValueDefault(&ssc.HostKeyPath, ssc.HostKeyPath))
	fs.Register(SecondStarIPMIToolPath, ffval.NewValueDefault(&ssc.Config.IPMITOOLPath, ssc.Config.IPMITOOLPath))
	fs.Register(SecondStarIdleTimeout, ffval.NewValueDefault(&ssc.Config.IdleTimeout, ssc.Config.IdleTimeout))
	fs.Register(SecondStarLogLevel, ffval.NewValueDefault(&ssc.LogLevel, ssc.LogLevel))
}

var SecondStarPort = Config{
	Name:  "secondstar-port",
	Usage: "Port to listen on for SecondStar",
}

var SecondStarHostKey = Config{
	Name:  "secondstar-host-key",
	Usage: "Path to the host key file for SecondStar",
}

var SecondStarIPMIToolPath = Config{
	Name:  "secondstar-ipmitool-path",
	Usage: "Path to the ipmitool binary",
}

var SecondStarIdleTimeout = Config{
	Name:  "secondstar-idle-timeout",
	Usage: "Idle timeout for SecondStar",
}

var SecondStarLogLevel = Config{
	Name:  "secondstar-log-level",
	Usage: "the higher the number the more verbose, level 0 inherits the global log level",
}

func (ssc *SecondStarConfig) Convert() error {
	// convert the host key path to an SSH Signer
	if ssc.HostKeyPath == "" {
		return nil
	}
	s, err := secondstar.HostKeyFrom(ssc.HostKeyPath)
	if err != nil {
		return err
	}
	ssc.Config.HostKey = s
	return nil
}
