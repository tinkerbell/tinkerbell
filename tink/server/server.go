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
	"google.golang.org/grpc/reflection"
)

type Config struct {
	Backend      grpcinternal.BackendReadUpdater
	AutoBackend  grpcinternal.AutoReadCreator
	BindAddrPort netip.AddrPort
	Logger       logr.Logger
}

// Option is a functional option type.
type Option func(*Config)

// WithBackend sets the backend for the server.
func WithBackend(b grpcinternal.BackendReadUpdater) Option {
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
				Enabled:     true,
				ReadCreator: c.AutoBackend,
			},
		},
	}

	params := []grpc.ServerOption{
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(grpcprometheus.UnaryServerInterceptor),
		grpc.StreamInterceptor(grpcprometheus.StreamServerInterceptor),
	}

	// register servers
	gs := grpc.NewServer(params...)
	proto.RegisterWorkflowServiceServer(gs, s)
	reflection.Register(gs)
	grpcprometheus.Register(gs)

	lis, err := net.Listen("tcp", c.BindAddrPort.String())
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	go func() {
		<-ctx.Done()
		time.Sleep(1 * time.Second)
		log.Info("Initiating graceful shutdown")
		timer := time.AfterFunc(10*time.Second, func() {
			log.Info("Server couldn't stop gracefully in time. Doing force stop.")
			gs.Stop()
		})
		defer timer.Stop()
		gs.GracefulStop() // gracefully stop server after in-flight server streaming rpc finishes
		log.Info("Server stopped gracefully.")
	}()

	log.Info("starting gRPC server", "bindAddr", c.BindAddrPort.String())
	if err := gs.Serve(lis); err != nil {
		log.Error(err, "failed to serve")
		return err
	}

	return nil
}
