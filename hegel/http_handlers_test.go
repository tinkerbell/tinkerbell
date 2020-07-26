package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tinkerbell/tink/protos/packet"
)

func TestGetMetadataCacher(t *testing.T) {
	for name, test := range cacherTests {
		t.Log(name)
		hegelServer.hardwareClient = hardwareGetterMock{test.json}

		os.Setenv("DATA_MODEL_VERSION", "")
		os.Unsetenv("CUSTOM_ENDPOINTS")

		req, err := http.NewRequest("GET", "/metadata", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.RemoteAddr = test.remote
		resp := httptest.NewRecorder()
		http.HandleFunc("/metadata", filterMetadata("")) // filter not used in cacher mode

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
	}
}

// TestGetMetadataTinkerbell tests the default use case in tinkerbell mode
func TestGetMetadataTinkerbell(t *testing.T) {
	os.Setenv("DATA_MODEL_VERSION", "1")
	os.Unsetenv("CUSTOM_ENDPOINTS")

	for name, test := range tinkerbellTests {
		t.Log(name)
		hegelServer.hardwareClient = hardwareGetterMock{test.json}

		http.DefaultServeMux = &http.ServeMux{} // reset registered patterns
		err := registerCustomEndpoints()
		if err != nil {
			t.Fatal("Error registering custom endpoints", err)
		}

		req, err := http.NewRequest("GET", "/metadata", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.RemoteAddr = test.remote
		resp := httptest.NewRecorder()

		http.DefaultServeMux.ServeHTTP(resp, req)

		if status := resp.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v",
				status, http.StatusOK)
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
	}
}

func TestGetMetadataTinkerbellKant(t *testing.T) {
	os.Setenv("DATA_MODEL_VERSION", "1")
	os.Setenv("CUSTOM_ENDPOINTS", `{"/metadata":".metadata.instance","/components":".metadata.components","/userdata":".metadata.userdata"}`)

	for name, test := range tinkerbellKantTests {
		t.Log(name)
		hegelServer.hardwareClient = hardwareGetterMock{test.json}

		http.DefaultServeMux = &http.ServeMux{} // reset registered patterns
		err := registerCustomEndpoints()
		if err != nil {
			t.Fatal("Error registering custom endpoints", err)
		}

		req, err := http.NewRequest("GET", test.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.RemoteAddr = test.remote
		resp := httptest.NewRecorder()

		http.DefaultServeMux.ServeHTTP(resp, req)

		if status := resp.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v",
				status, http.StatusOK)
		}

		if resp.Body.String() != test.response {
			t.Errorf("handler returned unexpected bonding mode: got %v want %v",
				resp.Body.String(), test.response)
		}
	}
}

