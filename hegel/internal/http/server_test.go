package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/packethost/pkg/log"
	"github.com/stretchr/testify/require"
	"github.com/tinkerbell/hegel/internal/datamodel"
	"github.com/tinkerbell/hegel/internal/hardware/mock"
	_ "github.com/tinkerbell/hegel/internal/metrics" // Initialize metrics.
	"github.com/tinkerbell/hegel/internal/xff"
)

// TestTrustedProxies tests if the actual remote user IP is extracted correctly from the X-FORWARDED-FOR header according to the list of trusted proxies provided.
func TestTrustedProxies(t *testing.T) {
	logger := log.Test(t, t.Name())

	for name, test := range trustedProxiesTests {
		t.Run(name, func(t *testing.T) {
			client := mock.HardwareClient{Model: datamodel.TinkServer, Data: test.json}

			mux := &http.ServeMux{}
			mux.Handle("/2009-04-04/", EC2MetadataHandler(logger, client))

			trustedProxies, err := xff.Parse(test.trustedProxies)
			if err != nil {
				t.Fatal(err)
			}

			xffHandler, err := xff.Middleware(mux, trustedProxies)
			require.NoError(t, err)

			req, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Add("X-Forwarded-For", test.xffHeader)
			req.RemoteAddr = test.lastProxyIP // the ip of the last proxy the request goes through before reaching the server

			resp := httptest.NewRecorder()
			xffHandler.ServeHTTP(resp, req)

			if status := resp.Code; status != test.status {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, test.status)
			}

			if resp.Body.String() != test.resp {
				t.Errorf("handler returned wrong status code: got %v want %v",
					resp.Body.String(), test.resp)
			}
		})
	}
}

