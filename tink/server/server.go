package server

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/go-logr/logr"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	grpcinternal "github.com/tinkerbell/tinkerbell/tink/server/internal/grpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

type Config struct {
	Backend grpcinternal.BackendReadWriter

	BindAddrPort netip.AddrPort
	Logger       logr.Logger
	Auto         AutoCapabilities
	TLS          TLS
}

type AutoCapabilities struct {
	Enrollment Enrollment
	Discovery  Discovery
}

type Enrollment struct {
	Enabled bool
	Backend grpcinternal.AutoEnrollmentReadCreator
}

type Discovery struct {
	Enabled           bool
	Namespace         string
	EnrollmentEnabled bool
	Backend           grpcinternal.AutoDiscoveryReadCreator
}

type TLS struct {
	CertFile string
	KeyFile  string
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
func WithBackend(b grpcinternal.BackendReadWriter) Option {
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

// WithTLSCertFile sets the TLS certificate file for the server.
func WithTLSCertFile(certFile string) Option {
	return func(c *Config) {
		c.TLS.CertFile = certFile
	}
}

// WithTLSKeyFile sets the TLS key file for the server.
func WithTLSKeyFile(keyFile string) Option {
	return func(c *Config) {
		c.TLS.KeyFile = keyFile
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
		BackendReadWriter: c.Backend,
		Logger:            log,
		NowFunc:           time.Now,
		AutoCapabilities: grpcinternal.AutoCapabilities{
			Enrollment: grpcinternal.AutoEnrollment{
				Enabled:     c.Auto.Enrollment.Enabled,
				ReadCreator: c.Auto.Enrollment.Backend,
			},
			Discovery: grpcinternal.AutoDiscovery{
				Enabled:                  c.Auto.Discovery.Enabled,
				Namespace:                c.Auto.Discovery.Namespace,
				EnrollmentEnabled:        c.Auto.Discovery.EnrollmentEnabled,
				AutoDiscoveryReadCreator: c.Auto.Discovery.Backend,
			},
		},
	}

	params := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(grpcprometheus.UnaryServerInterceptor),
		grpc.StreamInterceptor(grpcprometheus.StreamServerInterceptor),
	}
	if c.TLS.CertFile != "" && c.TLS.KeyFile != "" {
		creds, err := loadTLSCredentials(c.TLS.CertFile, c.TLS.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS credentials: %w", err)
		}
		params = append(params, grpc.Creds(creds))
	}

	// register servers
	gs := grpc.NewServer(params...)
	proto.RegisterWorkflowServiceServer(gs, s)
	reflection.Register(gs)
	grpcprometheus.Register(gs)

	n := net.ListenConfig{}
	lis, err := n.Listen(ctx, "tcp", c.BindAddrPort.String())
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
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

	log.Info("starting gRPC server", "bindAddr", c.BindAddrPort.String())
	if err := gs.Serve(lis); err != nil {
		log.Error(err, "failed to serve")
		return err
	}

	return nil
}

func loadTLSCredentials(certFile, keyFile string) (credentials.TransportCredentials, error) {
	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return creds, nil
}
