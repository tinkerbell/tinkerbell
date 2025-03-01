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

package rufio

import (
	"context"
	"net/netip"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
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
	BMCConnectTimeout       time.Duration
	PowerCheckInterval      time.Duration
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

func WithBmcConnectTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.BMCConnectTimeout = timeout
	}
}

func WithPowerCheckInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.PowerCheckInterval = interval
	}
}

func WithLeaderElectionNamespace(namespace string) Option {
	return func(c *Config) {
		c.LeaderElectionNamespace = namespace
	}
}

func NewConfig(opts ...Option) *Config {
	defaults := &Config{
		EnableLeaderElection: true,
	}

	for _, opt := range opts {
		opt(defaults)
	}

	return defaults
}

func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	options := controllerruntime.Options{
		Logger:                  log,
		LeaderElection:          c.EnableLeaderElection,
		LeaderElectionID:        "e74dec1a.bmc.tinkerbell.org",
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

	mgr, err := controller.NewManager(c.Client, options, c.PowerCheckInterval)
	if err != nil {
		return err
	}

	return mgr.Start(ctx)
}
