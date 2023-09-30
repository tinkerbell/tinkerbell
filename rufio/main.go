/*
Copyright 2022 Tinkerbell.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/rs/zerolog"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/rufio/controller"
	//+kubebuilder:scaffold:imports
)

const (
	appName = "rufio"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// defaultLogger is a zerolog logr implementation.
func defaultLogger(level string) logr.Logger {
	zl := zerolog.New(os.Stdout)
	zl = zl.With().Caller().Timestamp().Logger()
	var l zerolog.Level
	switch level {
	case "debug":
		l = zerolog.TraceLevel
	default:
		l = zerolog.InfoLevel
	}
	zl = zl.Level(l)

	return zerologr.New(&zl)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var kubeAPIServer string
	var kubeconfig string
	var kubeNamespace string
	var bmcConnectTimeout time.Duration
	fs := flag.NewFlagSet(appName, flag.ExitOnError)
	fs.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	fs.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&kubeAPIServer, "kubernetes", "", "The Kubernetes API URL, used for in-cluster client construction.")
	fs.StringVar(&kubeconfig, "kubeconfig", "", "Absolute path to the kubeconfig file.")
	fs.StringVar(&kubeNamespace, "kube-namespace", "", "Namespace that the controller watches to reconcile objects.")
	fs.DurationVar(&bmcConnectTimeout, "bmc-connect-timeout", 60*time.Second, "Timeout for establishing a connection to BMCs.")
	cli := &ffcli.Command{
		Name:    appName,
		FlagSet: fs,
		Options: []ff.Option{ff.WithEnvVarPrefix(appName)},
	}

	_ = cli.Parse(os.Args[1:])

	ctrl.SetLogger(defaultLogger("debug"))

	ccfg := newClientConfig(kubeAPIServer, kubeconfig)

	cfg, err := ccfg.ClientConfig()
	if err != nil {
		setupLog.Error(err, "unable to get client config")
		os.Exit(1)
	}

	setupLog.Info("Watching objects in namespace for reconciliation", "namespace", kubeNamespace)

	opts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "e74dec1a.tinkerbell.org",
	}
	// If a namespace is specified, only watch that namespace. Otherwise, watch all namespaces.
	if kubeNamespace != "" {
		opts.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{kubeNamespace: {}},
		}
	}

	mgr, err := ctrl.NewManager(cfg, opts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	bmcClientFactory := controller.NewClientFunc(bmcConnectTimeout)

	// Setup controller reconcilers
	setupReconcilers(ctx, mgr, bmcClientFactory)

	//+kubebuilder:scaffold:builder

	err = mgr.AddHealthzCheck("healthz", healthz.Ping)
	if err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	err = mgr.AddReadyzCheck("readyz", healthz.Ping)
	if err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	err = mgr.Start(ctx)
	if err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func newClientConfig(kubeAPIServer, kubeconfig string) clientcmd.ClientConfig {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: kubeAPIServer}})
}

// setupReconcilers initializes the controllers with the Manager.
func setupReconcilers(ctx context.Context, mgr ctrl.Manager, bmcClientFactory controller.ClientFunc) {
	err := (controller.NewMachineReconciler(
		mgr.GetClient(),
		mgr.GetEventRecorderFor("machine-controller"),
		bmcClientFactory,
	)).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Machine")
		os.Exit(1)
	}

	err = (controller.NewJobReconciler(
		mgr.GetClient(),
	)).SetupWithManager(ctx, mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Job")
		os.Exit(1)
	}

	err = (controller.NewTaskReconciler(
		mgr.GetClient(),
		bmcClientFactory,
	)).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Task")
		os.Exit(1)
	}
}
