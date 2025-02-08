package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/tinkerbell/tinkerbell/api/bmc/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

var schemeBuilder = runtime.NewSchemeBuilder(
	scheme.AddToScheme,
	v1alpha1.AddToScheme,
)

// DefaultScheme returns a scheme with all the types necessary for the Rufio controller.
func DefaultScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = schemeBuilder.AddToScheme(s)
	return s
}

// Reconciler is a type for managing Workflows.
type Reconciler struct {
	client  client.Client
	nowFunc func() time.Time
	backoff *backoff.ExponentialBackOff
}

func NewManager(cfg *rest.Config, opts controllerruntime.Options) (controllerruntime.Manager, error) {
	if opts.Scheme == nil {
		opts.Scheme = DefaultScheme()
	}

	mgr, err := controllerruntime.NewManager(cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("controller manager: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("set up health check: %w", err)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("set up ready check: %w", err)
	}

	if err := NewReconciler(mgr.GetClient()).SetupWithManager(context.Background(), mgr, NewClientFunc(time.Minute)); err != nil {
		return nil, fmt.Errorf("unable to create reconciler: %w", err)
	}

	return mgr, nil
}

// TODO(jacobweinstock): add functional arguments to the signature.
// TODO(jacobweinstock): write functional argument for customizing the backoff.
func NewReconciler(client client.Client) *Reconciler {
	return &Reconciler{
		client:  client,
		nowFunc: time.Now,
		backoff: backoff.NewExponentialBackOff([]backoff.ExponentialBackOffOpts{
			backoff.WithMaxInterval(5 * time.Second), // this should keep all NextBackOff's under 10 seconds
		}...),
	}
}

func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, bmcClientFactory ClientFunc) error {
	if err := NewMachineReconciler(mgr.GetClient(), mgr.GetEventRecorderFor("machine-controller"), bmcClientFactory).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create Machines controller: %w", err)
	}

	if err := NewJobReconciler(mgr.GetClient()).SetupWithManager(ctx, mgr); err != nil {
		return fmt.Errorf("unable to create Jobs controller: %w", err)
	}

	if err := NewTaskReconciler(mgr.GetClient(), bmcClientFactory).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create Tasks controller: %w", err)
	}

	return nil
}
