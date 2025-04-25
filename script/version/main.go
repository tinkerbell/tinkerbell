package main

import (
	"fmt"

	"github.com/tinkerbell/tinkerbell/pkg/build"
)

func main() {
	fmt.Print(build.GitRevision())
}
