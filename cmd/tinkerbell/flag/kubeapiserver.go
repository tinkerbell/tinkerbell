//go:build embedded
// +build embedded

package flag

import "github.com/peterbourgon/ff/v4/ffval"

type EmbeddedKubeAPIServerConfig struct {
	DisableLogging bool
}

func RegisterKubeAPIServer(fs *Set, ec *EmbeddedKubeAPIServerConfig) {
	fs.Register(KubeAPIServerDisableLogging, ffval.NewValueDefault(&ec.DisableLogging, ec.DisableLogging))
}

var KubeAPIServerDisableLogging = Config{
	Name:  "kubeapi-server-disable-logging",
	Usage: "disable logging for kube-apiserver",
}
