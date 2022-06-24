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
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/rufio/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(bmcv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var kubeAPIServer string
	var kubeconfig string
	var kubeNamespace string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&kubeAPIServer, "kubernetes", "", "The Kubernetes API URL, used for in-cluster client construction.")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Absolute path to the kubeconfig file.")
	flag.StringVar(&kubeNamespace, "kube-namespace", "", "Namespace that the controller watches to reconcile objects.")

	flag.Parse()

	ctrl.SetLogger(zap.New())

	ccfg := newClientConfig(kubeAPIServer, kubeconfig)

	cfg, err := ccfg.ClientConfig()
	if err != nil {
		setupLog.Error(err, "unable to get client config")
		os.Exit(1)
	}

	if kubeNamespace == "" {
		namespace, _, err := ccfg.Namespace()
		if err != nil {
			setupLog.Error(err, "unable to get client config namespace")
			os.Exit(1)
		}
		kubeNamespace = namespace
	}
	setupLog.Info("Watching objects in namespace for reconciliation", "namespace", kubeNamespace)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "e74dec1a.tinkerbell.org",
		Namespace:              kubeNamespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	// Setup controller reconcilers
	setupReconcilers(ctx, mgr)

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
func setupReconcilers(ctx context.Context, mgr ctrl.Manager) {
	err := (controllers.NewMachineReconciler(
		mgr.GetClient(),
		mgr.GetEventRecorderFor("machine-controller"),
		controllers.NewBMCClientFactoryFunc(ctx),
		ctrl.Log.WithName("controller").WithName("Machine"),
	)).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Machine")
		os.Exit(1)
	}

	err = (controllers.NewJobReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controller").WithName("Job"),
	)).SetupWithManager(ctx, mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Job")
		os.Exit(1)
	}

	err = (controllers.NewTaskReconciler(
		mgr.GetClient(),
		controllers.NewBMCClientFactoryFunc(ctx),
		ctrl.Log.WithName("controller").WithName("Task"),
	)).SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Task")
		os.Exit(1)
	}
}
