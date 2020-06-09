package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/packethost/cacher/protos/cacher"
	"google.golang.org/grpc"
)

type hardwareGetterMock struct{}

func (hg hardwareGetterMock) ByIP(ctx context.Context, in getRequest, opts ...grpc.CallOption) (hardware, error) {
	var hw hardware
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	switch dataModelVersion {
	case "1":
		err := json.Unmarshal([]byte(tinkerbellDataModel), &hw)
		if err != nil {
			return nil, err
		}
	default:
		hw = cacher.Hardware{JSON: cacherDataModel}
	}

	return hw, nil
}

func (hg hardwareGetterMock) Watch(ctx context.Context, in getRequest, opts ...grpc.CallOption) (watchClient, error) {
	return nil, nil
}

func TestGetByIPCacher(t *testing.T) {
	t.Log("DATA_MODEL_VERSION (empty to use cacher):", os.Getenv("DATA_MODEL_VERSION"))

	var hgm hardwareGetter = hardwareGetterMock{}
	hegelTestServer := &server{
		log:            logger,
		hardwareClient: hgm,
	}
	for name, test := range cacherGrpcTests {
		t.Log(name)
		ehw, err := getByIP(context.Background(), hegelTestServer, test.userIP)
		if err != nil {
			t.Fatal("Error in Finding Hardware", err)
		}
		hw := exportedHardwareCacher{}
		err = json.Unmarshal(ehw, &hw)
		if err != nil {
			// todo
		}
		if hw.PlanSlug != test.planSlug {
			t.Fatalf("unexpected plan slug, want: %v, got: %v\n", test.planSlug, hw.PlanSlug)
		}
	}
}

func TestGetByIPTinkerbell(t *testing.T) {
	os.Setenv("DATA_MODEL_VERSION", "1")
	t.Log("DATA_MODEL_VERSION:", os.Getenv("DATA_MODEL_VERSION"))

	var hgm hardwareGetter = hardwareGetterMock{}
	hegelTestServer := &server{
		log:            logger,
		hardwareClient: hgm,
	}

	for name, test := range tinkerbellTests {
		t.Log(name)
		ehw, err := getByIP(context.Background(), hegelTestServer, test.remote)
		if err != nil {
			t.Fatal("Error in Finding Hardware", err)
		}
		hw := exportedHardwareTinkerbell{}
		err = json.Unmarshal(ehw, &hw)
		if err != nil {
			t.Fatal("Error in unmarshalling hardware", err)
		}
		if hw.Metadata.BondingMode != test.bondingMode {
			t.Fatalf("unexpected primary data mac, want: %v, got: %v\n", test.bondingMode, hw.Metadata.BondingMode)
		}
	}
}

var tinkerbellGrpcTests = map[string]struct {
	userIP      string
	bondingMode int
	json        string
}{
	"tinkerbell": {
		userIP:      "192.168.1.5",
		bondingMode: 5,
		json:        tinkerbellDataModel,
	},
}

var cacherGrpcTests = map[string]struct {
	userIP   string
	planSlug string
	json     string
}{
	"cacher": {
		userIP:   "192.168.1.5",
		planSlug: "t1.small.x86",
		json:     cacherDataModel,
	},
}
