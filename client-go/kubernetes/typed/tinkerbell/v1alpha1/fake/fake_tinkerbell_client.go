// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package fake provides fake typed clients for tinkerbell.org/v1alpha1.
package fake

import (
	typedv1alpha1 "github.com/tinkerbell/tinkerbell/client-go/kubernetes/typed/tinkerbell/v1alpha1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/testing"
)

// TinkerbellV1alpha1 implements TinkerbellV1alpha1Interface.
type TinkerbellV1alpha1 struct {
	*testing.Fake
}

// Hardware returns a fake namespaced Hardware client.
func (c *TinkerbellV1alpha1) Hardware(namespace string) typedv1alpha1.HardwareInterface {
	return newFakeHardware(c, namespace)
}

// RESTClient returns nil because fake clients do not use a REST transport.
func (c *TinkerbellV1alpha1) RESTClient() rest.Interface {
	return nil
}
