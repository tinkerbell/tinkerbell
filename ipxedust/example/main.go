package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	ipxe "github.com/tinkerbell/boots-ipxe"
	cli "github.com/tinkerbell/boots-ipxe/cmd/ipxe"
)

func main() {
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()
	logger := stdr.New(log.New(os.Stdout, "", log.Lshortfile))

	// how := "listenAndServe"
	// how := "serve"
	how := "serveCLI"
	switch how {
	case "serve":
		logger.Info("serve")
		if err := serve(ctx, logger); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error(err, "failed to serve ipxe")
		}
	case "listenAndServe":
		logger.Info("listening and serve")
		if err := listenAndServe(ctx, logger); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error(err, "failed to listen and serve ipxe")
		}
	case "serveCLI":
		if err := serveCLI(ctx, logger); err != nil {
			logger.Error(err, "failed to serve ipxe cli")
		}
	default:
	}

	logger.Info("exiting")
}

func listenAndServe(ctx context.Context, logger logr.Logger) error {
	s := ipxe.Server{Log: logger}
	return s.ListenAndServe(ctx)
}

func serve(ctx context.Context, logger logr.Logger) error {
	conn, err := net.Listen("tcp", "0.0.0.0:0") // nolint: gosec // this is just example code
	if err != nil {
		return err
	}
	a, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	if err != nil {
		return err
	}
	uconn, err := net.ListenUDP("udp", a)
	if err != nil {
		return err
	}

	s := ipxe.Server{Log: logger}
	return s.Serve(ctx, conn, uconn)
}

func serveCLI(ctx context.Context, _ logr.Logger) error {
	return cli.Execute(ctx, os.Args[1:])
}
