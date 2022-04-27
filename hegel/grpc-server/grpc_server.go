package grpcserver

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/packethost/pkg/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tinkerbell/hegel/grpc/protos/hegel"
	"github.com/tinkerbell/hegel/hardware"
	"github.com/tinkerbell/hegel/metrics"
	"github.com/tinkerbell/hegel/xff"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

//go:generate protoc -I grpc/protos grpc/protos/hegel.proto --go_out=plugins=grpc:grpc/hegel

type Server struct {
	log            log.Logger
	hardwareClient hardware.Client

	subLock       *sync.RWMutex
	subscriptions map[string]*Subscription
}

type Subscription struct {
	ID           string        `json:"id"`
	IP           string        `json:"ip"`
	InitDuration time.Duration `json:"init_duration"`
	StartedAt    time.Time     `json:"started_at"`
	cancel       func()
	updateChan   chan []byte
}

func NewServer(l log.Logger, hc hardware.Client) *Server {
	return &Server{
		log:            l,
		hardwareClient: hc,
		subLock:        &sync.RWMutex{},
		subscriptions:  make(map[string]*Subscription),
	}
}

func Serve(_ context.Context, l log.Logger, srv *Server, port int, unparsedProxies, tlsCertPath, tlsKeyPath string, useTLS bool) error {
	serverOpts := make([]grpc.ServerOption, 0)

	if useTLS {
		creds, err := credentials.NewServerTLSFromFile(tlsCertPath, tlsKeyPath)
		if err != nil {
			l.Error(err, "failed to initialize server credentials")
			panic(err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}

	proxies := xff.ParseTrustedProxies(unparsedProxies)
	xffStream, xffUnary := xff.GRPCMiddlewares(l, proxies)
	streamLogger, unaryLogger := l.GRPCLoggers()
	serverOpts = append(serverOpts,
		grpc_middleware.WithUnaryServerChain(
			xffUnary,
			unaryLogger,
			grpc_prometheus.UnaryServerInterceptor,
			otelgrpc.UnaryServerInterceptor(),
		),
		grpc_middleware.WithStreamServerChain(
			xffStream,
			streamLogger,
			grpc_prometheus.StreamServerInterceptor,
			otelgrpc.StreamServerInterceptor(),
		),
	)

	grpcServer := grpc.NewServer(serverOpts...)

	grpc_prometheus.Register(grpcServer)

	hegel.RegisterHegelServer(grpcServer, srv)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		err = errors.Wrap(err, "failed to listen")
		l.Error(err)
		panic(err)
	}

	metrics.State.Set(metrics.Ready)
	l.Info("serving grpc")
	err = grpcServer.Serve(lis)
	if err != nil {
		l.Fatal(err, "failed to serve grpc")
	}

	return nil
}

func (s *Server) Log() log.Logger {
	return s.log
}

func (s *Server) HardwareClient() hardware.Client {
	return s.hardwareClient
}

func (s *Server) SubLock() *sync.RWMutex {
	return s.subLock
}

func (s *Server) Subscriptions() map[string]*Subscription {
	return s.subscriptions
}

func (s *Server) SetHardwareClient(hc hardware.Client) {
	s.hardwareClient = hc
}

// Try to parse out the peer IP.
func peerIP(a net.Addr) string {
	if tcp, ok := a.(*net.TCPAddr); ok {
		return tcp.IP.String()
	}
	// we see "bufconn" here under TestSubscribe
	return a.String()
}

func (s *Server) Get(ctx context.Context, _ *hegel.GetRequest) (*hegel.GetResponse, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, errors.New("could not get peer info from client")
	}
	s.log.With("client", p.Addr, "op", "get").Info()

	ip := peerIP(p.Addr)

	hw, err := s.hardwareClient.ByIP(ctx, ip)
	if err != nil {
		return nil, err
	}
	ehw, err := hw.Export()
	if err != nil {
		return nil, err
	}
	return &hegel.GetResponse{
		JSON: string(ehw),
	}, nil
}

func (s *Server) Subscribe(_ *hegel.SubscribeRequest, stream hegel.Hegel_SubscribeServer) error {
	startedAt := time.Now().UTC()
	metrics.TotalSubscriptions.Inc()
	metrics.Subscriptions.WithLabelValues("initializing").Inc()
	timer := prometheus.NewTimer(metrics.InitDuration)

	logger := s.log.With("op", "subscribe")

	initError := func(err error) error {
		logger.Error(err)
		metrics.Subscriptions.WithLabelValues("initializing").Dec()
		metrics.Errors.WithLabelValues("subscribe", "initializing").Inc()
		timer.ObserveDuration()
		return err
	}

	p, ok := peer.FromContext(stream.Context())
	if !ok {
		return initError(errors.New("could not get peer info from client"))
	}

	ip := peerIP(p.Addr)

	logger = logger.With("ip", ip, "client", p.Addr)

	logger.Info()

	hw, err := s.hardwareClient.ByIP(stream.Context(), ip)
	if err != nil {
		return initError(err)
	}

	id, err := hw.ID()
	if err != nil {
		return initError(err)
	}

	ctx, cancel := context.WithCancel(stream.Context())
	watch, err := s.hardwareClient.Watch(ctx, id)
	if err != nil {
		cancel()
		return initError(err)
	}

	sub := &Subscription{
		ID:           id,
		IP:           ip,
		StartedAt:    startedAt,
		InitDuration: time.Since(startedAt),
		cancel:       cancel,
		updateChan:   make(chan []byte, 1),
	}

	s.subLock.Lock()
	// NOTE: Access to s.subscriptions must be done within this lock to avoid race conditions
	old := s.subscriptions[id] // nolint:ifshort // variable 'old' is only used in the if-statement in :237
	s.subscriptions[id] = sub
	s.subLock.Unlock()

	// Disconnect previous client if a client is already connected for this hardware id
	if old != nil {
		old.cancel()
	}

	defer func() {
		s.subLock.Lock()
		defer s.subLock.Unlock()
		// Check if subscription for hardware id exists.
		// If the subscriptions exists, make sure it has not been replaced by a new connection.
		if cSub := s.subscriptions[id]; cSub == sub {
			delete(s.subscriptions, id)
		}
	}()

	timer.ObserveDuration()
	metrics.Subscriptions.WithLabelValues("initializing").Dec()
	metrics.Subscriptions.WithLabelValues("active").Inc()

	activeError := func(err error) error {
		if err == nil {
			return nil
		}

		logger.Error(err)
		metrics.Subscriptions.WithLabelValues("active").Dec()
		metrics.Errors.WithLabelValues("subscribe", "active").Inc()
		return err
	}

	errs := make(chan error, 1)
	go func() {
		for {
			hw, err := watch.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = status.Error(codes.OK, "stream ended")
				}
				errs <- err
				close(sub.updateChan)
				return
			}

			ehw, err := hw.Export()
			if err != nil {
				errs <- err
				close(sub.updateChan)
				return
			}

			sub.updateChan <- ehw
		}
	}()
	go func() {
		l := logger.With("op", "send")
		for ehw := range sub.updateChan {
			l.Info()
			err := stream.Send(&hegel.SubscribeResponse{
				JSON: string(ehw),
			})
			if err != nil {
				errs <- err
				cancel()
				return
			}
		}
	}()

	var retErr error
	if err := <-errs; status.Code(err) != codes.OK && retErr == nil {
		retErr = err
	}
	return activeError(retErr)
}
