// package tootles is the code for serving metadata (ec2 style, etc).
// Useful for Cloud-init integration.
package tootles

import (
	"context"
	"fmt"

	"dario.cat/mergo"
	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/tootles/internal/backend"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/ec2"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/hack"
	"github.com/tinkerbell/tinkerbell/tootles/internal/http"
	"github.com/tinkerbell/tinkerbell/tootles/internal/metrics"
	"github.com/tinkerbell/tinkerbell/tootles/internal/middleware"
	"github.com/tinkerbell/tinkerbell/tootles/internal/xff"
)

type Config struct {
	BackendEc2       ec2.Client
	BackendHack      hack.Client
	TrustedProxies   string
	DebugMode        bool
	BindAddrPort     string
	InstanceEndpoint bool
}

// HardwareReader is the interface required to read Hardware objects.
// It is implemented by the kube and noop backends.
type HardwareReader interface {
	ReadHardware(ctx context.Context, id, namespace string, opts data.ReadListOptions) (*v1alpha1.Hardware, error)
}

// SetBackendFromReader configures BackendEc2 and BackendHack from a HardwareReader.
// This allows callers to wire a backend without importing tootles internal packages.
func (c *Config) SetBackendFromReader(reader HardwareReader) {
	b := backend.New(reader)
	c.BackendEc2 = b
	c.BackendHack = b
}

func NewConfig(c Config, addrPort string) *Config {
	defaults := &Config{
		DebugMode:    false,
		BindAddrPort: addrPort,
	}

	if err := mergo.Merge(defaults, &c); err != nil {
		panic(fmt.Sprintf("failed to merge config: %v", err))
	}

	return defaults
}

func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	xffmw, err := xff.MiddlewareFromUnparsed(c.TrustedProxies)
	if err != nil {
		return err
	}

	registry := prometheus.NewRegistry()

	if !c.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(
		metrics.InstrumentRequestCount(registry),
		metrics.InstrumentRequestDuration(registry),
		gin.Recovery(),
		middleware.Logging(log),
		xffmw,
	)

	metrics.Configure(router, registry)
	// healthcheck.Configure(router, be)

	// TODO(chrisdoherty4) Handle multiple frontends.
	fe := ec2.New(c.BackendEc2, c.InstanceEndpoint)
	fe.Configure(router)

	hack.Configure(router, c.BackendHack)

	return http.Serve(ctx, log, c.BindAddrPort, router)
}
