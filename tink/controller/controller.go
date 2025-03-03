package controller

import (
	"context"
	"net/netip"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/tink/controller/internal/controller"
	"k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	clog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type Config struct {
	Namespace               string
	Client                  *rest.Config
	EnableLeaderElection    bool
	LeaderElectionNamespace string
	MetricsAddr             netip.AddrPort
	ProbeAddr               netip.AddrPort
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

	mgr, err := controller.NewManager(c.Client, options)
	if err != nil {
		return err
	}

	return mgr.Start(ctx)
}
