package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"

	tink "github.com/tinkerbell/tink/protos/hardware"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	cacherClient "github.com/packethost/cacher/client"
	"github.com/packethost/cacher/protos/cacher"
	"github.com/packethost/hegel/grpc/hegel"
	"github.com/packethost/hegel/metrics"
	"github.com/packethost/hegel/xff"
	"github.com/packethost/pkg/env"
	"github.com/packethost/pkg/log"
	"github.com/pkg/errors"
	tinkClient "github.com/tinkerbell/tink/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type server struct {
	log            log.Logger
	hardwareClient hardwareGetter

	subLock       sync.RWMutex
	subscriptions map[string]*subscription
}

type hardwareGetter interface {
	ByIP(ctx context.Context, in getRequest, opts ...grpc.CallOption) (hardware, error)
	Watch(ctx context.Context, in getRequest, opts ...grpc.CallOption) (watchClient, error)
}

type getRequest interface{}
type hardware interface{}

type subscription struct {
	ID           string        `json:"id"`
	IP           string        `json:"ip"`
	InitDuration time.Duration `json:"init_duration"`
	StartedAt    time.Time     `json:"started_at"`
	cancel       func()
	updateChan   chan []byte
}

type hardwareGetterCacher struct {
	client cacher.CacherClient
}

type hardwareGetterTinkerbell struct {
	client tink.HardwareServiceClient
}

func (hg hardwareGetterCacher) ByIP(ctx context.Context, in getRequest, opts ...grpc.CallOption) (hardware, error) {
	hw, err := hg.client.ByIP(ctx, in.(*cacher.GetRequest), opts...)
	if err != nil {
		return nil, err
	}
	return hw, nil
}

