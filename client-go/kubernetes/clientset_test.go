// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package kubernetes

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type recordingTransport struct {
	lock  sync.Mutex
	paths []string
}

func (r *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r.lock.Lock()
	r.paths = append(r.paths, request.URL.Path)
	r.lock.Unlock()
	body := `{"apiVersion":"tinkerbell.org/v1alpha1","kind":"Hardware","metadata":{"name":"node"}}`
	if request.URL.Path == "/version" {
		body = `{"gitVersion":"v1.36.0"}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}, nil
}

func TestClientsetSharesSuppliedHTTPClient(t *testing.T) {
	t.Parallel()

	transport := &recordingTransport{}
	httpClient := &http.Client{Transport: transport}
	clientset, err := NewForConfigAndClient(&rest.Config{Host: "https://example.invalid"}, httpClient)
	if err != nil {
		t.Fatalf("NewForConfigAndClient: %v", err)
	}
	if _, err := clientset.TinkerbellV1alpha1().Hardware("rack").Get(context.Background(), "node", metav1.GetOptions{}); err != nil {
		t.Fatalf("Hardware.Get: %v", err)
	}
	if _, err := clientset.Discovery().ServerVersion(); err != nil {
		t.Fatalf("Discovery.ServerVersion: %v", err)
	}

	transport.lock.Lock()
	defer transport.lock.Unlock()
	if len(transport.paths) != 2 || transport.paths[0] != "/apis/tinkerbell.org/v1alpha1/namespaces/rack/hardware/node" || transport.paths[1] != "/version" {
		t.Fatalf("shared transport paths = %v", transport.paths)
	}
}
