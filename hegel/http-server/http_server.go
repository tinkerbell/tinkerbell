package httpserver

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/packethost/pkg/env"
	"github.com/packethost/pkg/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	grpcserver "github.com/tinkerbell/hegel/grpc-server"
	"github.com/tinkerbell/hegel/metrics"
	"github.com/tinkerbell/hegel/xff"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var (
	isHardwareClientAvailableMu sync.RWMutex
	isHardwareClientAvailable   bool
	startTime                   time.Time
	metricsPort                 = flag.Int("http_port", env.Int("HEGEL_HTTP_PORT", 50061), "Port to listen on http")
	gitRev                      string
	gitRevJSON                  []byte
	logger                      log.Logger
	hegelServer                 *grpcserver.Server
)

func Serve(l log.Logger, srv *grpcserver.Server, gRev string, t time.Time, customEndpoints string) error {
	startTime = t
	gitRev = gRev
	logger = l
	hegelServer = srv

	go func() {
		c := time.Tick(15 * time.Second)
		for range c {
			checkHardwareClientHealth()
		}
	}()

	mux := &http.ServeMux{}
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/_packet/healthcheck", healthCheckHandler)
	mux.HandleFunc("/_packet/version", versionHandler)

	ec2hf := otelhttp.WithRouteTag("/2009-04-04", http.HandlerFunc(ec2Handler))
	mux.Handle("/2009-04-04", ec2hf) // workaround for making trailing slash optional
	mux.Handle("/2009-04-04/", ec2hf)

	buildSubscriberHandlers()

	err := registerCustomEndpoints(mux, customEndpoints)
	if err != nil {
		l.Fatal(err, "could not register custom endpoints")
	}

	trustedProxies := xff.ParseTrustedProxies()
	http.Handle("/", xff.HTTPHandler(logger, mux, trustedProxies))

	l.With("port", *metricsPort).Info("Starting http server")
	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", *metricsPort), nil)
		if err != nil {
			l.Fatal(err, "failed to serve http")
		}
	}()

	return nil
}

func registerCustomEndpoints(mux *http.ServeMux, customEndpoints string) error {
	if mux == nil {
		mux = http.DefaultServeMux
	}

	endpoints := make(map[string]string)
	err := json.Unmarshal([]byte(customEndpoints), &endpoints)
	if err != nil {
		return errors.Wrap(err, "error in parsing custom endpoints")
	}
	for endpoint, filter := range endpoints {
		route := getMetadata(filter)                 // generates a handler
		hf := otelhttp.WithRouteTag(endpoint, route) // wrap it with otel
		mux.Handle(endpoint, hf)
	}

	return nil
}

func checkHardwareClientHealth() {
	// Get All hardware as a proxy for a healthcheck
	// TODO (patrickdevivo) until Cacher gets a proper healthcheck RPC
	// a la https://github.com/grpc/grpc/blob/master/doc/health-checking.md
	// this will have to do.
	// Note that we don't do anything with the stream (we don't read from it)
	var isHardwareClientAvailableTemp bool

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if hegelServer.HardwareClient().IsHealthy(ctx) {
		isHardwareClientAvailableTemp = true
	}
	cancel()

	isHardwareClientAvailableMu.Lock()
	isHardwareClientAvailable = isHardwareClientAvailableTemp
	isHardwareClientAvailableMu.Unlock()

	if isHardwareClientAvailableTemp {
		metrics.CacherConnected.Set(1)
		metrics.CacherHealthcheck.WithLabelValues("true").Inc()
		logger.With("status", isHardwareClientAvailableTemp).Debug("tick")
	} else {
		metrics.CacherConnected.Set(0)
		metrics.CacherHealthcheck.WithLabelValues("false").Inc()
		metrics.Errors.WithLabelValues("cacher", "healthcheck").Inc()
		logger.With("status", isHardwareClientAvailableTemp).Error(errors.New("client reported unhealthy"))
	}
}