func TestEC2Endpoint(t *testing.T) {
	logger, err := log.Init(t.Name())
	require.NoError(t, err)

	for name, test := range tinkerbellEC2Tests {
		t.Run(name, func(t *testing.T) {
			client := mock.HardwareClient{Model: datamodel.TinkServer, Data: test.json}
			http.DefaultServeMux = &http.ServeMux{} // reset registered patterns

			// workaround for making trailing slash optional
			http.Handle("/2009-04-04", EC2MetadataHandler(logger, client))
			http.Handle("/2009-04-04/", EC2MetadataHandler(logger, client))

			req, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = mock.UserIP
			resp := httptest.NewRecorder()

			http.DefaultServeMux.ServeHTTP(resp, req)

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

func TestFilterMetadata(t *testing.T) {
	for name, test := range tinkerbellFilterMetadataTests {
		t.Run(name, func(t *testing.T) {
			res, err := filterMetadata([]byte(test.json), test.filter)
			if test.error != "" {
				if err == nil {
					t.Errorf("FilterMetadata should have returned error: %v", test.error)
				} else if err.Error() != test.error {
					t.Errorf("FilterMetadata returned wrong error: got %v want %v", err, test.error)
				}
			}

			if string(res) != test.result {
				t.Errorf("FilterMetadata returned wrong result: got %s want %v", res, test.result)
			}
		})
	}
}

func TestProcessEC2Query(t *testing.T) {
	for name, test := range processEC2QueryTests {
		t.Run(name, func(t *testing.T) {
			res, err := processEC2Query(test.url)
			if test.error != "" {
				if err == nil {
					t.Fatalf("processEC2Query should have returned error: %v", test.error)
				} else if err.Error() != test.error {
					t.Fatalf("processEC2Query returned wrong error: got %v want %v", err, test.error)
				}
			}

			if !reflect.DeepEqual(res, test.result) {
				t.Errorf("handler returned wrong result: got %v want %v", res, test.result)
			}
		})
	}
}

// TestEC2FiltersMap checks if the all the metadata items are listed in their corresponding directory-listing filter
// itemsFromQueries are the metadata items "extracted" from the queries (keys) of the ec2Filters map
// itemsFromFilter are the metadata items "extracted" from the filters (values) of the ec2Filters map.
func TestEC2FiltersMap(t *testing.T) {
	directories := make(map[string][]string) // keys are the directory base paths; values are a list of metadata items that are under the base paths

	for query := range ec2Filters {
		basePath, metadataItem := path.Split(query)
		directories[basePath] = append(directories[basePath], metadataItem)
	}

	for basePath, metadataItems := range directories {
		if basePath == "" { // ignore the `"": []` entry
			continue
		}
		t.Run(basePath, func(t *testing.T) {
			hw := `{"metadata":{"instance":{"spot":{}}}}` // to make sure the 'spot' metadata item will be included
			query := strings.TrimSuffix(basePath, "/")
			dirListFilter := ec2Filters[query] // get the directory-list filter
			itemsFromFilter, err := filterMetadata([]byte(hw), dirListFilter)
			if err != nil {
				t.Errorf("failed to filter metadata: %s", err)
			}

			sort.Strings(metadataItems)
			itemsFromQueries := strings.Join(metadataItems, "\n")

			if string(itemsFromFilter) != itemsFromQueries {
				t.Error("directory-list does not match the actual queries")
				t.Errorf("from filter: %s", itemsFromFilter)
				t.Errorf("from queries: %s", itemsFromQueries)
			}
		})
	}
}

// test cases for TestTrustedProxies.
var trustedProxiesTests = map[string]struct {
	trustedProxies string
	url            string
	xffHeader      string // the X-Forwarded-For header does not include the IP of the last proxy
	lastProxyIP    string // the IP must include a port, but the port number is completely arbitrary
	status         int
	resp           string
	json           string
}{
	"single proxy": {
		trustedProxies: "172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      mock.UserIP,
		lastProxyIP:    "172.18.0.1:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           mock.TinkerbellKant,
	},
	"multiple proxies": {
		trustedProxies: "172.18.0.5, 172.18.0.6, 172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mock.UserIP, "172.18.0.1", "172.18.0.5"}, ", "),
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           mock.TinkerbellKant,
	},
	"single proxy, no trusted proxies set": {
		trustedProxies: "",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      mock.UserIP,
		lastProxyIP:    "172.18.0.1:8080",
		status:         404,
		resp:           "",
		json:           mock.TinkerbellKant,
	},
	"single proxy, multiple trusted proxies set (proxy in list)": {
		trustedProxies: "172.18.0.6, 172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      mock.UserIP,
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           mock.TinkerbellKant,
	},
	"single proxy, multiple trusted proxies set (proxy not in list)": {
		trustedProxies: "172.18.0.5, 172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      mock.UserIP,
		lastProxyIP:    "172.18.0.6:8080",
		status:         404,
		resp:           "",
		json:           mock.TinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (one proxy not in list)": {
		trustedProxies: "172.18.0.5, 172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mock.UserIP, "172.18.0.6"}, ", "),
		lastProxyIP:    "172.18.0.5:8080",
		status:         404,
		resp:           "",
		json:           mock.TinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (proxies in mask)": {
		trustedProxies: "172.18.1.1, 172.18.0.0/29",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mock.UserIP, "172.18.0.5", "172.18.0.6"}, ", "),
		lastProxyIP:    "172.18.1.1:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           mock.TinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (multiple masks)": {
		trustedProxies: "172.18.1.0/29, 172.18.0.0/27",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mock.UserIP, "172.18.0.5", "172.18.1.6"}, ", "),
		lastProxyIP:    "172.18.1.1:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           mock.TinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (overlapping masks)": {
		trustedProxies: "172.18.0.0/29, 172.18.0.0/27",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mock.UserIP, "172.18.0.5", "172.18.0.27"}, ", "),
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           mock.TinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (one proxy not in mask)": {
		trustedProxies: "172.18.0.0/29",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mock.UserIP, "172.18.0.5", "172.18.1.1"}, ", "),
		lastProxyIP:    "172.18.0.1:8080",
		status:         404,
		resp:           "",
		json:           mock.TinkerbellKant,
	},
	"multiple trusted proxies set (no spaces)": {
		trustedProxies: "172.18.0.6,172.18.0.5,172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mock.UserIP, "172.18.0.5", "172.18.0.1"}, ", "),
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           mock.TinkerbellKant,
	},
	"multiple trusted proxies set (extra commas)": {
		trustedProxies: "172.18.0.6,, 172.18.0.5,172.18.0.1,",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mock.UserIP, "172.18.0.5", "172.18.0.1"}, ", "),
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           mock.TinkerbellKant,
	},
}

