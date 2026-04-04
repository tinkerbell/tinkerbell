package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

//go:generate go tool controller-gen crd webhook paths="../../..." output:crd:artifacts:config=../../crd/bases
//go:generate go tool controller-gen paths="../../..." object:headerFile="../../script/boilerplate.go.txt"
//go:generate go tool buf generate ../.. --config ../../buf.yaml --template ../../buf.gen.yaml --output ../../
func main() {
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)

	if err := Execute(ctx, done, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		done()
		os.Exit(1)
	}

	done()
}