func TestRegisterEndpoints(t *testing.T) {
	os.Setenv("DATA_MODEL_VERSION", "1")

	for name, test := range registerEndpointTests {
		t.Log(name)
		hegelServer.hardwareClient = hardwareGetterMock{test.json}

		os.Unsetenv("CUSTOM_ENDPOINTS")
		if test.customEndpoints != "" {
			os.Setenv("CUSTOM_ENDPOINTS", test.customEndpoints)
		}

		http.DefaultServeMux = &http.ServeMux{} // reset registered patterns
		err := registerCustomEndpoints()
		if err != nil {
			t.Fatal("Error registering custom endpoints", err)
		}

		req, err := http.NewRequest("GET", test.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.RemoteAddr = test.remote
		resp := httptest.NewRecorder()

		http.DefaultServeMux.ServeHTTP(resp, req)

		if status := resp.Code; status != test.status {
			t.Errorf("handler returned wrong status code: got %v want %v",
				status, test.status)
		}

		t.Log("response:", resp.Body.String()) // logging response instead of explicitly checking content
		if resp.Body.String() == "" && !test.expectResponseEmpty {
			t.Errorf("handler should have returned a non-empty response")
		}
	}
}

var cacherTests = map[string]struct {
	id       string
	remote   string
	planSlug string
	json     string
}{
	"cacher": {
		id:       "8978e7d4-1a55-4845-8a66-a5259236b104",
		remote:   "192.168.1.5",
		planSlug: "t1.small.x86",
		json:     cacherDataModel,
	},
}

var tinkerbellTests = map[string]struct {
	id          string
	remote      string
	bondingMode int64
	json        string
}{
	"tinkerbell": {
		id:          "fde7c87c-d154-447e-9fce-7eb7bdec90c0",
		remote:      "192.168.1.5",
		bondingMode: 5,
		json:        tinkerbellDataModel,
	},
	"tinkerbell no metadata": {
		id:     "363115b0-f03d-4ce5-9a15-5514193d131a",
		remote: "192.168.1.5",
		json:   tinkerbellNoMetadata,
	},
}

var tinkerbellKantTests = map[string]struct {
	url      string
	remote   string
	status   int
	response string
	json     string
}{
	"metadata endpoint": {
		url:      "/metadata",
		remote:   "192.168.1.5",
		status:   200,
		response: `{"facility":"sjc1","hostname":"tink-provisioner","id":"f955e31a-cab6-44d6-872c-9614c2024bb4"}`,
		json:     tinkerbellKant,
	},
	"components endpoint": {
		url:      "/components",
		remote:   "192.168.1.5",
		status:   200,
		response: `{"id":"bc9ce39b-7f18-425b-bc7b-067914fa9786","type":"DiskComponent"}`,
		json:     tinkerbellKant,
	},
	"userdata endpoint": {
		url:      "/userdata",
		remote:   "192.168.1.5",
		status:   200,
		response: `"#!/bin/bash\n\necho \"Hello world!\""`,
		json:     tinkerbellKant,
	},
	"no metadata": {
		url:      "/metadata",
		remote:   "192.168.1.5",
		status:   200,
		response: "null",
		json:     tinkerbellNoMetadata,
	},
}

var registerEndpointTests = map[string]struct {
	customEndpoints     string
	url                 string
	remote              string
	status              int
	expectResponseEmpty bool
	json                string
}{
	"single custom endpoint": {
		customEndpoints: `{"/facility": ".metadata.facility"}`,
		url:             "/facility",
		remote:          "192.168.1.5",
		status:          200,
		json:            tinkerbellDataModel,
	},
	"single custom endpoint, invalid url call": {
		customEndpoints: `{"/userdata": ".metadata.userdata"}`,
		url:             "/metadata",
		remote:          "192.168.1.5",
		status:          404,
		json:            tinkerbellDataModel,
	},
	"multiple custom endpoints": {
		customEndpoints: `{"/metadata":".metadata.instance","/components":".metadata.components","/all":".","/userdata":".metadata.userdata"}`,
		url:             "/components",
		remote:          "192.168.1.5",
		status:          200,
		json:            tinkerbellDataModel,
	},
	"default endpoint": {
		url:    "/metadata",
		remote: "192.168.1.5",
		status: 200,
		json:   tinkerbellDataModel,
	},
	"custom endpoints invalid format (not a map)": {
		customEndpoints: `"/userdata":".metadata.userdata"`,
		url:             "/userdata",
		remote:          "192.168.1.5",
		status:          404,
		json:            tinkerbellDataModel,
	},
	"custom endpoints invalid format (endpoint missing forward slash)": {
		customEndpoints: `{"userdata":".metadata.userdata"}`,
		url:             "/userdata",
		remote:          "192.168.1.5",
		status:          404,
		json:            tinkerbellDataModel,
	},
	"custom endpoints invalid format (invalid jq filter)": {
		customEndpoints:     `{"/userdata":"invalid"}`,
		url:                 "/userdata",
		remote:              "192.168.1.5",
		status:              200,
		expectResponseEmpty: true,
		json:                tinkerbellDataModel,
	},
}