func (hg hardwareGetterCacher) Watch(ctx context.Context, in getRequest, opts ...grpc.CallOption) (watchClient, error) {
	w, err := hg.client.Watch(ctx, in.(*cacher.GetRequest), opts...)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (hg hardwareGetterTinkerbell) ByIP(ctx context.Context, in getRequest, opts ...grpc.CallOption) (hardware, error) {
	hw, err := hg.client.ByIP(ctx, in.(*tink.GetRequest), opts...)
	if err != nil {
		return nil, err
	}
	return hw, nil
}

func (hg hardwareGetterTinkerbell) Watch(ctx context.Context, in getRequest, opts ...grpc.CallOption) (watchClient, error) {
	w, err := hg.client.Watch(ctx, in.(*tink.GetRequest), opts...)
	if err != nil {
		return nil, err
	}
	return w, nil
}

const defaultCustomEndpoints = `{"/metadata":".metadata"}`

var (
	facility = flag.String("facility", env.Get("HEGEL_FACILITY", "onprem"),
		"The facility we are running in (mostly to connect to cacher)")
	tlsCertPath = flag.String("tls_cert", env.Get("HEGEL_TLS_CERT"),
		"Path of tls certificat to use.")
	tlsKeyPath = flag.String("tls_key", env.Get("HEGEL_TLS_KEY"),
		"Path of tls key to use.")
	useTLS = flag.Bool("use_tls", env.Bool("HEGEL_USE_TLS", true),
		"Whether we should use tls or not (should be disabled for traefik)")
	metricsPort = flag.Int("http_port", env.Int("HEGEL_HTTP_PORT", 50061),
		"Port to liten on http")
	gitRev              string = "undefind"
	gitRevJSON          []byte
	StartTime           = time.Now()
	logger              log.Logger
	hegelServer         *server
	isCacherAvailableMu sync.RWMutex
	isCacherAvailable   bool
)

func main() {
	flag.Parse()
	// setup structured logging
	l, err := log.Init("github.com/packethost/hegel")
	logger = l.Package("main")
	if err != nil {
		panic(err)
	}
	defer l.Close()
	metrics.Init(l)

	metrics.State.Set(metrics.Initializing)

	port := env.Int("GRPC_PORT", 42115)

	serverOpts := make([]grpc.ServerOption, 0)

	// setup tls credentials
	if *useTLS {
		creds, err := credentials.NewServerTLSFromFile(*tlsCertPath, *tlsKeyPath)
		if err != nil {
			logger.Fatal(err, "failed to initialize server credentials")
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}

	trustedProxies := xff.ParseTrustedProxies()
	xffStream, xffUnary := xff.GRPCMiddlewares(logger, trustedProxies)
	streamLogger, unaryLogger := logger.GRPCLoggers()
	serverOpts = append(serverOpts,
		grpc_middleware.WithUnaryServerChain(
			xffUnary,
			unaryLogger,
			grpc_prometheus.UnaryServerInterceptor,
		),
		grpc_middleware.WithStreamServerChain(
			xffStream,
			streamLogger,
			grpc_prometheus.StreamServerInterceptor,
		),
	)

	var hg hardwareGetter
	dataModelVersion := env.Get("DATA_MODEL_VERSION")
	switch dataModelVersion {
	case "1":
		tc, err := tinkClient.TinkHardwareClient()
		if err != nil {
			logger.Fatal(err, "Failed to create the tink client")
		}
		hg = hardwareGetterTinkerbell{tc}
		// add health check for tink?
	default:
		cc, err := cacherClient.New(*facility)
		if err != nil {
			logger.Fatal(err, "Failed to create the cacher client")
		}
		hg = hardwareGetterCacher{cc}
		go func() {
			c := time.Tick(15 * time.Second)
			for range c {
				// Get All hardware as a proxy for a healthcheck
				// TODO (patrickdevivo) until Cacher gets a proper healthcheck RPC
				// a la https://github.com/grpc/grpc/blob/master/doc/health-checking.md
				// this will have to do.
				// Note that we don't do anything with the stream (we don't read from it)
				var isCacherAvailableTemp bool
				ctx, cancel := context.WithCancel(context.Background())
				_, err := cc.All(ctx, &cacher.Empty{})
				if err == nil {
					isCacherAvailableTemp = true
				}
				cancel()

				isCacherAvailableMu.Lock()
				isCacherAvailable = isCacherAvailableTemp
				isCacherAvailableMu.Unlock()

				if isCacherAvailableTemp {
					metrics.CacherConnected.Set(1)
					metrics.CacherHealthcheck.WithLabelValues("true").Inc()
					logger.With("status", isCacherAvailableTemp).Debug("tick")
				} else {
					metrics.CacherConnected.Set(0)
					metrics.CacherHealthcheck.WithLabelValues("false").Inc()
					metrics.Errors.WithLabelValues("cacher", "healthcheck").Inc()
					logger.With("status", isCacherAvailableTemp).Error(err)
				}
			}

		}()
	}

	grpcServer := grpc.NewServer(serverOpts...)
	hegelServer = &server{
		log:            logger,
		hardwareClient: hg,
		subscriptions:  make(map[string]*subscription),
	}

	hegel.RegisterHegelServer(grpcServer, hegelServer)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		err = errors.Wrap(err, "failed to listen")
		logger.Fatal(err)
	}

	// Register grpc prometheus server
	grpc_prometheus.Register(grpcServer)

	endpoints, err := parseCustomEndpoints(env.Get("CUSTOM_ENDPOINTS"))
	if err != nil {
		logger.Fatal(err)
	}
	ServeHTTP(endpoints)

	metrics.State.Set(metrics.Ready)
	//Serving GRPC
	logger.Info("serving grpc")
	err = grpcServer.Serve(lis)
	if err != nil {
		logger.Fatal(err, "Failed to serve  grpc")
	}
}

func parseCustomEndpoints(customEndpoints string) (map[string]string, error) {
	if customEndpoints == "" {
		customEndpoints = defaultCustomEndpoints
	}

	endpoints := make(map[string]string)
	err := json.Unmarshal([]byte(customEndpoints), &endpoints)
	if err != nil {
		err = errors.Wrap(err, "error in parsing custom endpoints")
		return nil, err
	}
	return endpoints, nil
}

func registerCustomEndpoints(mux *http.ServeMux, endpoints map[string]string) error {
	if mux == nil {
		mux = http.DefaultServeMux
	}

	for endpoint, filter := range endpoints {
		mux.HandleFunc(endpoint, getMetadata(filter))
	}

	return nil
}
