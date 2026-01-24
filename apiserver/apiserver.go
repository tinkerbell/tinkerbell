//go:build embedded
// +build embedded

package apiserver

import (
	"context"
	_ "time/tzdata" // for timeZone support in CronJob

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/component-base/cli/globalflag"
	basecompatibility "k8s.io/component-base/compatibility"
	"k8s.io/component-base/featuregate"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"          // for JSON log format registration
	_ "k8s.io/component-base/metrics/prometheus/clientgo" // load all the prometheus client-go plugins
	_ "k8s.io/component-base/metrics/prometheus/version"  // for version metric registration
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
	apiapp "k8s.io/kubernetes/cmd/kube-apiserver/app"
	apioptions "k8s.io/kubernetes/cmd/kube-apiserver/app/options"
	ctrlapp "k8s.io/kubernetes/cmd/kube-controller-manager/app"
	ctrloptions "k8s.io/kubernetes/cmd/kube-controller-manager/app/options"
)

func ConfigAndFlags(disableLogging *bool) (*pflag.FlagSet, func(context.Context, logr.Logger) error) {
	s := apioptions.NewServerRunOptions()
	featureGate := s.GenericServerRunOptions.ComponentGlobalsRegistry.FeatureGateFor(basecompatibility.DefaultKubeComponent)

	runFunc := func(ctx context.Context, log logr.Logger) error {
		// Activate logging as soon as possible, after that
		// show flags with the final logging configuration.
		if err := logsapi.ValidateAndApply(s.Logs, featureGate); err != nil {
			return err
		}
		// cliflag.PrintFlags(fs)

		// set default options
		completedOptions, err := s.Complete(ctx)
		if err != nil {
			return err
		}

		// validate options
		if errs := completedOptions.Validate(); len(errs) != 0 {
			return utilerrors.NewAggregate(errs)
		}
		// add feature enablement metrics
		if f, ok := featureGate.(featuregate.MutableFeatureGate); ok {
			f.AddMetrics()
		}

		if disableLogging != nil && *disableLogging {
			log = logr.Discard()
		}
		klog.SetLogger(log)
		return apiapp.Run(ctx, completedOptions)
	}

	fs := pflag.NewFlagSet("kube-apiserver", pflag.ContinueOnError)
	namedFlagSets := s.Flags()
	verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), "kube-apiserver", logs.SkipLoggingConfigurationFlags())
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	return fs, runFunc
}

func Kubecontrollermanager(ctx context.Context, kubeconfig string) error {
	s, err := ctrloptions.NewKubeControllerManagerOptions()
	if err != nil {
		return err
	}

	// Directly configure the options instead of using flags to avoid
	// ComponentGlobalsRegistry conflicts with the apiserver
	s.Generic.ClientConnection.Kubeconfig = kubeconfig
	s.Generic.Controllers = []string{"garbage-collector-controller"}

	// Apply ComponentGlobalsRegistry settings
	if err := s.ComponentGlobalsRegistry.Set(); err != nil {
		return err
	}

	c, err := s.Config(ctx, ctrlapp.KnownControllers(), ctrlapp.ControllersDisabledByDefault(), ctrlapp.ControllerAliases())
	if err != nil {
		return err
	}

	// add feature enablement metrics
	fg := s.ComponentGlobalsRegistry.FeatureGateFor(basecompatibility.DefaultKubeComponent)
	if f, ok := fg.(featuregate.MutableFeatureGate); ok {
		f.AddMetrics()
	}
	return ctrlapp.Run(ctx, c.Complete())
}
