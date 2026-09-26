package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/go-logr/logr"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinkerbell/tinkerbell/pkg/listener"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	grpcinternal "github.com/tinkerbell/tinkerbell/tink/server/internal/grpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

// Registry is the Prometheus registry for all Tink server gRPC metrics.
// It is separate from the default registry so that gRPC metrics can be
// served on a dedicated /tink-server/metrics endpoint.
var Registry = prometheus.NewRegistry()

// grpcServerMetrics is an isolated set of gRPC server metrics registered
// on [Registry] instead of the global default.
var grpcServerMetrics = grpcprometheus.NewServerMetrics()

func init() {
	Registry.MustRegister(grpcServerMetrics)
}

type Config struct {
	Backend      grpcinternal.Backend
	BindAddrPort netip.AddrPort
	// BindAddrPortV6 is the IPv6 address and port to serve on. Unset leaves IPv6 unserved.
	BindAddrPortV6 netip.AddrPort
	Logger         logr.Logger
	Auto           AutoCapabilities
	TLS            TLS
}

type AutoCapabilities struct {
	Enrollment Enrollment
	Discovery  Discovery
}

type Enrollment struct {
	Enabled               bool
	WorkflowRuleSetLister grpcinternal.WorkflowRuleSetLister
	WorkflowCreator       grpcinternal.WorkflowCreator
}

type Discovery struct {
	Enabled           bool
	Namespace         string
	EnrollmentEnabled bool
	HardwareCreator   grpcinternal.HardwareCreator
	HardwareFilterer  grpcinternal.HardwareFilterer
}

type TLS struct {
	Cert credentials.TransportCredentials
}

// Option is a functional option type.
type Option func(*Config)

// WithAutoDiscoveryNamespace sets the namespace for auto discovery.
func WithAutoDiscoveryNamespace(ns string) Option {
	return func(c *Config) {
		c.Auto.Discovery.Namespace = ns
	}
}

// WithAutoDiscoveryAutoEnrollmentEnabled sets the value for hardware.spec.auto.enrollmentEnabled when auto discovery creates Hardware objects.
func WithAutoDiscoveryAutoEnrollmentEnabled(enabled bool) Option {
	return func(c *Config) {
		c.Auto.Discovery.EnrollmentEnabled = enabled
	}
}

// WithBackend sets the backend for the server.
func WithBackend(b grpcinternal.Backend) Option {
	return func(c *Config) {
		c.Backend = b
	}
}

// WithBindAddrPort sets the bind address and port for the server.
func WithBindAddrPort(addrPort netip.AddrPort) Option {
	return func(c *Config) {
		c.BindAddrPort = addrPort
	}
}

// WithLogger sets the logger for the server.
func WithLogger(l logr.Logger) Option {
	return func(c *Config) {
		c.Logger = l
	}
}

// WithTLSCert sets the TLS key file for the server.
func WithTLSCert(cert credentials.TransportCredentials) Option {
	return func(c *Config) {
		c.TLS.Cert = cert
	}
}

func NewConfig(opts ...Option) *Config {
	c := &Config{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	s := &grpcinternal.Handler{
		Backend: c.Backend,
		Logger:  log,
		NowFunc: time.Now,
		AutoCapabilities: grpcinternal.AutoCapabilities{
			Enrollment: grpcinternal.AutoEnrollment{
				Enabled:               c.Auto.Enrollment.Enabled,
				WorkflowRuleSetLister: c.Auto.Enrollment.WorkflowRuleSetLister,
				WorkflowCreator:       c.Auto.Enrollment.WorkflowCreator,
			},
			Discovery: grpcinternal.AutoDiscovery{
				Enabled:           c.Auto.Discovery.Enabled,
				Namespace:         c.Auto.Discovery.Namespace,
				EnrollmentEnabled: c.Auto.Discovery.EnrollmentEnabled,
				HardwareCreator:   c.Auto.Discovery.HardwareCreator,
				HardwareFilterer:  c.Auto.Discovery.HardwareFilterer,
			},
		},
	}

	params := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(grpcServerMetrics.UnaryServerInterceptor()),
		grpc.StreamInterceptor(grpcServerMetrics.StreamServerInterceptor()),
	}
	if c.TLS.Cert != nil {
		params = append(params, grpc.Creds(c.TLS.Cert))
	}

	// register servers
	gs := grpc.NewServer(params...)
	proto.RegisterWorkflowServiceServer(gs, s)
	reflection.Register(gs)
	grpcServerMetrics.InitializeMetrics(gs)

	var listeners []net.Listener
	for _, addrPort := range []netip.AddrPort{c.BindAddrPort, c.BindAddrPortV6} {
		if !addrPort.Addr().IsValid() {
			continue
		}
		lis, err := listener.TCP(ctx, addrPort.Addr(), int(addrPort.Port()))
		if err != nil {
			for _, l := range listeners {
				_ = l.Close()
			}
			return fmt.Errorf("failed to listen: %w", err)
		}
		listeners = append(listeners, lis)
	}
	if len(listeners) == 0 {
		return errors.New("tink server has no IPv4 or IPv6 bind address")
	}

	go func() {
		<-ctx.Done()
		time.Sleep(1 * time.Second)
		log.Info("Initiating graceful shutdown")
		timer := time.AfterFunc(5*time.Second, func() {
			log.Info("Server couldn't stop gracefully in time, doing force stop")
			gs.Stop()
		})
		defer timer.Stop()
		gs.GracefulStop() // gracefully stop server after in-flight server streaming rpc finishes
		log.Info("Server stopped")
	}()

	// A graceful shutdown closes every listener, so each Serve returns nil and
	// g.Wait reports success. A Serve that fails on its own stops the shared
	// server, otherwise the other family keeps serving and g.Wait never returns.
	g, _ := errgroup.WithContext(ctx)
	for _, lis := range listeners {
		log.Info("starting gRPC server", "bindAddr", lis.Addr().String())
		g.Go(func() error {
			if err := gs.Serve(lis); err != nil {
				gs.Stop()
				return err
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		log.Error(err, "failed to serve")
		return err
	}

	return nil
}

type allInterfaces interface {
	grpcinternal.Backend
	grpcinternal.HardwareCreator
	grpcinternal.HardwareFilterer
	grpcinternal.WorkflowRuleSetLister
	grpcinternal.WorkflowCreator
}

// SetBackends is a helper function to set a single backend implementation for all backend interfaces.
// This is useful for backends that implement multiple interfaces, such as the kube backend.
func (c *Config) SetBackends(b allInterfaces) {
	c.Backend = b
	c.Auto.Discovery.HardwareCreator = b
	c.Auto.Discovery.HardwareFilterer = b
	c.Auto.Enrollment.WorkflowRuleSetLister = b
	c.Auto.Enrollment.WorkflowCreator = b
}
