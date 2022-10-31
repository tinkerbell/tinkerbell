package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/packethost/pkg/log"
	"github.com/tinkerbell/hegel/internal/datamodel"
	"github.com/tinkerbell/hegel/internal/hardware/mock"
	"github.com/tinkerbell/hegel/internal/http/handler"
)

func TestEC2Endpoints(t *testing.T) {
	logger, err := log.Init(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// test cases for TestEC2Endpoint.
	cases := map[string]struct {
		url      string
		status   int
		response string
		json     string
	}{
		"user-data": {
			url:    "/2009-04-04/user-data",
			status: 200,
			response: `#!/bin/bash

echo "Hello world!"`,
			json: mock.TinkerbellKantEC2,
		},
		"meta-data": {
			url:    "/2009-04-04/meta-data",
			status: 200,
			response: `facility
hostname
instance-id
iqn
local-hostname
local-ipv4
operating-system
plan
public-ipv4
public-ipv6
public-keys
tags`,
			json: mock.TinkerbellKantEC2,
		},
		"instance-id": {
			url:      "/2009-04-04/meta-data/instance-id",
			status:   200,
			response: "7c9a5711-aadd-4fa0-8e57-789431626a27",
			json:     mock.TinkerbellKantEC2,
		},
		"public-ipv4": {
			url:      "/2009-04-04/meta-data/public-ipv4",
			status:   200,
			response: "139.175.86.114",
			json:     mock.TinkerbellKantEC2,
		},
		"public-ipv6": {
			url:      "/2009-04-04/meta-data/public-ipv6",
			status:   200,
			response: "2604:1380:1000:ca00::7",
			json:     mock.TinkerbellKantEC2,
		},
		"local-ipv4": {
			url:      "/2009-04-04/meta-data/local-ipv4",
			status:   200,
			response: "10.87.63.3",
			json:     mock.TinkerbellKantEC2,
		},
		"tags": {
			url:    "/2009-04-04/meta-data/tags",
			status: 200,
			response: `hello
test`,
			json: mock.TinkerbellKantEC2,
		},
		"operating-system slug": {
			url:      "/2009-04-04/meta-data/operating-system/slug",
			status:   200,
			response: "ubuntu_18_04",
			json:     mock.TinkerbellKantEC2,
		},
		"invalid metadata item": {
			url:      "/2009-04-04/meta-data/invalid",
			status:   404,
			response: "404 not found",
			json:     mock.TinkerbellKantEC2,
		},
		"valid metadata item, but not found": {
			url:      "/2009-04-04/meta-data/public-keys",
			status:   200,
			response: "",
			json:     mock.TinkerbellNoMetadata,
		},
		"with trailing slash": {
			url:      "/2009-04-04/meta-data/hostname/",
			status:   200,
			response: "tink-provisioner",
			json:     mock.TinkerbellKantEC2,
		},
		"base endpoint": {
			url:    "/2009-04-04",
			status: 200,
			response: `meta-data
user-data`,
			json: mock.TinkerbellKantEC2,
		},
		"base endpoint with trailing slash": {
			url:    "/2009-04-04/",
			status: 200,
			response: `meta-data
user-data`,
			json: mock.TinkerbellKantEC2,
		},
		"spot instance with empty (but still present) spot field": {
			url:    "/2009-04-04/meta-data",
			status: 200,
			response: `facility
hostname
instance-id
iqn
local-hostname
local-ipv4
operating-system
plan
public-ipv4
public-ipv6
public-keys
spot
tags`,
			json: mock.TinkerbellKantEC2SpotEmpty,
		},
		"termination-time": {
			url:      "/2009-04-04/meta-data/spot/termination-time",
			status:   200,
			response: "now",
			json:     mock.TinkerbellKantEC2SpotWithTermination,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			client := mock.HardwareClient{Model: datamodel.TinkServer, Data: test.json}

			handlr, err := handler.New(logger, handler.EC2, client)
			if err != nil {
				t.Fatal(err)
			}

			req, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = mock.UserIP
			resp := httptest.NewRecorder()

			handlr.ServeHTTP(resp, req)

			if status := resp.Code; status != test.status {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, test.status)
			}

			if resp.Body.String() != test.response {
				t.Errorf("handler returned wrong body: got %v want %v", resp.Body.String(), test.response)
			}
		})
	}
}
