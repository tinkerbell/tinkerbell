/*
Copyright 2022 The Tinkerbell Authors.

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
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/zerologr"
	"github.com/rs/zerolog"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	cgrecord "k8s.io/client-go/tools/record"
	"k8s.io/component-base/version"
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	rufiov1 "github.com/tinkerbell/rufio/api/v1alpha1"
	tinkv1 "github.com/tinkerbell/tink/api/v1alpha1"

	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	infrastructurev1 "github.com/tinkerbell/cluster-api-provider-tinkerbell/api/v1beta1"
	"github.com/tinkerbell/cluster-api-provider-tinkerbell/controller/cluster"
	"github.com/tinkerbell/cluster-api-provider-tinkerbell/controller/machine"
	// +kubebuilder:scaffold:imports
)

//nolint:gochecknoglobals
var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

//nolint:wsl,gochecknoinits
func init() {
	klog.InitFlags(nil)

	_ = clientgoscheme.AddToScheme(scheme)
	_ = infrastructurev1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = tinkv1.AddToScheme(scheme)
	_ = rufiov1.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
}

//nolint:gochecknoglobals
var (
	enableLeaderElection          bool
	metricsAddr                   string
	leaderElectionNamespace       string
	watchNamespace                string
	profilerAddress               string
	healthAddr                    string
	watchFilterValue              string
	webhookCertDir                string
	tinkerbellClusterConcurrency  int
	tinkerbellMachineConcurrency  int
	tinkerbellHardwareConcurrency int
	tinkerbellTemplateConcurrency int
	tinkerbellWorkflowConcurrency int
	webhookPort                   int
	syncPeriod                    time.Duration
	leaderElectionLeaseDuration   time.Duration
	leaderElectionRenewDeadline   time.Duration
	leaderElectionRetryPeriod     time.Duration
)

func initFlags(fs *pflag.FlagSet) { //nolint:funlen
	fs.StringVar(
		&metricsAddr,
		"metrics-bind-addr",
		"localhost:8080",
		"The address the metric endpoint binds to.",
	)

	fs.BoolVar(
		&enableLeaderElection,
		"leader-elect",
		false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.", //nolint:lll
	)

	fs.DurationVar(
		&leaderElectionLeaseDuration,
		"leader-elect-lease-duration",
		15*time.Second, //nolint:gomnd
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)",
	)

	fs.DurationVar(
		&leaderElectionRenewDeadline,
		"leader-elect-renew-deadline",
		10*time.Second, //nolint:gomnd
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)",
	)

	fs.DurationVar(
		&leaderElectionRetryPeriod,
		"leader-elect-retry-period",
		2*time.Second, //nolint:gomnd
		"Duration the LeaderElector clients should wait between tries of actions (duration string)",
	)

	fs.StringVar(
		&watchNamespace,
		"namespace",
		"",
		"Namespace that the controller watches to reconcile cluster-api objects. If unspecified, the controller watches for cluster-api objects across all namespaces.", //nolint:lll
	)

	fs.StringVar(
		&leaderElectionNamespace,
		"leader-election-namespace",
		"",
		"Namespace that the controller performs leader election in. If unspecified, the controller will discover which namespace it is running in.", //nolint:lll
	)

	fs.StringVar(
		&profilerAddress,
		"profiler-address",
		"",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)",
	)

	fs.StringVar(
		&watchFilterValue,
		"watch-filter",
		"",
		fmt.Sprintf("Label value that the controller watches to reconcile cluster-api objects. Label key is always %s. If unspecified, the controller watches for all cluster-api objects.", clusterv1.WatchLabel), //nolint:lll
	)

	fs.IntVar(&tinkerbellClusterConcurrency,
		"tinkerbellcluster-concurrency",
		10, //nolint:gomnd
		"Number of TinkerbellClusters to process simultaneously",
	)

	fs.IntVar(&tinkerbellMachineConcurrency,
		"tinkerbellmachine-concurrency",
		10, //nolint:gomnd
		"Number of TinkerbellMachines to process simultaneously",
	)

	fs.IntVar(&tinkerbellHardwareConcurrency,
		"tinkerbell-hardware-concurrency",
		10, //nolint:gomnd
		"Number of Tinkerbell Hardware resources to process simultaneously",
	)

	fs.IntVar(&tinkerbellTemplateConcurrency,
		"tinkerbell-template-concurrency",
		10, //nolint:gomnd
		"Number of Tinkerbell Template resources to process simultaneously",
	)

	fs.IntVar(&tinkerbellWorkflowConcurrency,
		"tinkerbell-workflow-concurrency",
		10, //nolint:gomnd
		"Number of Tinkerbell Workflow resources to process simultaneously",
	)

	fs.DurationVar(&syncPeriod,
		"sync-period",
		10*time.Minute, //nolint:gomnd
		"The minimum interval at which watched resources are reconciled (e.g. 15m)",
	)

	fs.IntVar(&webhookPort,
		"webhook-port",
		9443, //nolint:gomnd
		"Webhook Server port",
	)

	fs.StringVar(&webhookCertDir,
		"webhook-cert-dir",
		"/tmp/k8s-webhook-server/serving-certs",
		"Webhook Server Certificate Directory, is the directory that contains the server key and certificate",
	)

	fs.StringVar(&healthAddr,
		"health-addr",
		":9440",
		"The address the health endpoint binds to.",
	)
}

func addHealthChecks(mgr ctrl.Manager) error {
	if err := mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		return fmt.Errorf("unable to create ready check: %w", err)
	}

	if err := mgr.AddHealthzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		return fmt.Errorf("unable to create healthz check: %w", err)
	}

	return nil
}

func setupReconcilers(ctx context.Context, mgr ctrl.Manager) error {
	if err := (&cluster.TinkerbellClusterReconciler{
		Client:           mgr.GetClient(),
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: tinkerbellClusterConcurrency}); err != nil {
		return fmt.Errorf("unable to setup TinkerbellCluster controller:%w", err)
	}

	if err := (&machine.TinkerbellMachineReconciler{
		Client:           mgr.GetClient(),
		WatchFilterValue: watchFilterValue,
	}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: tinkerbellMachineConcurrency}); err != nil {
		return fmt.Errorf("unable to setup TinkerbellMachine controller:%w", err)
	}

	return nil
}

func setupWebhooks(mgr ctrl.Manager) error {
	if err := (&infrastructurev1.TinkerbellCluster{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("unable to setup TinkerbellCluster webhook:%w", err)
	}

	if err := (&infrastructurev1.TinkerbellMachine{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("unable to setup TinkerbellMachine webhook:%w", err)
	}

	if err := (&infrastructurev1.TinkerbellMachineTemplate{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("unable to setup TinkerbellMachineTemplate webhook:%w", err)
	}

	return nil
}

func main() { //nolint:funlen
	initFlags(pflag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	if watchNamespace != "" {
		setupLog.Info("Watching cluster-api objects only in namespace for reconciliation", "namespace", watchNamespace)
	}

	if profilerAddress != "" {
		setupLog.Info("Profiler listening for requests", "profiler-address", profilerAddress)

		go func() {
			//nolint:gosec
			setupLog.Error(http.ListenAndServe(profilerAddress, nil), "listen and serve error")
		}()
	}

	zl := zerolog.New(os.Stdout).Level(zerolog.InfoLevel).With().Caller().Timestamp().Logger()
	logger := zerologr.New(&zl)
	ctrl.SetLogger(logger)
	klog.SetLogger(logger)

	// Machine and cluster operations can create enough events to trigger the event recorder spam filter
	// Setting the burst size higher ensures all events will be recorded and submitted to the API
	broadcaster := cgrecord.NewBroadcasterWithCorrelatorOptions(cgrecord.CorrelatorOptions{
		BurstSize: 100, //nolint:gomnd
	})
	opts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "controller-leader-election-capt",
		LeaderElectionNamespace: leaderElectionNamespace,
		LeaseDuration:           &leaderElectionLeaseDuration,
		RenewDeadline:           &leaderElectionRenewDeadline,
		RetryPeriod:             &leaderElectionRetryPeriod,
		HealthProbeBindAddress:  healthAddr,
		EventBroadcaster:        broadcaster,
	}

	if watchNamespace != "" {
		opts.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{watchNamespace: {}},
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), opts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Initialize event recorder.
	record.InitFromRecorder(mgr.GetEventRecorderFor("tinkerbell-controller"))

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	if err := setupReconcilers(ctx, mgr); err != nil {
		setupLog.Error(err, "failed to add Tinkerbell Reconcilers")
		os.Exit(1)
	}

	if err := setupWebhooks(mgr); err != nil {
		setupLog.Error(err, "failed to add Tinkerbell Webhooks")
		os.Exit(1)
	}

	if err := addHealthChecks(mgr); err != nil {
		setupLog.Error(err, "failed to add health checks")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder
	setupLog.Info("starting manager", "version", version.Get().String())

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
