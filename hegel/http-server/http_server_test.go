package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/packethost/pkg/log"
	"github.com/tinkerbell/hegel/datamodel"
	grpcserver "github.com/tinkerbell/hegel/grpc-server"
	"github.com/tinkerbell/hegel/hardware"
	"github.com/tinkerbell/hegel/hardware/mock"
	"github.com/tinkerbell/hegel/metrics"
	"github.com/tinkerbell/hegel/xff"
)

func TestMain(m *testing.M) {
	l, _ := log.Init("github.com/tinkerbell/hegel")
	logger = l.Package("httpserver")
	metrics.Init(l)

	hc := mock.HardwareClient{}
	hegelServer = grpcserver.NewServer(logger, hc)

	os.Exit(m.Run())
}

// TestTrustedProxies tests if the actual remote user IP is extracted correctly from the X-FORWARDED-FOR header according to the list of trusted proxies provided.
func TestTrustedProxies(t *testing.T) {
	for name, test := range trustedProxiesTests {
		t.Run(name, func(t *testing.T) {
			hegelServer.SetHardwareClient(mock.HardwareClient{Model: datamodel.TinkServer, Data: test.json})

			mux := &http.ServeMux{}
			mux.HandleFunc("/2009-04-04/", ec2Handler)

			trustedProxies := xff.ParseTrustedProxies(test.trustedProxies)
			xffHandler := xff.HTTPHandler(logger, mux, trustedProxies)

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

func TestGetMetadataCacher(t *testing.T) {
	for name, test := range cacherTests {
		t.Log(name)
		hegelServer.SetHardwareClient(mock.HardwareClient{Data: test.json})

		req, err := http.NewRequest("GET", "/metadata", nil)
		if err != nil {
			t.Fatal(err)
		}

		req.RemoteAddr = mock.UserIP
		resp := httptest.NewRecorder()
		http.HandleFunc("/metadata", getMetadata("", datamodel.Cacher)) // filter not used in cacher mode

		http.DefaultServeMux.ServeHTTP(resp, req)

		if status := resp.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v",
				status, http.StatusOK)
		}

		hw := hardware.ExportedCacher{}
		err = json.Unmarshal(resp.Body.Bytes(), &hw)
		if err != nil {
			t.Error("Error in unmarshalling hardware:", err)
		}

		if hw.ID != test.id {
			t.Errorf("handler returned unexpected id: got %v want %v",
				hw.ID, test.id)
		}
		if hw.PlanSlug != test.planSlug {
			t.Errorf("handler returned unexpected plan slug: got %v want %v",
				hw.PlanSlug, test.planSlug)
		}
	}
}

// TestGetMetadataTinkerbell tests the default use case in tinkerbell mode.
func TestGetMetadataTinkerbell(t *testing.T) {
	customEndpoints := `{"/metadata":".metadata.instance"}`

	for name, test := range tinkerbellTests {
		t.Run(name, func(t *testing.T) {
			hegelServer.SetHardwareClient(mock.HardwareClient{Model: datamodel.TinkServer, Data: test.json})

			mux := &http.ServeMux{}

			err := registerCustomEndpoints(mux, datamodel.TinkServer, customEndpoints)
			if err != nil {
				t.Fatal("Error registering custom endpoints", err)
			}

			req, err := http.NewRequest("GET", "/metadata", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = mock.UserIP
			resp := httptest.NewRecorder()

			mux.ServeHTTP(resp, req)

			if status := resp.Code; status != http.StatusOK {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, http.StatusOK)
			}

			if resp.Body.Bytes() == nil {
				return
			}

			var metadata map[string]interface{}
			err = json.Unmarshal(resp.Body.Bytes(), &metadata)
			if err != nil {
				t.Error("Error in unmarshalling hardware metadata:", err)
			}

			if metadata["crypted_root_password"].(string) != test.password {
				t.Errorf("handler returned unexpected crypted_root_password: got %v want %v",
					metadata["crypted_root_password"], test.password)
			}
		})
	}
}

// TestGetMetadataTinkerbellKant tests the kant specific use case in tinkerbell mode.
func TestGetMetadataTinkerbellKant(t *testing.T) {
	customEndpoints := `{"/metadata":".metadata.instance","/components":".metadata.components","/userdata":".metadata.userdata"}`

	for name, test := range tinkerbellKantTests {
		t.Run(name, func(t *testing.T) {
			hegelServer.SetHardwareClient(mock.HardwareClient{Model: datamodel.TinkServer, Data: test.json})

			mux := &http.ServeMux{}

			err := registerCustomEndpoints(mux, datamodel.TinkServer, customEndpoints)
			if err != nil {
				t.Fatal("Error registering custom endpoints", err)
			}

			req, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = mock.UserIP
			resp := httptest.NewRecorder()

			mux.ServeHTTP(resp, req)

			if status := resp.Code; status != http.StatusOK {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, http.StatusOK)
			}

			if resp.Body.String() != test.response {
				t.Errorf("handler returned with unexpected body: got %v want %v",
					resp.Body.String(), test.response)
			}
		})
	}
}

func TestRegisterEndpoints(t *testing.T) {
	for name, test := range registerEndpointTests {
		t.Run(name, func(t *testing.T) {
			hegelServer.SetHardwareClient(mock.HardwareClient{Model: datamodel.TinkServer, Data: test.json})

			if test.customEndpoints == "" {
				test.customEndpoints = `{"/metadata":".metadata.instance"}`
			}

			mux := &http.ServeMux{}

			err := registerCustomEndpoints(mux, datamodel.TinkServer, test.customEndpoints)
			if test.error != "" {
				if err == nil {
					t.Fatalf("registerCustomEndpoints should have returned error: %v", test.error)
				} else if err.Error() != test.error {
					t.Fatalf("registerCustomEndpoints returned wrong error: got %v want %v", err, test.error)
				}
			}

			req, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = mock.UserIP
			resp := httptest.NewRecorder()

			mux.ServeHTTP(resp, req)

			if status := resp.Code; status != test.status {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, test.status)
			}

			t.Log("response:", resp.Body.String()) // logging response instead of explicitly checking content
			if resp.Body.String() == "" && !test.expectResponseEmpty {
				t.Errorf("handler should have returned a non-empty response")
			}
		})
	}
}

