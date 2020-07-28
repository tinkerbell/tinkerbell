package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	tink "github.com/tinkerbell/tink/protos/hardware"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	cacherClient "github.com/packethost/cacher/client"
	"github.com/packethost/cacher/protos/cacher"
	"github.com/packethost/hegel/grpc/hegel"
	"github.com/packethost/hegel/gxff"
	"github.com/packethost/hegel/metrics"
	"github.com/packethost/pkg/env"
	"github.com/packethost/pkg/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	tinkClient "github.com/tinkerbell/tink/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type server struct {
	log            log.Logger
	hardwareClient hardwareGetter
}

type hardwareGetter interface {
	ByIP(ctx context.Context, in getRequest, opts ...grpc.CallOption) (hardware, error)
	Watch(ctx context.Context, in getRequest, opts ...grpc.CallOption) (watchClient, error)
}

type getRequest interface{}
type hardware interface{}

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

var (
	facility = flag.String("facility", envString("HEGEL_FACILITY", "onprem"),
		"The facility we are running in (mostly to connect to cacher)")
	tlsCertPath = flag.String("tls_cert", envString("HEGEL_TLS_CERT", ""),
		"Path of tls certificat to use.")
	tlsKeyPath = flag.String("tls_key", envString("HEGEL_TLS_KEY", ""),
		"Path of tls key to use.")
	useTLS = flag.Bool("use_tls", envBool("HEGEL_USE_TLS", true),
		"Whether we should use tls or not (should be disabled for traefik)")
	metricsPort = flag.Int("http_port", envInt("HEGEL_HTTP_PORT", 50061),
		"Port to liten on http")
	customEndpoints     string
	gitRev              string = "undefind"
	gitRevJSON          []byte
	StartTime           = time.Now()
	logger              log.Logger
	hegelServer         *server
	isCacherAvailableMu sync.RWMutex
	isCacherAvailable   bool
)

func envString(key string, def string) (val string) {
	val, ok := os.LookupEnv(key)
	if !ok {
		val = def
	}
	return
}

func envBool(key string, def bool) (val bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		val = def
	} else {
		val, _ = strconv.ParseBool(v)
	}
	return
}

func envInt(key string, def int) (val int) {
	v, ok := os.LookupEnv(key)
	if !ok {
		val = def
		return
	}
	i64, _ := strconv.ParseInt(v, 10, 64)
	val = int(i64)
	return
}

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

	port, err := strconv.Atoi(env.Get("GRPC_PORT", "42115"))
	if err != nil {
		logger.Fatal(err, "parse grpc port from env")
	}
	if port < 1 {
		logger.Fatal(err, "parse grpc port from env")
	}

	serverOpts := make([]grpc.ServerOption, 0)

	// setup tls credentials
	if *useTLS {
		creds, err := credentials.NewServerTLSFromFile(*tlsCertPath, *tlsKeyPath)
		if err != nil {
			logger.Error(err, "failed to initialize server credentials")
			panic(err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}

	xffStream, xffUnary := gxff.New(logger, nil)
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
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
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
	}

	hegel.RegisterHegelServer(grpcServer, hegelServer)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		err = errors.Wrap(err, "failed to listen")
		logger.Error(err)
		panic(err)
	}

	// Register grpc prometheus server
	grpc_prometheus.Register(grpcServer)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/_packet/healthcheck", healthCheckHandler)
	http.HandleFunc("/_packet/version", versionHandler)
	err = registerCustomEndpoints()
	if err != nil {
		logger.Fatal(err, "could not register custom endpoints")
	}

	logger.With("port", *metricsPort).Info("Starting http server")
	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", *metricsPort), nil)
		if err != nil {
			logger.Error(err, "failed to serve http")
			panic(err)
		}
	}()

	metrics.State.Set(metrics.Ready)
	//Serving GRPC
	logger.Info("serving grpc")
	err = grpcServer.Serve(lis)
	if err != nil {
		logger.Fatal(err, "Failed to serve  grpc")
	}
}

func registerCustomEndpoints() error {
	customEndpoints = envString("CUSTOM_ENDPOINTS", `{"/metadata":".metadata"}`)

	endpoints := make(map[string]string)
	err := json.Unmarshal([]byte(customEndpoints), &endpoints)
	if err != nil {
		logger.Info("Error in parsing custom endpoints: ", err)
	}
	for endpoint, filter := range endpoints {
		http.HandleFunc(endpoint, getMetadata(filter))
	}

	return nil
}
