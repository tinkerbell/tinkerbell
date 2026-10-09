// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package kubernetes provides a clientset for Tinkerbell APIs.
package kubernetes

import (
	"fmt"
	"net/http"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/client-go/kubernetes/typed/tinkerbell/v1alpha1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
)

// Interface exposes discovery and supported typed clients.
type Interface interface {
	Discovery() discovery.DiscoveryInterface
	TinkerbellV1alpha1() tinkv1alpha1.TinkerbellV1alpha1Interface
}

// Clientset contains clients for supported Tinkerbell API groups.
type Clientset struct {
	*discovery.DiscoveryClient
	tinkerbellV1alpha1 *tinkv1alpha1.TinkerbellV1alpha1Client
}

// Discovery returns the discovery client.
func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	if c == nil {
		return nil
	}
	return c.DiscoveryClient
}

// TinkerbellV1alpha1 returns the tinkerbell.org/v1alpha1 client.
func (c *Clientset) TinkerbellV1alpha1() tinkv1alpha1.TinkerbellV1alpha1Interface {
	return c.tinkerbellV1alpha1
}

// NewForConfig creates a clientset whose clients share one HTTP transport.
func NewForConfig(c *rest.Config) (*Clientset, error) {
	config := *c
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	httpClient, err := rest.HTTPClientFor(&config)
	if err != nil {
		return nil, err
	}
	return NewForConfigAndClient(&config, httpClient)
}

// NewForConfigAndClient creates a clientset using the supplied HTTP client.
func NewForConfigAndClient(c *rest.Config, httpClient *http.Client) (*Clientset, error) {
	config := *c
	if config.RateLimiter == nil && config.QPS > 0 {
		if config.Burst <= 0 {
			return nil, fmt.Errorf("burst must be greater than zero when QPS is set and RateLimiter is nil")
		}
		config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(config.QPS, config.Burst)
	}

	var cs Clientset
	var err error
	cs.tinkerbellV1alpha1, err = tinkv1alpha1.NewForConfigAndClient(&config, httpClient)
	if err != nil {
		return nil, err
	}
	cs.DiscoveryClient, err = discovery.NewDiscoveryClientForConfigAndClient(&config, httpClient)
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

// NewForConfigOrDie creates a clientset and panics if the configuration is invalid.
func NewForConfigOrDie(c *rest.Config) *Clientset {
	clientset, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return clientset
}
