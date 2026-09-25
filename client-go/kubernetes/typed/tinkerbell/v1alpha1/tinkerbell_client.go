// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package v1alpha1 provides a typed client for tinkerbell.org/v1alpha1.
package v1alpha1

import (
	"net/http"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// TinkerbellV1alpha1Interface provides access to tinkerbell.org/v1alpha1 resources.
type TinkerbellV1alpha1Interface interface {
	RESTClient() rest.Interface
	HardwareGetter
}

// TinkerbellV1alpha1Client is used to interact with tinkerbell.org/v1alpha1.
type TinkerbellV1alpha1Client struct {
	restClient rest.Interface
}

// Hardware returns a namespaced Hardware client.
func (c *TinkerbellV1alpha1Client) Hardware(namespace string) HardwareInterface {
	return newHardware(c, namespace)
}

// NewForConfig creates a client using a transport built from c.
func NewForConfig(c *rest.Config) (*TinkerbellV1alpha1Client, error) {
	config := *c
	setConfigDefaults(&config)
	httpClient, err := rest.HTTPClientFor(&config)
	if err != nil {
		return nil, err
	}
	return NewForConfigAndClient(&config, httpClient)
}

// NewForConfigAndClient creates a client using the supplied HTTP client.
func NewForConfigAndClient(c *rest.Config, httpClient *http.Client) (*TinkerbellV1alpha1Client, error) {
	config := *c
	setConfigDefaults(&config)
	client, err := rest.RESTClientForConfigAndClient(&config, httpClient)
	if err != nil {
		return nil, err
	}
	return &TinkerbellV1alpha1Client{restClient: client}, nil
}

// NewForConfigOrDie creates a client and panics if the configuration is invalid.
func NewForConfigOrDie(c *rest.Config) *TinkerbellV1alpha1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a client using c.
func New(c rest.Interface) *TinkerbellV1alpha1Client {
	return &TinkerbellV1alpha1Client{restClient: c}
}

func setConfigDefaults(config *rest.Config) {
	gv := tinkv1alpha1.GroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = rest.CodecFactoryForGeneratedClient(scheme.Scheme, scheme.Codecs).WithoutConversion()
	config.ContentType = "application/json"
	config.AcceptContentTypes = "application/json"
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
}

// RESTClient returns the underlying REST client.
func (c *TinkerbellV1alpha1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
