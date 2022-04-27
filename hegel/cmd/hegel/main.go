package main

import (
	"fmt"
	"os"
)

func main() {
	root, err := NewRootCommand()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", err)
		os.Exit(1)
	}

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
