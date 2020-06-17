package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/packethost/cacher/protos/cacher"
	"google.golang.org/grpc"
)

type hardwareGetterMock struct {
	hardwareResp string
}

func (hg hardwareGetterMock) ByIP(ctx context.Context, in getRequest, opts ...grpc.CallOption) (hardware, error) {
	var hw hardware
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	switch dataModelVersion {
	case "1":
		err := json.Unmarshal([]byte(hg.hardwareResp), &hw)
		if err != nil {
			return nil, err
		}
	default:
		hw = cacher.Hardware{JSON: hg.hardwareResp}
	}

	return hw, nil
}

func (hg hardwareGetterMock) Watch(ctx context.Context, in getRequest, opts ...grpc.CallOption) (watchClient, error) {
	// TODO (kdeng3849)
	return nil, nil
}

func TestGetByIPCacher(t *testing.T) {
	t.Log("DATA_MODEL_VERSION (empty to use cacher):", os.Getenv("DATA_MODEL_VERSION"))

	for name, test := range cacherGrpcTests {
		t.Log(name)

		var hgm hardwareGetter = hardwareGetterMock{test.json}
		hegelTestServer := &server{
			log:            logger,
			hardwareClient: hgm,
		}
		ehw, err := getByIP(context.Background(), hegelTestServer, test.remote)
		if err != nil {
			if err.Error() != test.error {
				t.Fatalf("unexpected error in getByIP, want: %v, got: %v\n", test.error, err.Error())
			}
			return
		}
		hw := exportedHardwareCacher{}
		err = json.Unmarshal(ehw, &hw)
		if err != nil {
			t.Fatal("Error in unmarshalling hardware", err)
		}

		if hw.State != test.state {
			t.Fatalf("unexpected state, want: %v, got: %v\n", test.state, hw.State)
		}
		if hw.Facility != test.facility {
			t.Fatalf("unexpected facility, want: %v, got: %v\n", test.facility, hw.Facility)
		}
		if len(hw.NetworkPorts) > 0 && hw.NetworkPorts[0]["data"].(map[string]interface{})["mac"] != test.mac {
			t.Fatalf("unexpected mac, want: %v, got: %v\n", test.mac, hw.NetworkPorts[0]["data"].(map[string]interface{})["mac"])
		}
		if len(hw.Instance.Storage.Disks) > 0 {
			if hw.Instance.Storage.Disks[0].Device != test.diskDevice {
				t.Fatalf("unexpected storage disk device, want: %v, got: %v\n", test.diskDevice, hw.Instance.Storage.Disks[0].Device)
			}
			if hw.Instance.Storage.Disks[0].WipeTable != test.wipeTable {
				t.Fatalf("unexpected storage disk wipe table, want: %v, got: %v\n", test.wipeTable, hw.Instance.Storage.Disks[0].WipeTable)
			}
			if int(hw.Instance.Storage.Disks[0].Paritions[0].Size) != test.partitionSize {
				t.Fatalf("unexpected storage disk partition size, want: %v, got: %v\n", test.partitionSize, hw.Instance.Storage.Disks[0].Paritions[0].Size)
			}
		}
		if len(hw.Instance.Storage.Filesystems) > 0 {
			if hw.Instance.Storage.Filesystems[0].Mount.Device != test.filesystemDevice {
				t.Fatalf("unexpected storage filesystem mount device, want: %v, got: %v\n", test.filesystemDevice, hw.Instance.Storage.Filesystems[0].Mount.Device)
			}
			if hw.Instance.Storage.Filesystems[0].Mount.Format != test.filesystemFormat {
				t.Fatalf("unexpected storage filesystem mount format, want: %v, got: %v\n", test.filesystemFormat, hw.Instance.Storage.Filesystems[0].Mount.Format)
			}
		}
		if hw.Instance.OS.OsSlug != test.osSlug {
			t.Fatalf("unexpected os slug, want: %v, got: %v\n", test.osSlug, hw.Instance.OS.OsSlug)
		}
		if hw.PlanSlug != test.planSlug {
			t.Fatalf("unexpected plan slug, want: %v, got: %v\n", test.planSlug, hw.PlanSlug)
		}
	}
}

