//go:build tools
// +build tools

package main

import (
	_ "github.com/onsi/ginkgo/ginkgo"
	_ "sigs.k8s.io/kustomize/kustomize/v5"
)
