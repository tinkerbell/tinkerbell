package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/packethost/cacher/client"
	"github.com/packethost/cacher/protos/cacher"
	"github.com/packethost/hegel/grpc/hegel"
	"github.com/packethost/hegel/gxff"
	"github.com/packethost/pkg/env"
	"github.com/packethost/pkg/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type server struct {
	log          log.Logger
	cacherClient cacher.CacherClient
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
	gitRev      string = "undefind"
	gitRevJSON  []byte
	StartTime   = time.Now()
	logger      log.Logger
	hegelServer *server
)

func envString(key string, def string) (val string) {
	val, ok := os.LookupEnv(key)
	if !ok {
		val = def
	}
	return
}

func envBool(key string, def bool) (val bool) {
	val_, ok := os.LookupEnv(key)
	if !ok {
		val = def
	}
	val, _ = strconv.ParseBool(val_)
	return
}

func envInt(key string, def int) (val int) {
	val_, ok := os.LookupEnv(key)
	if !ok {
		val = def
		return
	}
	i64, _ := strconv.ParseInt(val_, 10, 64)
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

	cacherClient, err := client.New(*facility)
	if err != nil {
		logger.Fatal(err, "Failed to create the cacher client")
	}

	grpcServer := grpc.NewServer(serverOpts...)
	hegelServer = &server{
		log:          logger,
		cacherClient: cacherClient,
	}

	hegel.RegisterHegelServer(grpcServer, hegelServer)
	grpc_prometheus.Register(grpcServer)

	logger.Info("serving grpc")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		err = errors.Wrap(err, "failed to listen")
		logger.Error(err)
		panic(err)
	}

	err = grpcServer.Serve(lis)
	if err != nil {
		logger.Fatal(err, "Failed to serve  grpc")
	}

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/_packet/healthcheck", healthCheckHandler)
	http.HandleFunc("/_packet/version", versionHandler)
	http.HandleFunc("/metadata", getMetadata)

	logger.With("port", *metricsPort).Info("Starting http server")
	go http.ListenAndServe(fmt.Sprintf(":%d", *metricsPort), nil)
}
