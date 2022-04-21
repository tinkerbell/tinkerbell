package grpcserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"sync"
	"testing"

	"github.com/packethost/pkg/log"
	"github.com/tinkerbell/hegel/hardware"
	"github.com/tinkerbell/hegel/hardware/mock"
	"github.com/tinkerbell/tink/protos/packet"
	"google.golang.org/grpc/peer"
)

func TestGetCacher(t *testing.T) {
	for name, test := range cacherGrpcTests {
		t.Log(name)

		l, err := log.Init("github.com/tinkerbell/hegel")
		if err != nil {
			panic(err)
		}
		logger := l.Package("grpcserver")

		hegelTestServer := NewServer(logger, mock.HardwareClient{Data: test.json})

		addr, err := net.ResolveTCPAddr("tcp", mock.UserIP+":80")
		if err != nil {
			t.Fatal(err, "failed to resolve tcp addr")
		}
		p := &peer.Peer{Addr: addr}
		ctx := peer.NewContext(context.Background(), p)

		ehw, err := hegelTestServer.Get(ctx, nil)
		if err != nil {
			t.Fatal("unexpected error while getting hardware by ip:", err)
		}
		t.Log(ehw.JSON)

		hw := hardware.ExportedCacher{}
		err = json.Unmarshal([]byte(ehw.JSON), &hw)
		if test.error != "" {
			if err == nil {
				t.Fatalf("unmarshal should have returned error: %v", test.error)
			} else if err.Error() != test.error {
				t.Fatalf("unmarshal returned wrong error, want: %v, got: %v\n", err, test.error)
			}
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
			t.Log("want:", test.partitionSize, " got:", hw.Instance.Storage.Disks[0].Paritions[0].Size)
			if fmt.Sprintf("%v", hw.Instance.Storage.Disks[0].Paritions[0].Size) != fmt.Sprintf("%v", test.partitionSize) {
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
		if test.osSlug != "" && hw.Instance.OS.OsSlug != test.osSlug {
			t.Fatalf("unexpected os slug, want: %v, got: %v\n", test.osSlug, hw.Instance.OS.OsSlug)
		}
		if hw.PlanSlug != test.planSlug {
			t.Fatalf("unexpected plan slug, want: %v, got: %v\n", test.planSlug, hw.PlanSlug)
		}
	}
}

func TestGetByIPTinkerbell(t *testing.T) {
	for name, test := range tinkerbellGrpcTests {
		t.Log(name)

		l, err := log.Init("github.com/tinkerbell/hegel")
		if err != nil {
			panic(err)
		}
		logger := l.Package("grpcserver")

		hegelTestServer := NewServer(logger, mock.HardwareClient{Data: test.json})

		addr, err := net.ResolveTCPAddr("tcp", mock.UserIP+":80")
		if err != nil {
			t.Fatal(err, "failed to resolve tcp addr")
		}
		p := &peer.Peer{Addr: addr}
		ctx := peer.NewContext(context.Background(), p)

		ehw, err := hegelTestServer.Get(ctx, nil)
		if err != nil {
			t.Fatal("unexpected error while getting hardware by ip:", err)
		}
		t.Log(ehw.JSON)

		hw := struct {
			ID       string          `json:"id"`
			Metadata packet.Metadata `json:"metadata"`
		}{}
		err = json.Unmarshal([]byte(ehw.JSON), &hw)
		if err != nil {
			t.Error("error in unmarshalling hardware metadata", err)
		}

		if hw.ID != test.id {
			t.Errorf("handler returned unexpected id: got %v want %v",
				hw.ID, test.id)
		}

		if reflect.DeepEqual(&hw.Metadata, &packet.Metadata{}) { // continue if metadata is empty
			continue
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
			if fmt.Sprintf("%v", hw.Metadata.Instance.Storage.Disks[0].Partitions[0].Size) != fmt.Sprintf("%v", test.partitionSize) {
				t.Fatalf("unexpected partition size, want: %v, got: %v\n", test.partitionSize, hw.Metadata.Instance.Storage.Disks[0].Partitions[0].Size)
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
		if hw.Metadata.Instance.OperatingSystem != nil && hw.Metadata.Instance.OperatingSystem.Slug != test.osSlug {
			t.Fatalf("unexpected os slug, want: %v, got: %v\n", test.osSlug, hw.Metadata.Instance.OperatingSystem.Slug)
		}
		if hw.Metadata.Facility.PlanSlug != test.planSlug {
			t.Fatalf("unexpected os slug, want: %v, got: %v\n", test.planSlug, hw.Metadata.Facility.PlanSlug)
		}
	}
}

// test cases for TestGetByIPCacher.
var cacherGrpcTests = map[string]struct {
	id               string
	state            string
	facility         string
	mac              string
	diskDevice       string
	wipeTable        bool
	partitionSize    interface{}
	filesystemDevice string
	filesystemFormat string
	osSlug           string
	planSlug         string
	json             string
	error            string
}{
	"cacher": {
		id:               "",
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
		json:             mock.CacherDataModel,
		error:            "",
	},
	"cacher_partition_size_int": {
		id:               "",
		state:            "",
		facility:         "",
		mac:              "",
		diskDevice:       "",
		wipeTable:        false,
		partitionSize:    4096,
		filesystemDevice: "",
		filesystemFormat: "",
		osSlug:           "",
		planSlug:         "",
		json:             mock.CacherPartitionSizeInt,
		error:            "",
	},
	"cacher_partition_size_string": {
		id:               "",
		state:            "",
		facility:         "",
		mac:              "",
		diskDevice:       "",
		wipeTable:        false,
		partitionSize:    3333,
		filesystemDevice: "",
		filesystemFormat: "",
		osSlug:           "",
		planSlug:         "",
		json:             mock.CacherPartitionSizeString,
		error:            "",
	},
	"cacher_partition_size_b_lower": {
		id:               "",
		state:            "",
		facility:         "",
		mac:              "",
		diskDevice:       "",
		wipeTable:        false,
		partitionSize:    "1000000b",
		filesystemDevice: "",
		filesystemFormat: "",
		osSlug:           "",
		planSlug:         "",
		json:             mock.CacherPartitionSizeBLower,
		error:            "",
	},
}

// test cases for TestGetByIPTinkerbell.
var tinkerbellGrpcTests = map[string]struct {
	id               string
	state            string
	bondingMode      int64
	diskDevice       string
	wipeTable        bool
	partitionSize    interface{}
	filesystemDevice string
	filesystemFormat string
	osSlug           string
	planSlug         string
	json             string
	error            string
}{
	"tinkerbell": {
		id:               "fde7c87c-d154-447e-9fce-7eb7bdec90c0",
		state:            "",
		bondingMode:      5,
		diskDevice:       "/dev/sda",
		wipeTable:        true,
		partitionSize:    4096,
		filesystemDevice: "/dev/sda3",
		filesystemFormat: "ext4",
		osSlug:           "ubuntu_18_04",
		planSlug:         "c2.medium.x86",
		json:             mock.TinkerbellDataModel,
		error:            "",
	},
	"tinkerbell no metadata": {
		id:               "363115b0-f03d-4ce5-9a15-5514193d131a",
		state:            "",
		bondingMode:      0,
		diskDevice:       "",
		wipeTable:        false,
		partitionSize:    nil,
		filesystemDevice: "",
		filesystemFormat: "",
		osSlug:           "",
		planSlug:         "",
		json:             mock.TinkerbellNoMetadata,
		error:            "",
	},
}

func TestServer_SubLock(t *testing.T) {
	var mutex sync.RWMutex
	mutex.Lock()

	tests := []struct {
		name    string
		subLock *sync.RWMutex
	}{
		{
			name:    "test_sublock_lock_unlock",
			subLock: &mutex,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{subLock: tt.subLock}
			retvalLock := s.SubLock()
			if retvalLock == nil {
				t.Errorf("Server.SubLock() lock failed")
			}

			mutex.Unlock()
			retvalUnlock := s.SubLock()
			if retvalUnlock == nil {
				t.Errorf("Server.SubLock() unlock failed")
			}
		})
	}
}

func TestServer_SetHardwareClient(t *testing.T) {
	tests := []struct {
		name           string
		hardwareClient hardware.Client
	}{
		{
			name:           "test_hw_client",
			hardwareClient: mock.HardwareClient{Data: "test_success"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{}
			s.SetHardwareClient(tt.hardwareClient)
			retval := s.HardwareClient()
			if retval == nil {
				t.Error("Server.SetHardwareClient() failed")
			}
		})
	}
}

func TestServer_Subscriptions(t *testing.T) {
	tests := []struct {
		name string
		sub  map[string]*Subscription
	}{
		{
			name: "test_subscription",
			sub:  make(map[string]*Subscription),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{subscriptions: tt.sub}
			retval := s.Subscriptions()
			if retval == nil {
				t.Error("Server.Subscriptions() failed")
			}
		})
	}
}
