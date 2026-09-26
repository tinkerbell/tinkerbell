package flag

import (
	"net/netip"

	"github.com/peterbourgon/ff/v4/ffval"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	ntip "github.com/tinkerbell/tinkerbell/pkg/flag/netip"
	"github.com/tinkerbell/tinkerbell/secondstar"
)

type SecondStarConfig struct {
	Config      *secondstar.Config
	BindAddr    netip.Addr
	BindAddrV6  netip.Addr
	Port        uint16
	PortV6      uint16
	HostKeyPath string
	LogLevel    int
}

var KubeIndexesSecondStar = map[kube.IndexType]kube.Index{
	kube.IndexTypeHardwareName: kube.Indexes[kube.IndexTypeHardwareName],
	kube.IndexTypeMachineName:  kube.Indexes[kube.IndexTypeMachineName],
}

func RegisterSecondStarFlags(fs *Set, ssc *SecondStarConfig) {
	fs.RegisterFamily(SecondStarBindAddr, V4, &ntip.Addr{Addr: &ssc.BindAddr})
	fs.RegisterFamily(SecondStarBindAddr, V6, &ntip.Addr{Addr: &ssc.BindAddrV6})
	fs.RegisterFamily(SecondStarPort, V4, ffval.NewValueDefault(&ssc.Port, ssc.Port))
	fs.RegisterFamily(SecondStarPort, V6, ffval.NewValueDefault(&ssc.PortV6, ssc.PortV6))
	fs.Register(SecondStarHostKey, ffval.NewValueDefault(&ssc.HostKeyPath, ssc.HostKeyPath))
	fs.Register(SecondStarIPMIToolPath, ffval.NewValueDefault(&ssc.Config.IPMITOOLPath, ssc.Config.IPMITOOLPath))
	fs.Register(SecondStarIdleTimeout, ffval.NewValueDefault(&ssc.Config.IdleTimeout, ssc.Config.IdleTimeout))
	fs.Register(SecondStarLogLevel, ffval.NewValueDefault(&ssc.LogLevel, ssc.LogLevel))
}

var SecondStarBindAddr = Config{
	Name:  "secondstar-bind-addr",
	Usage: "address on which to listen for SecondStar",
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
	Usage: logLevelUsage,
}

// Convert CLI specific fields to secondstar.Config fields. A family with no
// service-specific address falls back to the global one.
func (ssc *SecondStarConfig) Convert(bindAddr, bindAddrV6 netip.Addr) error {
	if addr := firstValidAddr(ssc.BindAddr, bindAddr); addr.IsValid() {
		ssc.Config.V4 = netip.AddrPortFrom(addr, ssc.Port)
	}
	if addr := firstValidAddr(ssc.BindAddrV6, bindAddrV6); addr.IsValid() {
		ssc.Config.V6 = netip.AddrPortFrom(addr, ssc.PortV6)
	}

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

// firstValidAddr returns the first address that is set.
func firstValidAddr(addrs ...netip.Addr) netip.Addr {
	for _, a := range addrs {
		if a.IsValid() {
			return a
		}
	}
	return netip.Addr{}
}