func TestGetByIPTinkerbell(t *testing.T) {
	os.Setenv("DATA_MODEL_VERSION", "1")
	t.Log("DATA_MODEL_VERSION:", os.Getenv("DATA_MODEL_VERSION"))

	for name, test := range tinkerbellGrpcTests {
		t.Log(name)

		var hgm hardwareGetter = hardwareGetterMock{test.json}
		hegelTestServer := &server{
			log:            logger,
			hardwareClient: hgm,
		}
		ehw, err := getByIP(context.Background(), hegelTestServer, test.remote)
		if err != nil {
			if err.Error() != test.error {
				t.Fatalf("unexpected error in getByIP, want: %v, got: %v\n", test.error, err.Error())
			}
			return
		}
		hw := exportedHardwareTinkerbell{}
		err = json.Unmarshal(ehw, &hw)
		if err != nil {
			t.Fatal("Error in unmarshalling hardware", err)
		}
		if hw.Metadata.State != test.state {
			t.Fatalf("unexpected state, want: %v, got: %v\n", test.state, hw.Metadata.State)
		}
		if hw.Metadata.BondingMode != test.bondingMode {
			t.Fatalf("unexpected bonding mode, want: %v, got: %v\n", test.bondingMode, hw.Metadata.BondingMode)
		}
		if len(hw.Metadata.Instance.Storage.Disks) > 0 {
			if hw.Metadata.Instance.Storage.Disks[0].Device != test.diskDevice {
				t.Fatalf("unexpected disk device, want: %v, got: %v\n", test.diskDevice, hw.Metadata.Instance.Storage.Disks[0].Device)
			}
			if hw.Metadata.Instance.Storage.Disks[0].WipeTable != test.wipeTable {
				t.Fatalf("unexpected wipe table, want: %v, got: %v\n", test.wipeTable, hw.Metadata.Instance.Storage.Disks[0].WipeTable)
			}
			if int(hw.Metadata.Instance.Storage.Disks[0].Paritions[0].Size) != test.partitionSize {
				t.Fatalf("unexpected partition size, want: %v, got: %v\n", test.partitionSize, hw.Metadata.Instance.Storage.Disks[0].Paritions[0].Size)
			}
		}
		if len(hw.Metadata.Instance.Storage.Filesystems) > 0 {
			if hw.Metadata.Instance.Storage.Filesystems[0].Mount.Device != test.filesystemDevice {
				t.Fatalf("unexpected filesystem mount device, want: %v, got: %v\n", test.filesystemDevice, hw.Metadata.Instance.Storage.Filesystems[0].Mount.Device)
			}
			if hw.Metadata.Instance.Storage.Filesystems[0].Mount.Format != test.filesystemFormat {
				t.Fatalf("unexpected filesystem mount format, want: %v, got: %v\n", test.filesystemFormat, hw.Metadata.Instance.Storage.Filesystems[0].Mount.Format)
			}
		}
		if hw.Metadata.Instance.OS.OsSlug != test.osSlug {
			t.Fatalf("unexpected os slug, want: %v, got: %v\n", test.osSlug, hw.Metadata.Instance.OS.OsSlug)
		}
		if hw.Metadata.Facility.(map[string]interface{})["plan_slug"] != test.planSlug {
			t.Fatalf("unexpected os slug, want: %v, got: %v\n", test.planSlug, hw.Metadata.Facility.(map[string]interface{})["plan_slug"])
		}
	}
}

var cacherGrpcTests = map[string]struct {
	remote           string
	state            string
	facility         string
	mac              string
	diskDevice       string
	wipeTable        bool
	partitionSize    int
	filesystemDevice string
	filesystemFormat string
	osSlug           string
	planSlug         string
	json             string
	error            string
}{
	"cacher": {
		remote:           "192.168.1.5",
		state:            "provisioning",
		facility:         "onprem",
		mac:              "98:03:9b:48:de:bc",
		diskDevice:       "/dev/sda",
		wipeTable:        true,
		partitionSize:    4096,
		filesystemDevice: "/dev/sda3",
		filesystemFormat: "ext4",
		osSlug:           "ubuntu_16_04",
		planSlug:         "t1.small.x86",
		json:             cacherDataModel,
	},
	"cacher_partition_size_int": {
		partitionSize: 4096,
		json:          cacherPartitionSizeInt,
	},
	"cacher_partition_size_string": {
		partitionSize: 3333,
		json:          cacherPartitionSizeString,
	},
	"cacher_partition_size_whitespace": {
		partitionSize: 1234,
		json:          cacherPartitionSizeWhitespace,
	},
	"cacher_partition_size_k": {
		partitionSize: 24576,
		json:          cacherPartitionSizeK,
	},
	"cacher_partition_size_kb": {
		partitionSize: 25600,
		json:          cacherPartitionSizeKB,
	},
	"cacher_partition_size_m": {
		partitionSize: 3145728,
		json:          cacherPartitionSizeM,
	},
	"cacher_partition_size_invalid_suffix": {
		partitionSize: -1,
		json:          cacherPartitionSizeInvalidSuffix,
		error:         "invalid suffix",
	},
	"cacher_partition_size_invalid_intertwined": {
		json:  cacherPartitionSizeInvalidIntertwined,
		error: `strconv.Atoi: parsing "12kb3": invalid syntax`,
	},
}

var tinkerbellGrpcTests = map[string]struct {
	remote           string
	state            string
	bondingMode      int
	diskDevice       string
	wipeTable        bool
	partitionSize    int
	filesystemDevice string
	filesystemFormat string
	osSlug           string
	planSlug         string
	json             string
	error            string
}{
	"tinkerbell": {
		remote:           "192.168.1.5",
		bondingMode:      5,
		diskDevice:       "/dev/sda",
		wipeTable:        true,
		partitionSize:    4096,
		filesystemDevice: "/dev/sda3",
		filesystemFormat: "ext4",
		osSlug:           "ubuntu_18_04",
		planSlug:         "c2.medium.x86",
		json:             tinkerbellDataModel,
	},
}
