package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/packethost/hegel/xff"
	"github.com/tinkerbell/tink/protos/packet"
)

// TestTrustedProxies tests if the actual remote user IP is extracted correctly from the X-FORWARDED-FOR header according to the list of trusted proxies provided
func TestTrustedProxies(t *testing.T) {
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	defer os.Setenv("DATA_MODEL_VERSION", dataModelVersion)
	os.Setenv("DATA_MODEL_VERSION", "1")

	trustedProxies := os.Getenv("TRUSTED_PROXIES")
	defer os.Setenv("TRUSTED_PROXIES", trustedProxies)

	for name, test := range trustedProxiesTests {
		t.Run(name, func(t *testing.T) {
			os.Setenv("TRUSTED_PROXIES", test.trustedProxies)
			hegelServer.hardwareClient = hardwareGetterMock{test.json}

			mux := &http.ServeMux{}
			mux.HandleFunc("/2009-04-04/", ec2Handler)

			trustedProxies := xff.ParseTrustedProxies()
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
		t.Run(name, func(t *testing.T) {
			hegelServer.hardwareClient = hardwareGetterMock{test.json}

			dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
			defer os.Setenv("DATA_MODEL_VERSION", dataModelVersion)
			os.Unsetenv("DATA_MODEL_VERSION")

			req, err := http.NewRequest("GET", "/metadata", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = mockUserIP
			resp := httptest.NewRecorder()
			http.HandleFunc("/metadata", getMetadata("")) // filter not used in cacher mode

			http.DefaultServeMux.ServeHTTP(resp, req)

			if status := resp.Code; status != http.StatusOK {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, http.StatusOK)
			}

			hw := exportedHardwareCacher{}
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
		})
	}
}

// TestGetMetadataTinkerbell tests the default use case in tinkerbell mode
func TestGetMetadataTinkerbell(t *testing.T) {
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	defer os.Setenv("DATA_MODEL_VERSION", dataModelVersion)
	os.Setenv("DATA_MODEL_VERSION", "1")

	defaultCustomEndpoints, err := parseCustomEndpoints("")
	if err != nil {
		t.Error(err)
	}
	for name, test := range tinkerbellTests {
		t.Run(name, func(t *testing.T) {
			hegelServer.hardwareClient = hardwareGetterMock{test.json}

			mux := &http.ServeMux{}

			err := registerCustomEndpoints(mux, defaultCustomEndpoints)
			if err != nil {
				t.Fatal("Error registering custom endpoints", err)
			}

			req, err := http.NewRequest("GET", "/metadata", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = mockUserIP
			resp := httptest.NewRecorder()

			mux.ServeHTTP(resp, req)

			if status := resp.Code; status != http.StatusOK {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, http.StatusOK)
			}

			if resp.Body.Bytes() == nil {
				return
			}

			var metadata packet.Metadata
			err = json.Unmarshal(resp.Body.Bytes(), &metadata)
			if err != nil {
				t.Error("Error in unmarshalling hardware metadata:", err)
			}

			if metadata.BondingMode != test.bondingMode {
				t.Errorf("handler returned unexpected bonding mode: got %v want %v",
					metadata.BondingMode, test.bondingMode)
			}
		})
	}
}

// TestGetMetadataTinkerbellKant tests the kant specific use case in tinkerbell mode
func TestGetMetadataTinkerbellKant(t *testing.T) {
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	defer os.Setenv("DATA_MODEL_VERSION", dataModelVersion)
	os.Setenv("DATA_MODEL_VERSION", "1")

	customEndpoints := map[string]string{
		"/metadata":   ".metadata.instance",
		"/components": ".metadata.components",
		"/userdata":   ".metadata.userdata",
	}
	for name, test := range tinkerbellKantTests {
		t.Run(name, func(t *testing.T) {
			hegelServer.hardwareClient = hardwareGetterMock{test.json}

			mux := &http.ServeMux{}

			err := registerCustomEndpoints(mux, customEndpoints)
			if err != nil {
				t.Fatal("Error registering custom endpoints", err)
			}

			req, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = mockUserIP
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
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	defer os.Setenv("DATA_MODEL_VERSION", dataModelVersion)
	os.Setenv("DATA_MODEL_VERSION", "1")

	for name, test := range registerEndpointTests {
		t.Run(name, func(t *testing.T) {
			hegelServer.hardwareClient = hardwareGetterMock{test.json}

			customEndpoints, err := parseCustomEndpoints(test.customEndpoints)
			if test.error != "" {
				if err == nil {
					t.Fatalf("parseCustomEndpoints should have returned error: %v", test.error)
				} else if err.Error() != test.error {
					t.Fatalf("parseCustomEndpoints returned wrong error: got %v want %v", err, test.error)
				}
				return
			}

			mux := &http.ServeMux{}

			err = registerCustomEndpoints(mux, customEndpoints)
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
			req.RemoteAddr = mockUserIP
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
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	defer os.Setenv("DATA_MODEL_VERSION", dataModelVersion)
	os.Setenv("DATA_MODEL_VERSION", "1")

	for name, test := range tinkerbellEC2Tests {
		t.Run(name, func(t *testing.T) {
			hegelServer.hardwareClient = hardwareGetterMock{test.json}

			http.DefaultServeMux = &http.ServeMux{} // reset registered patterns

			http.HandleFunc("/2009-04-04", ec2Handler) // workaround for making trailing slash optional
			http.HandleFunc("/2009-04-04/", ec2Handler)

			req, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = mockUserIP
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
// itemsFromFilter are the metadata items "extracted" from the filters (values) of the ec2Filters map
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

// test cases for TestTrustedProxies
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
		xffHeader:      mockUserIP,
		lastProxyIP:    "172.18.0.1:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           tinkerbellKant,
	},
	"multiple proxies": {
		trustedProxies: "172.18.0.5, 172.18.0.6, 172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mockUserIP, "172.18.0.1", "172.18.0.5"}, ", "),
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           tinkerbellKant,
	},
	"single proxy, no trusted proxies set": {
		url:         "/2009-04-04/meta-data/hostname",
		xffHeader:   mockUserIP,
		lastProxyIP: "172.18.0.1:8080",
		status:      404,
		json:        tinkerbellKant,
	},
	"single proxy, multiple trusted proxies set (proxy in list)": {
		trustedProxies: "172.18.0.6, 172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      mockUserIP,
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           tinkerbellKant,
	},
	"single proxy, multiple trusted proxies set (proxy not in list)": {
		trustedProxies: "172.18.0.5, 172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      mockUserIP,
		lastProxyIP:    "172.18.0.6:8080",
		status:         404,
		json:           tinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (one proxy not in list)": {
		trustedProxies: "172.18.0.5, 172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mockUserIP, "172.18.0.6"}, ", "),
		lastProxyIP:    "172.18.0.5:8080",
		status:         404,
		json:           tinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (proxies in mask)": {
		trustedProxies: "172.18.1.1, 172.18.0.0/29",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mockUserIP, "172.18.0.5", "172.18.0.6"}, ", "),
		lastProxyIP:    "172.18.1.1:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           tinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (multiple masks)": {
		trustedProxies: "172.18.1.0/29, 172.18.0.0/27",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mockUserIP, "172.18.0.5", "172.18.1.6"}, ", "),
		lastProxyIP:    "172.18.1.1:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           tinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (overlapping masks)": {
		trustedProxies: "172.18.0.0/29, 172.18.0.0/27",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mockUserIP, "172.18.0.5", "172.18.0.27"}, ", "),
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           tinkerbellKant,
	},
	"multiple proxies, multiple trusted proxies set (one proxy not in mask)": {
		trustedProxies: "172.18.0.0/29",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mockUserIP, "172.18.0.5", "172.18.1.1"}, ", "),
		lastProxyIP:    "172.18.0.1:8080",
		status:         404,
		json:           tinkerbellKant,
	},
	"multiple trusted proxies set (no spaces)": {
		trustedProxies: "172.18.0.6,172.18.0.5,172.18.0.1",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mockUserIP, "172.18.0.5", "172.18.0.1"}, ", "),
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           tinkerbellKant,
	},
	"multiple trusted proxies set (extra commas)": {
		trustedProxies: "172.18.0.6,, 172.18.0.5,172.18.0.1,",
		url:            "/2009-04-04/meta-data/hostname",
		xffHeader:      strings.Join([]string{mockUserIP, "172.18.0.5", "172.18.0.1"}, ", "),
		lastProxyIP:    "172.18.0.6:8080",
		status:         200,
		resp:           "tink-provisioner",
		json:           tinkerbellKant,
	},
}

// test cases for TestGetMetadataCacher
var cacherTests = map[string]struct {
	id       string
	planSlug string
	json     string
}{
	"cacher": {
		id:       "8978e7d4-1a55-4845-8a66-a5259236b104",
		planSlug: "t1.small.x86",
		json:     cacherDataModel,
	},
}

// test cases for TestGetMetadataTinkerbell
var tinkerbellTests = map[string]struct {
	id          string
	bondingMode int64
	json        string
}{
	"tinkerbell": {
		id:          "fde7c87c-d154-447e-9fce-7eb7bdec90c0",
		bondingMode: 5,
		json:        tinkerbellDataModel,
	},
	"tinkerbell no metadata": {
		id:   "363115b0-f03d-4ce5-9a15-5514193d131a",
		json: tinkerbellNoMetadata,
	},
}

// test cases for TestGetMetadataTinkerbellKant
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
		json:     tinkerbellKant,
	},
	"components endpoint": {
		url:      "/components",
		status:   200,
		response: `{"id":"bc9ce39b-7f18-425b-bc7b-067914fa9786","type":"DiskComponent"}`,
		json:     tinkerbellKant,
	},
	"userdata endpoint": {
		url:    "/userdata",
		status: 200,
		response: `#!/bin/bash

echo "Hello world!"`,
		json: tinkerbellKant,
	},
	"no metadata": {
		url:      "/metadata",
		status:   200,
		response: "",
		json:     tinkerbellNoMetadata,
	},
}

