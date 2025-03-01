package apiserver

import (
	"context"
	_ "time/tzdata" // for timeZone support in CronJob

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	"github.com/tinkerbell/tinkerbell/apiserver/internal"
	_ "k8s.io/component-base/logs/json/register"          // for JSON log format registration
	_ "k8s.io/component-base/metrics/prometheus/clientgo" // load all the prometheus client-go plugins
	_ "k8s.io/component-base/metrics/prometheus/version"  // for version metric registration
)

func ConfigAndFlags() (*pflag.FlagSet, func(context.Context, *pflag.FlagSet, logr.Logger) error) {
	return internal.ServerOptionsAndFlags()
}
