package apiserver

import (
	"context"
	_ "time/tzdata" // for timeZone support in CronJob

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/featuregate"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"          // for JSON log format registration
	_ "k8s.io/component-base/metrics/prometheus/clientgo" // load all the prometheus client-go plugins
	_ "k8s.io/component-base/metrics/prometheus/version"  // for version metric registration
	utilversion "k8s.io/component-base/version"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
	apiapp "k8s.io/kubernetes/cmd/kube-apiserver/app"
	apioptions "k8s.io/kubernetes/cmd/kube-apiserver/app/options"
	ctrlapp "k8s.io/kubernetes/cmd/kube-controller-manager/app"
	ctrloptions "k8s.io/kubernetes/cmd/kube-controller-manager/app/options"
)

func ConfigAndFlags() (*pflag.FlagSet, func(context.Context, logr.Logger) error) {
	_, featureGate := featuregate.DefaultComponentGlobalsRegistry.ComponentGlobalsOrRegister(
		featuregate.DefaultKubeComponent, utilversion.DefaultBuildEffectiveVersion(), utilfeature.DefaultMutableFeatureGate)
	s := apioptions.NewServerRunOptions()

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
		featureGate.AddMetrics()

		klog.SetLogger(log)
		return apiapp.Run(ctx, completedOptions)
	}

	fs := pflag.NewFlagSet("kube-apiserver", pflag.ContinueOnError)
	namedFlagSets := s.Flags()
	verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global") /*cmd.Name()*/, "kube-apiserver", logs.SkipLoggingConfigurationFlags())
	apioptions.AddCustomGlobalFlags(namedFlagSets.FlagSet("generic"))
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	return fs, runFunc
}

func Kubecontrollermanager(ctx context.Context, kubeconfig string) error {
	_, _ = featuregate.DefaultComponentGlobalsRegistry.ComponentGlobalsOrRegister(
		featuregate.DefaultKubeComponent, utilversion.DefaultBuildEffectiveVersion(), utilfeature.DefaultMutableFeatureGate)

	s, err := ctrloptions.NewKubeControllerManagerOptions()
	if err != nil {
		return err
	}
	fs := pflag.NewFlagSet("kube-controller-manager", pflag.ExitOnError)
	namedFlagSets := s.Flags(ctrlapp.KnownControllers(), ctrlapp.ControllersDisabledByDefault(), ctrlapp.ControllerAliases())
	verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), "kube-controller-manager", logs.SkipLoggingConfigurationFlags())
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}
	if err := fs.Set("kubeconfig", kubeconfig); err != nil {
		return err
	}
	if err := fs.Set("controllers", "garbage-collector-controller"); err != nil {
		return err
	}

	verflag.PrintAndExitIfRequested()
	cliflag.PrintFlags(fs)

	c, err := s.Config(ctrlapp.KnownControllers(), ctrlapp.ControllersDisabledByDefault(), ctrlapp.ControllerAliases())
	if err != nil {
		return err
	}

	// add feature enablement metrics
	fg := s.ComponentGlobalsRegistry.FeatureGateFor(featuregate.DefaultKubeComponent)
	if f, ok := fg.(featuregate.MutableFeatureGate); ok {
		f.AddMetrics()
	}
	return ctrlapp.Run(ctx, c.Complete())
}
