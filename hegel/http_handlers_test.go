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

		req, err := http.NewRequest("GET", "/metadata", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.RemoteAddr = test.remote
		resp := httptest.NewRecorder()
		handler := http.HandlerFunc(getMetadata)

		handler.ServeHTTP(resp, req)

		if status := resp.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v",
				status, http.StatusOK)
		}

		hw := exportedHardwareCacher{}
		err = json.Unmarshal(resp.Body.Bytes(), &hw)
		if err != nil {
			t.Error("Error in unmarshalling hardware")
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

func TestGetMetadataTinkerbell(t *testing.T) {
	os.Setenv("DATA_MODEL_VERSION", "1")

	for name, test := range tinkerbellTests {
		t.Log(name)
		hegelServer.hardwareClient = hardwareGetterMock{test.json}

		req, err := http.NewRequest("GET", "/metadata", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.RemoteAddr = test.remote
		resp := httptest.NewRecorder()
		handler := http.HandlerFunc(getMetadata)

		handler.ServeHTTP(resp, req)

		if status := resp.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v",
				status, http.StatusOK)
		}

		hw := exportedHardwareTinkerbell{}
		err = json.Unmarshal(resp.Body.Bytes(), &hw)
		if err != nil {
			t.Error("Error in unmarshalling hardware")
		}

		if hw.ID != test.id {
			t.Errorf("handler returned unexpected id: got %v want %v",
				hw.ID, test.id)
		}

		if hw.Metadata == nil {
			return
		}

		var metadata packet.Metadata
		md, err := json.Marshal(hw.Metadata)
		if err != nil {
			t.Error("Error in marshalling hardware metadata", err)
		}
		err = json.Unmarshal(md, &metadata)
		if err != nil {
			t.Error("Error in unmarshalling hardware metadata", err)
		}

		if metadata.BondingMode != test.bondingMode {
			t.Errorf("handler returned unexpected bonding mode: got %v want %v",
				metadata.BondingMode, test.bondingMode)
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
