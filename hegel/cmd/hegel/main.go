package main

import (
	"fmt"
	"os"

	"github.com/tinkerbell/hegel/internal/cmd"
)

func main() {
	root, err := cmd.NewRootCommand()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