// test cases for TestRegisterEndpoints
var registerEndpointTests = map[string]struct {
	customEndpoints     string
	url                 string
	status              int
	expectResponseEmpty bool
	error               string
	json                string
}{
	"single custom endpoint": {
		customEndpoints: `{"/facility": ".metadata.facility"}`,
		url:             "/facility",
		status:          200,
		json:            tinkerbellDataModel,
	},
	"single custom endpoint, non-metadata": {
		customEndpoints: `{"/id": ".id"}`,
		url:             "/id",
		status:          200,
		json:            tinkerbellDataModel,
	},
	"single custom endpoint, invalid url call": {
		customEndpoints: `{"/userdata": ".metadata.userdata"}`,
		url:             "/metadata",
		status:          404,
		json:            tinkerbellDataModel,
	},
	"multiple custom endpoints": {
		customEndpoints: `{"/metadata":".metadata.instance","/components":".metadata.components","/all":".","/userdata":".metadata.userdata"}`,
		url:             "/components",
		status:          200,
		json:            tinkerbellDataModel,
	},
	"default endpoint": {
		url:    "/metadata",
		status: 200,
		json:   tinkerbellDataModel,
	},
	"custom endpoints invalid format (not a map)": {
		customEndpoints: `"/userdata":".metadata.userdata"`,
		url:             "/userdata",
		status:          404,
		error:           "error in parsing custom endpoints: invalid character ':' after top-level value",
		json:            tinkerbellDataModel,
	},
	"custom endpoints invalid format (endpoint missing forward slash)": {
		customEndpoints: `{"userdata":".metadata.userdata"}`,
		url:             "/userdata",
		status:          404,
		json:            tinkerbellDataModel,
	},
	"custom endpoints invalid format (invalid jq filter syntax)": {
		customEndpoints:     `{"/userdata":"invalid"}`,
		url:                 "/userdata",
		status:              500,
		expectResponseEmpty: true,
		json:                tinkerbellDataModel,
	},
	"custom endpoints invalid format (valid jq filter syntax, nonexistent field)": {
		customEndpoints:     `{"/userdata":".nonexistent"}`,
		url:                 "/userdata",
		status:              200,
		expectResponseEmpty: true,
		json:                tinkerbellDataModel,
	},
}

