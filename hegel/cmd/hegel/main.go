package main

import (
	"context"
	"flag"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/equinix-labs/otel-init-go/otelinit"
	"github.com/packethost/pkg/env"
	"github.com/packethost/pkg/log"
	grpcserver "github.com/tinkerbell/hegel/grpc-server"
	httpserver "github.com/tinkerbell/hegel/http-server"
	"github.com/tinkerbell/hegel/metrics"
)

var (
	GitRev          string
	logger          log.Logger
	customEndpoints string
)

func main() {
	flag.Parse()
	// setup structured logging
	l, err := log.Init("github.com/tinkerbell/hegel")
	if err != nil {
		logger.Fatal(err)
	}
	logger = l.Package("main")
	defer l.Close()
	metrics.Init(l)

	ctx := context.Background()
	ctx, otelShutdown := otelinit.InitOpenTelemetry(ctx, "hegel")
	defer otelShutdown(ctx)

	metrics.State.Set(metrics.Initializing)

	hegelServer, err := grpcserver.NewServer(l, nil)
	if err != nil {
		logger.Fatal(err, "failed to create hegel server")
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	var ret error
	ctx, cancel := context.WithCancel(context.TODO())
	var once sync.Once
	var wg sync.WaitGroup
	customEndpoints = env.Get("CUSTOM_ENDPOINTS", `{"/metadata":".metadata.instance"}`)

	runGoroutine(&ret, cancel, &once, &wg, func() error {
		return httpserver.Serve(ctx, logger, hegelServer, GitRev, time.Now(), customEndpoints)
	})

	runGoroutine(&ret, cancel, &once, &wg, func() error {
		return grpcserver.Serve(ctx, logger, hegelServer)
	})
	runGoroutine(&ret, cancel, &once, &wg, func() error {
		select {
		case sig, ok := <-c:
			if ok {
				l.With("signal", sig).Info("received stop signal, gracefully shutting down")
				cancel()
			}
		case <-ctx.Done():
		}
		return nil
	})
	l.Info("waiting")
	wg.Wait()
	if ret != nil {
		logger.Fatal(ret)
	}
}

func runGoroutine(ret *error, cancel context.CancelFunc, once *sync.Once, wg *sync.WaitGroup, fn func() error) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := fn()
		if err != nil {
			// we only care about the first error
			once.Do(func() {
				*ret = err
				cancel()
			})
		}
	}()
}
