package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/packethost/hegel/metrics"
	"github.com/packethost/pkg/log"
)

func TestMain(m *testing.M) {
	fetcher = dataFetcherMock{}

	os.Setenv("PACKET_ENV", "test")
	os.Setenv("PACKET_VERSION", "ignored")
	os.Setenv("ROLLBAR_TOKEN", "ignored")

	l, _ := log.Init("github.com/packethost/hegel")
	logger = l.Package("main")
	metrics.Init(l)

	os.Exit(m.Run())
}

func TestGetMetadataCacher(t *testing.T) {
	for name, test := range cacherTests {
		t.Log(name)

		os.Setenv("HARDWARE_DATA_MODEL", "")

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
	os.Setenv("HARDWARE_DATA_MODEL", "tinkerbell")

	for name, test := range tinkerbellTests {
		t.Log(name)

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

		if hw.ID != test.id {
			t.Errorf("handler returned unexpected id: got %v want %v",
				hw.ID, test.id)
		}
		if hw.Metadata.BondingMode != test.bondingMode {
			t.Errorf("handler returned unexpected bonding mode: got %v want %v",
				hw.Metadata.BondingMode, test.bondingMode)
		}
	}
}

var cacherTests = map[string]struct {
	id       string
	remote   string
	planSlug string
}{
	"cacher": {
		id:       "8978e7d4-1a55-4845-8a66-a5259236b104",
		remote:   "192.168.1.5",
		planSlug: "t1.small.x86",
	},
}

var tinkerbellTests = map[string]struct {
	id          string
	remote      string
	bondingMode int
}{
	"tinkerbell": {
		id:          "fde7c87c-d154-447e-9fce-7eb7bdec90c0",
		remote:      "192.168.1.5",
		bondingMode: 5,
	},
}
