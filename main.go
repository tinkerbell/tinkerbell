package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	_, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()
}
