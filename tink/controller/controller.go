package controller

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	"github.com/tinkerbell/tinkerbell/tink/controller/internal/workflow"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	clog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var schemeBuilder = runtime.NewSchemeBuilder(
	clientgoscheme.AddToScheme,
	api.AddToSchemeTinkerbell,
	api.AddToSchemeBMC,
)

type Config struct {
	Namespace               string
	Client                  *rest.Config
	EnableLeaderElection    bool
	LeaderElectionNamespace string
	MetricsAddr             netip.AddrPort
	ProbeAddr               netip.AddrPort
	DynamicClient           dynamicClient
	ReferenceAllowListRules []string
	ReferenceDenyListRules  []string
}

type dynamicClient interface {
	DynamicRead(ctx context.Context, gvr schema.GroupVersionResource, name, namespace string) (map[string]interface{}, error)
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

func WithDynamicClient(dynamicClient dynamicClient) Option {
	return func(c *Config) {
		c.DynamicClient = dynamicClient
	}
}

func WithEnableLeaderElection(enableLeaderElection bool) Option {
	return func(c *Config) {
		c.EnableLeaderElection = enableLeaderElection
	}
}

func WithMetricsAddr(addrPort netip.AddrPort) Option {
	return func(c *Config) {
		c.MetricsAddr = addrPort
	}
}

func WithProbeAddr(addrPort netip.AddrPort) Option {
	return func(c *Config) {
		c.ProbeAddr = addrPort
	}
}

func WithLeaderElectionNamespace(namespace string) Option {
	return func(c *Config) {
		c.LeaderElectionNamespace = namespace
	}
}

func WithReferenceAllowListRules(rules []string) Option {
	return func(c *Config) {
		c.ReferenceAllowListRules = rules
	}
}

func WithReferenceDenyListRules(rules []string) Option {
	return func(c *Config) {
		c.ReferenceDenyListRules = rules
	}
}

func NewConfig(opts ...Option) *Config {
	defatuls := &Config{
		EnableLeaderElection: true,
	}

	for _, opt := range opts {
		opt(defatuls)
	}

	return defatuls
}

func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	options := controllerruntime.Options{
		Logger:                  log,
		LeaderElection:          c.EnableLeaderElection,
		LeaderElectionID:        "tink-controller.tinkerbell.org",
		LeaderElectionNamespace: c.LeaderElectionNamespace,
		Metrics: server.Options{
			BindAddress: c.MetricsAddr.String(),
		},
		HealthProbeBindAddress: c.ProbeAddr.String(),
	}
	if c.Namespace != "" {
		options.Cache = cache.Options{DefaultNamespaces: map[string]cache.Config{c.Namespace: {}}}
	}

	controllerruntime.SetLogger(log)
	clog.SetLogger(log)

	wfOpts := []workflow.Option{}
	if len(c.ReferenceAllowListRules) > 0 {
		wfOpts = append(wfOpts, workflow.WithAllowReferenceRules(c.ReferenceAllowListRules))
	}
	if len(c.ReferenceDenyListRules) > 0 {
		wfOpts = append(wfOpts, workflow.WithDenyReferenceRules(c.ReferenceDenyListRules))
	}

	mgr, err := newManager(c.Client, c.DynamicClient, options, wfOpts...)
	if err != nil {
		return err
	}

	return mgr.Start(ctx)
}

// NewManager creates a new controller manager with tink controller controllers pre-registered.
// If opts.Scheme is nil, DefaultScheme() is used.
func newManager(cfg *rest.Config, dc dynamicClient, opts controllerruntime.Options, wfOpts ...workflow.Option) (controllerruntime.Manager, error) {
	if opts.Scheme == nil {
		s := runtime.NewScheme()
		_ = schemeBuilder.AddToScheme(s)
		opts.Scheme = s
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

	if err = workflow.NewReconciler(mgr.GetClient(), dc, wfOpts...).SetupWithManager(mgr); err != nil {
		return nil, fmt.Errorf("setup workflow reconciler: %w", err)
	}

	return mgr, nil
}