// test cases for TestEC2Endpoint
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
		json: tinkerbellKantEC2,
	},
	"meta-data": {
		url:    "/2009-04-04/meta-data",
		status: 200,
		response: `facility
hostname
instance-id
iqn
local-ipv4
operating-system
plan
public-ipv4
public-ipv6
public-keys
tags`,
		json: tinkerbellKantEC2,
	},
	"instance-id": {
		url:      "/2009-04-04/meta-data/instance-id",
		status:   200,
		response: "7c9a5711-aadd-4fa0-8e57-789431626a27",
		json:     tinkerbellKantEC2,
	},
	"public-ipv4": {
		url:      "/2009-04-04/meta-data/public-ipv4",
		status:   200,
		response: "139.175.86.114",
		json:     tinkerbellKantEC2,
	},
	"public-ipv6": {
		url:      "/2009-04-04/meta-data/public-ipv6",
		status:   200,
		response: "2604:1380:1000:ca00::7",
		json:     tinkerbellKantEC2,
	},
	"local-ipv4": {
		url:      "/2009-04-04/meta-data/local-ipv4",
		status:   200,
		response: "10.87.63.3",
		json:     tinkerbellKantEC2,
	},
	"tags": {
		url:    "/2009-04-04/meta-data/tags",
		status: 200,
		response: `hello
test`,
		json: tinkerbellKantEC2,
	},
	"operating-system slug": {
		url:      "/2009-04-04/meta-data/operating-system/slug",
		status:   200,
		response: "ubuntu_18_04",
		json:     tinkerbellKantEC2,
	},
	"invalid metadata item": {
		url:      "/2009-04-04/meta-data/invalid",
		status:   404,
		response: "404 not found",
		json:     tinkerbellKantEC2,
	},
	"valid metadata item, but not found": {
		url:      "/2009-04-04/meta-data/public-keys",
		status:   200,
		response: "",
		json:     tinkerbellNoMetadata,
	},
	"with trailing slash": {
		url:      "/2009-04-04/meta-data/hostname/",
		status:   200,
		response: "tink-provisioner",
		json:     tinkerbellKantEC2,
	},
	"base endpoint": {
		url:    "/2009-04-04",
		status: 200,
		response: `meta-data
user-data`,
		json: tinkerbellKantEC2,
	},
	"base endpoint with trailing slash": {
		url:    "/2009-04-04/",
		status: 200,
		response: `meta-data
user-data`,
		json: tinkerbellKantEC2,
	},
	"spot instance with empty (but still present) spot field": {
		url:    "/2009-04-04/meta-data",
		status: 200,
		response: `facility
hostname
instance-id
iqn
local-ipv4
operating-system
plan
public-ipv4
public-ipv6
public-keys
spot
tags`,
		json: tinkerbellKantEC2SpotEmpty,
	},
	"termination-time": {
		url:      "/2009-04-04/meta-data/spot/termination-time",
		status:   200,
		response: "now",
		json:     tinkerbellKantEC2SpotWithTermination,
	},
}

// test cases for TestProcessEC2Query
var processEC2QueryTests = map[string]struct {
	url    string
	error  string
	result string
}{
	"hardware filter result (simple query)": {
		url:    "/2009-04-04/user-data",
		result: ec2Filters["/user-data"],
	},
	"hardware filter result (long query)": {
		url:    "/2009-04-04/meta-data/operating-system/license_activation/state",
		result: ec2Filters["/meta-data/operating-system/license_activation/state"],
	},
	"directory-listing filter result": {
		url:    "/2009-04-04/meta-data/operating-system/license_activation",
		result: ec2Filters["/meta-data/operating-system/license_activation"],
	},
	"directory-listing filter result (base endpoint)": {
		url:    "/2009-04-04/",
		result: ec2Filters[""],
	},
	"directory-listing result (/meta-data endpoint)": {
		url:    "/2009-04-04/meta-data",
		result: ec2Filters["/meta-data"],
	},
	"invalid query (invalid metadata item)": {
		url:   "/2009-04-04/invalid",
		error: "invalid metadata item",
	},
	"invalid query (not a subdirectory)": {
		url:   "/2009-04-04/user-data/hostname",
		error: "invalid metadata item",
	},
}
