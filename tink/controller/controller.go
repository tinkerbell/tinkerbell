package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/tink/controller/internal/workflow"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var schemeBuilder = runtime.NewSchemeBuilder(
	clientgoscheme.AddToScheme,
	tinkerbell.AddToScheme,
	bmc.AddToScheme,
)

type Config struct {
	Namespace               string
	Client                  *rest.Config
	EnableLeaderElection    bool
	LeaderElectionNamespace string
	HardwareReader          renderedHardwareReader
	MaxConcurrentReconciles int
}

type renderedHardwareReader interface {
	RenderedHardware(ctx context.Context, hw *tinkerbell.Hardware) (*tinkerbell.Hardware, error)
	ResolveReferences(ctx context.Context, hw *tinkerbell.Hardware) (map[string]any, error)
}

type Option func(*Config)

func WithNamespace(namespace string) Option {
	return func(c *Config) {
		c.Namespace = namespace
	}
}

func WithClient(client *rest.Config) Option {
	return func(c *Config) {
		c.Client = client
	}
}

func WithHardwareReader(r renderedHardwareReader) Option {
	return func(c *Config) {
		c.HardwareReader = r
	}
}

func WithEnableLeaderElection(enableLeaderElection bool) Option {
	return func(c *Config) {
		c.EnableLeaderElection = enableLeaderElection
	}
}

func WithLeaderElectionNamespace(namespace string) Option {
	return func(c *Config) {
		c.LeaderElectionNamespace = namespace
	}
}

func NewConfig(opts ...Option) *Config {
	defatuls := &Config{
		EnableLeaderElection:    true,
		MaxConcurrentReconciles: 1,
	}

	for _, opt := range opts {
		opt(defatuls)
	}

	return defatuls
}

func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	if c.HardwareReader == nil {
		return errors.New("hardware reader is required")
	}

	options := controllerruntime.Options{
		Logger:                  log,
		LeaderElection:          c.EnableLeaderElection,
		LeaderElectionID:        "tink-controller.tinkerbell.org",
		LeaderElectionNamespace: c.LeaderElectionNamespace,
		Metrics: server.Options{
			BindAddress: "0",
		},
		HealthProbeBindAddress: "0",
	}
	if c.Namespace != "" {
		options.Cache = cache.Options{DefaultNamespaces: map[string]cache.Config{c.Namespace: {}}}
	}

	mgr, err := newManager(c.Client, c.HardwareReader, options, c.MaxConcurrentReconciles)
	if err != nil {
		return err
	}

	return mgr.Start(ctx)
}

// NewManager creates a new controller manager with tink controller controllers pre-registered.
// If opts.Scheme is nil, DefaultScheme() is used.
func newManager(cfg *rest.Config, hardwareReader renderedHardwareReader, opts controllerruntime.Options, maxConcurrentReconciles int) (controllerruntime.Manager, error) {
	if opts.Scheme == nil {
		s := runtime.NewScheme()
		_ = schemeBuilder.AddToScheme(s)
		opts.Scheme = s
	}

	mgr, err := controllerruntime.NewManager(cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("controller manager: %w", err)
	}

	if err = workflow.NewReconciler(mgr.GetClient(), hardwareReader).SetupWithManager(mgr, ctrlcontroller.Options{MaxConcurrentReconciles: maxConcurrentReconciles}); err != nil {
		return nil, fmt.Errorf("setup workflow reconciler: %w", err)
	}

	return mgr, nil
}