func TestEC2Endpoint(t *testing.T) {
	for name, test := range tinkerbellEC2Tests {
		t.Run(name, func(t *testing.T) {
			hegelServer.SetHardwareClient(mock.HardwareClient{Model: datamodel.TinkServer, Data: test.json})

			http.DefaultServeMux = &http.ServeMux{} // reset registered patterns

			http.HandleFunc("/2009-04-04", ec2Handler) // workaround for making trailing slash optional
			http.HandleFunc("/2009-04-04/", ec2Handler)

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

// test cases for TestGetMetadataCacher.
var cacherTests = map[string]struct {
	id       string
	planSlug string
	json     string
}{
	"cacher": {
		id:       "8978e7d4-1a55-4845-8a66-a5259236b104",
		planSlug: "t1.small.x86",
		json:     mock.CacherDataModel,
	},
}

// test cases for TestGetMetadataTinkerbell.
var tinkerbellTests = map[string]struct {
	id       string
	password string
	json     string
}{
	"tinkerbell": {
		id:       "fde7c87c-d154-447e-9fce-7eb7bdec90c0",
		password: "redacted/",
		json:     mock.TinkerbellDataModel,
	},
	"tinkerbell no metadata": {
		id:       "363115b0-f03d-4ce5-9a15-5514193d131a",
		password: "redacted/",
		json:     mock.TinkerbellNoMetadata,
	},
}

// test cases for TestGetMetadataTinkerbellKant.
var tinkerbellKantTests = map[string]struct {
	url      string
	status   int
	response string
	json     string
}{
	"metadata endpoint": {
		url:      "/metadata",
		status:   200,
		response: `{"facility":"sjc1","hostname":"tink-provisioner","id":"f955e31a-cab6-44d6-872c-9614c2024bb4"}`,
		json:     mock.TinkerbellKant,
	},
	"components endpoint": {
		url:      "/components",
		status:   200,
		response: `{"id":"bc9ce39b-7f18-425b-bc7b-067914fa9786","type":"DiskComponent"}`,
		json:     mock.TinkerbellKant,
	},
	"userdata endpoint": {
		url:    "/userdata",
		status: 200,
		response: `#!/bin/bash

echo "Hello world!"`,
		json: mock.TinkerbellKant,
	},
	"no metadata": {
		url:      "/metadata",
		status:   200,
		response: "",
		json:     mock.TinkerbellNoMetadata,
	},
}

// test cases for TestRegisterEndpoints.
var registerEndpointTests = map[string]struct {
	customEndpoints     string
	url                 string
	status              int
	expectResponseEmpty bool
	error               string
	json                string
}{
	"single custom endpoint": {
		customEndpoints:     `{"/facility": ".metadata.facility"}`,
		url:                 "/facility",
		status:              200,
		expectResponseEmpty: false,
		error:               "",
		json:                mock.TinkerbellDataModel,
	},
	"single custom endpoint, non-metadata": {
		customEndpoints:     `{"/id": ".id"}`,
		url:                 "/id",
		status:              200,
		expectResponseEmpty: false,
		error:               "",
		json:                mock.TinkerbellDataModel,
	},
	"single custom endpoint, invalid url call": {
		customEndpoints:     `{"/userdata": ".metadata.userdata"}`,
		url:                 "/metadata",
		status:              404,
		expectResponseEmpty: false,
		error:               "",
		json:                mock.TinkerbellDataModel,
	},
	"multiple custom endpoints": {
		customEndpoints:     `{"/metadata":".metadata.instance","/components":".metadata.components","/all":".","/userdata":".metadata.userdata"}`,
		url:                 "/components",
		status:              200,
		expectResponseEmpty: false,
		error:               "",
		json:                mock.TinkerbellDataModel,
	},
	"default endpoint": {
		customEndpoints:     "",
		url:                 "/metadata",
		status:              200,
		expectResponseEmpty: false,
		error:               "",
		json:                mock.TinkerbellDataModel,
	},
	"custom endpoints invalid format (not a map)": {
		customEndpoints:     `"/userdata":".metadata.userdata"`,
		url:                 "/userdata",
		status:              404,
		expectResponseEmpty: false,
		error:               "error in parsing custom endpoints: invalid character ':' after top-level value",
		json:                mock.TinkerbellDataModel,
	},
	"custom endpoints invalid format (endpoint missing forward slash)": {
		customEndpoints:     `{"userdata":".metadata.userdata"}`,
		url:                 "/userdata",
		status:              404,
		expectResponseEmpty: false,
		error:               "",
		json:                mock.TinkerbellDataModel,
	},
	"custom endpoints invalid format (invalid jq filter syntax)": {
		customEndpoints:     `{"/userdata":"invalid"}`,
		url:                 "/userdata",
		status:              500,
		expectResponseEmpty: true,
		error:               "",
		json:                mock.TinkerbellDataModel,
	},
	"custom endpoints invalid format (valid jq filter syntax, nonexistent field)": {
		customEndpoints:     `{"/userdata":".nonexistent"}`,
		url:                 "/userdata",
		status:              200,
		expectResponseEmpty: true,
		error:               "",
		json:                mock.TinkerbellDataModel,
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
		error:  "invalid metadata item",
		result: "",
	},
	"invalid query (not a subdirectory)": {
		url:    "/2009-04-04/user-data/hostname",
		error:  "invalid metadata item",
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
			httpreq: "/_packet/version",
		},
		{
			name:    "serve_endpoint_failed",
			status:  404,
			httpreq: "/_packet/version12",
		},
	}
	mport := 52000

	customEndpoints := `{"/metadata":".metadata.instance"}`

	go func() {
		if err := Serve(context.Background(), logger, hegelServer, mport, "grev", time.Now(), "", customEndpoints, ""); err != nil {
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
