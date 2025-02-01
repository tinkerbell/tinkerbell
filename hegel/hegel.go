package hegel

import (
	"context"
	"fmt"

	"dario.cat/mergo"
	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinkerbell/tinkerbell/hegel/internal/frontend/ec2"
	"github.com/tinkerbell/tinkerbell/hegel/internal/frontend/hack"
	"github.com/tinkerbell/tinkerbell/hegel/internal/http"
	"github.com/tinkerbell/tinkerbell/hegel/internal/metrics"
	"github.com/tinkerbell/tinkerbell/hegel/internal/middleware"
	"github.com/tinkerbell/tinkerbell/hegel/internal/xff"
)

type Config struct {
	BackendEc2     ec2.Client
	BackendHack    hack.Client
	TrustedProxies string
	DebugMode      bool
	BindAddrPort   string
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
	fe := ec2.New(c.BackendEc2)
	fe.Configure(router)

	hack.Configure(router, c.BackendHack)

	return http.Serve(ctx, log, c.BindAddrPort, router)
}
