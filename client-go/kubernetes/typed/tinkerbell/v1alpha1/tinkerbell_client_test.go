// Copyright 2026 The Tinkerbell Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package v1alpha1

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

type observedRequest struct {
	method      string
	path        string
	query       url.Values
	contentType string
	accept      string
	body        []byte
}

func TestHardwareRESTRequests(t *testing.T) {
	t.Parallel()

	requests := make(chan observedRequest, 16)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		requests <- observedRequest{
			method:      request.Method,
			path:        request.URL.Path,
			query:       request.URL.Query(),
			contentType: request.Header.Get("Content-Type"),
			accept:      request.Header.Get("Accept"),
			body:        body,
		}
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Query().Get("watch") == "true" {
			_, _ = io.WriteString(writer, `{"type":"ADDED","object":{"apiVersion":"tinkerbell.org/v1alpha1","kind":"Hardware","metadata":{"name":"node","namespace":"rack"}}}`+"\n")
			return
		}
		if request.Method == http.MethodDelete {
			_, _ = io.WriteString(writer, `{"kind":"Status","apiVersion":"v1","status":"Success"}`)
			return
		}
		if strings.HasSuffix(request.URL.Path, "/hardware") && request.Method == http.MethodGet {
			_, _ = io.WriteString(writer, `{"apiVersion":"tinkerbell.org/v1alpha1","kind":"HardwareList","items":[]}`)
			return
		}
		_, _ = io.WriteString(writer, `{"apiVersion":"tinkerbell.org/v1alpha1","kind":"Hardware","metadata":{"name":"node","namespace":"rack"}}`)
	}))
	t.Cleanup(server.Close)

	client, err := NewForConfigAndClient(&rest.Config{Host: server.URL}, server.Client())
	if err != nil {
		t.Fatalf("NewForConfigAndClient: %v", err)
	}
	hardware := client.Hardware("rack")
	ctx := context.Background()
	object := &tinkv1alpha1.Hardware{ObjectMeta: metav1.ObjectMeta{Name: "node", Namespace: "rack"}}

	assertRequest := func(method, path string, call func() error, check func(observedRequest)) {
		t.Helper()
		if err := call(); err != nil {
			t.Fatalf("request %s %s: %v", method, path, err)
		}
		request := <-requests
		if request.method != method || request.path != path {
			t.Fatalf("request = %s %s, want %s %s", request.method, request.path, method, path)
		}
		if request.accept != "application/json" {
			t.Errorf("Accept = %q, want application/json", request.accept)
		}
		if check != nil {
			check(request)
		}
	}

	base := "/apis/tinkerbell.org/v1alpha1/namespaces/rack/hardware"
	assertRequest(http.MethodGet, base+"/node", func() error {
		_, err := hardware.Get(ctx, "node", metav1.GetOptions{})
		return err
	}, nil)
	assertRequest(http.MethodGet, base, func() error {
		_, err := hardware.List(ctx, metav1.ListOptions{LabelSelector: "site=west"})
		return err
	}, func(request observedRequest) {
		if got := request.query.Get("labelSelector"); got != "site=west" {
			t.Errorf("labelSelector = %q, want site=west", got)
		}
	})
	assertRequest(http.MethodGet, base, func() error {
		watcher, err := hardware.Watch(ctx, metav1.ListOptions{FieldSelector: "metadata.name=node"})
		if err == nil {
			<-watcher.ResultChan()
			watcher.Stop()
		}
		return err
	}, func(request observedRequest) {
		if request.query.Get("watch") != "true" || request.query.Get("fieldSelector") != "metadata.name=node" {
			t.Errorf("watch query = %v", request.query)
		}
	})
	assertRequest(http.MethodPost, base, func() error {
		_, err := hardware.Create(ctx, object, metav1.CreateOptions{FieldManager: "poc"})
		return err
	}, func(request observedRequest) {
		if request.query.Get("fieldManager") != "poc" || request.contentType != "application/json" {
			t.Errorf("create query/content type = %v %q", request.query, request.contentType)
		}
		var decoded tinkv1alpha1.Hardware
		if err := json.Unmarshal(request.body, &decoded); err != nil || decoded.Name != "node" {
			t.Errorf("create body did not contain Hardware node: %s (%v)", request.body, err)
		}
	})
	assertRequest(http.MethodPut, base+"/node", func() error {
		_, err := hardware.Update(ctx, object, metav1.UpdateOptions{})
		return err
	}, nil)
	assertRequest(http.MethodPut, base+"/node/status", func() error {
		_, err := hardware.UpdateStatus(ctx, object, metav1.UpdateOptions{})
		return err
	}, nil)
	assertRequest(http.MethodPatch, base+"/node", func() error {
		_, err := hardware.Patch(ctx, "node", types.MergePatchType, []byte(`{"metadata":{"labels":{"a":"b"}}}`), metav1.PatchOptions{})
		return err
	}, func(request observedRequest) {
		if request.contentType != string(types.MergePatchType) {
			t.Errorf("patch content type = %q", request.contentType)
		}
	})
	assertRequest(http.MethodPatch, base+"/node/status", func() error {
		_, err := hardware.Patch(ctx, "node", types.JSONPatchType, []byte(`[]`), metav1.PatchOptions{}, "status")
		return err
	}, nil)
	assertRequest(http.MethodDelete, base+"/node", func() error {
		return hardware.Delete(ctx, "node", metav1.DeleteOptions{})
	}, nil)
	assertRequest(http.MethodDelete, base, func() error {
		return hardware.DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: "site=west"})
	}, func(request observedRequest) {
		if request.query.Get("labelSelector") != "site=west" {
			t.Errorf("delete collection query = %v", request.query)
		}
	})
}
