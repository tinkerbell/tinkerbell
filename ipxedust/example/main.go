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
)

func main() {
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()
	logger := stdr.New(log.New(os.Stdout, "", log.Lshortfile))

	//how := "listenAndServe"
	how := "serve"
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
	default:
	}

	logger.Info("exiting")
}

func listenAndServe(ctx context.Context, logger logr.Logger) error {
	s := ipxe.Server{Log: logger}
	return s.ListenAndServe(ctx)
}

func serve(ctx context.Context, logger logr.Logger) error {
	conn, err := net.Listen("tcp", "0.0.0.0:0")
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
