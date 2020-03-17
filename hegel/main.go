package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/packethost/cacher/client"
	"github.com/packethost/cacher/protos/cacher"
	"github.com/packethost/hegel/grpc/hegel"
	"github.com/packethost/hegel/gxff"
	packetgrpc "github.com/packethost/pkg/grpc"
	"github.com/packethost/pkg/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type server struct {
	log          log.Logger
	cacherClient cacher.CacherClient
}

var (
	facility    = flag.String("facility", envString("HEGEL_FACILITY", "lab1"), "The facility we are running in (mostly to connect to cacher)")
	tlsCertPath = flag.String("tls_cert", envString("HEGEL_TLS_CERT", ""), "Path of tls certificat to use.")
	tlsKeyPath  = flag.String("tls_key", envString("HEGEL_TLS_KEY", ""), "Path of tls key to use.")
	useTLS      = flag.Bool("use_tls", envBool("HEGEL_USE_TLS", true), "Whether we should use tls or not (should be disabled for traefik)")
	metricsPort = flag.Int("http_port", envInt("HEGEL_HTTP_PORT", 50061), "Port to liten on http")

	gitRev     string = "undefind"
	gitRevJSON []byte
	startTime  = time.Now()
	logger     log.Logger

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
	l, cleanup, err := log.Init("github.com/packethost/hegel")
	logger = l.Package("main")
	if err != nil {
		panic(err)
	}
	defer cleanup()

	grpcConfig, err := packetgrpc.ConfigFromEnv()
	if err != nil {
		logger.Error(err, "Failed to get config")
		panic(err)
	}

	grpcAddr, err := grpcConfig.BindAddress()
	if err != nil {
		logger.Error(err, "failed to get bind address")
		panic(err)
	}
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logger.Error(err, "failed to listen for gRPC addr", grpcAddr)
		panic(err)
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

	cacherClient := client.New(*facility)
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
			_, err := cacherClient.All(ctx, &cacher.Empty{})
			if err == nil {
				isCacherAvailableTemp = true
			}
			cancel()

			isCacherAvailableMu.Lock()
			isCacherAvailable = isCacherAvailableTemp
			isCacherAvailableMu.Unlock()

			logger.With("status", isCacherAvailableTemp).Debug("tick")
		}

	}()

	grpcServer := grpc.NewServer(serverOpts...)
	hegel.RegisterHegelServer(grpcServer, &server{
		log:          logger,
		cacherClient: cacherClient,
	})

	// Register grpc prometheus server
	grpc_prometheus.Register(grpcServer)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/_packet/healthcheck", healthCheckHandler)
	http.HandleFunc("/_packet/version", versionHandler)
	logger.With("port", *metricsPort).Info("Starting http server")
	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", *metricsPort), nil)
		if err != nil {
			logger.Error(err, "failed to serve http")
			panic(err)
		}
	}()

	logger.With("listen_address", grpcAddr).Info("starting gRPC server")
	err = grpcServer.Serve(lis)
	if err != nil {
		logger.Error(err, "failed to serve grpc")
		panic(err)
	}
}

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	//nolint:errcheck
	w.Write(gitRevJSON)
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	isCacherAvailableMu.RLock()
	isCacherAvailableTemp := isCacherAvailable
	isCacherAvailableMu.RUnlock()

	res := struct {
		GitRev          string  `json:"git_rev"`
		Uptime          float64 `json:"uptime"`
		Goroutines      int     `json:"goroutines"`
		CacherAvailable bool    `json:"cacher_status"`
	}{
		GitRev:          gitRev,
		Uptime:          time.Since(startTime).Seconds(),
		Goroutines:      runtime.NumGoroutine(),
		CacherAvailable: isCacherAvailableTemp,
	}
	b, err := json.Marshal(&res)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	if !isCacherAvailableTemp {
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
	//nolint:errcheck
	w.Write(b)
}