// test cases for TestEC2Endpoint.
var tinkerbellEC2Tests = map[string]struct {
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

// test cases for TestFilterMetadata.
var tinkerbellFilterMetadataTests = map[string]struct {
	filter string
	result string
	error  string
	json   string
}{
	"single result (simple)": {
		filter: ec2Filters["/user-data"],
		result: `#!/bin/bash

echo "Hello world!"`,
		error: "",
		json:  mock.TinkerbellFilterMetadata,
	},
	"single result (complex)": {
		filter: ec2Filters["/meta-data/public-ipv4"],
		result: "139.175.86.114",
		error:  "",
		json:   mock.TinkerbellFilterMetadata,
	},
	"multiple results (separated list results from hardware)": {
		filter: ec2Filters["/meta-data/tags"],
		result: `hello
test`,
		error: "",
		json:  mock.TinkerbellFilterMetadata,
	},
	"multiple results (separated list results from filter)": {
		filter: ec2Filters["/meta-data/operating-system"],
		result: `distro
image_tag
license_activation
slug
version`,
		error: "",
		json:  mock.TinkerbellFilterMetadata,
	},
	"multiple results (/meta-data filter with spot field present)": {
		filter: ec2Filters["/meta-data"],
		result: `facility
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
		error: "",
		json:  mock.TinkerbellFilterMetadata,
	},
	"invalid filter syntax": {
		filter: "invalid",
		error:  "error while filtering with gojq: function not defined: invalid/0",
		json:   mock.TinkerbellFilterMetadata,
	},
	"valid filter syntax, nonexistent field": {
		filter: "metadata.nonexistent",
		result: "",
		error:  "",
		json:   mock.TinkerbellFilterMetadata,
	},
	"empty string filter": {
		filter: "",
		result: mock.TinkerbellFilterMetadata,
		error:  "",
		json:   mock.TinkerbellFilterMetadata,
	},
	"list filter on nonexistent field, without '?'": {
		filter: ".metadata.nonexistent[]",
		result: "",
		error:  "error while filtering with gojq: cannot iterate over: null",
		json:   mock.TinkerbellFilterMetadata,
	},
	"list filter on nonexistent field, with '?'": {
		filter: ".metadata.nonexistent[]?",
		result: "",
		error:  "",
		json:   mock.TinkerbellFilterMetadata,
	},
}

// test cases for TestProcessEC2Query.
var processEC2QueryTests = map[string]struct {
	url    string
	error  string
	result string
}{
	"hardware filter result (simple query)": {
		url:    "/2009-04-04/user-data",
		error:  "",
		result: ec2Filters["/user-data"],
	},
	"hardware filter result (long query)": {
		url:    "/2009-04-04/meta-data/operating-system/license_activation/state",
		error:  "",
		result: ec2Filters["/meta-data/operating-system/license_activation/state"],
	},
	"directory-listing filter result": {
		url:    "/2009-04-04/meta-data/operating-system/license_activation",
		error:  "",
		result: ec2Filters["/meta-data/operating-system/license_activation"],
	},
	"directory-listing filter result (base endpoint)": {
		url:    "/2009-04-04/",
		result: ec2Filters[""],
	},
	"directory-listing result (/meta-data endpoint)": {
		url:    "/2009-04-04/meta-data",
		error:  "",
		result: ec2Filters["/meta-data"],
	},
	"invalid query (invalid metadata item)": {
		url:    "/2009-04-04/invalid",
		error:  "invalid metadata item: /invalid",
		result: "",
	},
	"invalid query (not a subdirectory)": {
		url:    "/2009-04-04/user-data/hostname",
		error:  "invalid metadata item: /user-data/hostname",
		result: "",
	},
}

func TestServe(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		httpreq string
	}{
		{
			name:    "serve_endpoint_success",
			status:  200,
			httpreq: "/versionz",
		},
		{
			name:    "serve_endpoint_failed",
			status:  404,
			httpreq: "/version12",
		},
	}
	mport := 52000

	logger, err := log.Init(t.Name())
	require.NoError(t, err)

	go func() {
		if err := Serve(context.Background(), logger, mock.HardwareClient{}, mport, time.Now(), "", "", false); err != nil {
			t.Errorf("Serve() error = %v", err)
		}
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%v"+"%v", mport, tt.httpreq), nil)
			if err != nil {
				t.Fatalf("request creation failed: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if status := resp.StatusCode; status != tt.status {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.status)
			}

			err = resp.Body.Close()
			if err != nil {
				t.Errorf("close failed: %v", err)
			}
		})
	}
}
