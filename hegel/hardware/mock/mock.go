package mock

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/packethost/cacher/protos/cacher"
	"github.com/tinkerbell/hegel/hardware"
	"google.golang.org/grpc"
)

// HardwareClient is a mock implementation of the hardwareGetter interface
// Data represents the hardware data stored inside tink db.
type HardwareClient struct {
	Data string
}

func (hg HardwareClient) All(_ context.Context, _ ...grpc.CallOption) (hardware.AllClient, error) {
	return nil, nil
}

// ByIP mocks the retrieval a piece of hardware from tink/cacher by ip
// In order to simulate the process of finding the piece of hardware that matches the IP provided in the get request without
// having to parse the (mock) hardware data `HardwareClient.Data`, the process has been simplified to only match with the constant `UserIP`.
// Given any other IP inside the get request, ByIP will return an empty piece of hardware regardless of whether or not the IP
// actually matches the IP inside `Data`.
func (hg HardwareClient) ByIP(_ context.Context, ip string, _ ...grpc.CallOption) (hardware.Hardware, error) {
	var hw hardware.Hardware
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	switch dataModelVersion {
	case "1":
		hw = &hardware.Tinkerbell{}

		if ip != UserIP {
			return hw, errors.New("rpc error: code = Unknown desc = unexpected end of JSON input")
		}

		err := json.Unmarshal([]byte(hg.Data), hw)
		if err != nil {
			return nil, err
		}
	default:
		if ip != UserIP {
			return &hardware.Cacher{}, errors.New("rpc error: code = Unknown desc = DB is not ready")
		}

		hw = &hardware.Cacher{Hardware: &cacher.Hardware{JSON: hg.Data}}
	}

	return hw, nil
}

func (hg HardwareClient) Watch(_ context.Context, _ string, _ ...grpc.CallOption) (hardware.Watcher, error) {
	return nil, nil
}

const (
	UserIP          = "192.168.1.5" // value is completely arbitrary, as long as it's an IP to be parsed by getIPFromRequest (could even be 0.0.0.0)
	CacherDataModel = `
	{
		"allow_pxe": true,
		"allow_workflow": true,
		"arch": "x86_64",
		"efi_boot": false,
		"facility_code": "onprem",
		"id": "8978e7d4-1a55-4845-8a66-a5259236b104",
		"instance": {
		  "crypted_root_password": "$6$qViImWbWFfH/a4pq$s1bpFFXMpQj1eQbHWsruLy6/",
		  "operating_system_version": {
			"distro": "ubuntu",
			"os_slug": "ubuntu_16_04",
			"version": "16.04"
		  },
		  "storage": {
			"disks": [
			  {
				"device": "/dev/sda",
				"partitions": [
				  {
					"label": "BIOS",
					"number": 1,
					"size": 4096
				  },
				  {
					"label": "SWAP",
					"number": 2,
					"size": "3993600"
				  },
				  {
					"label": "ROOT",
					"number": 3,
					"size": 0
				  }
				],
				"wipeTable": true
			  }
			],
			"filesystems": [
			  {
				"mount": {
				  "create": {
					"options": [
					  "-L",
					  "ROOT"
					]
				  },
				  "device": "/dev/sda3",
				  "format": "ext4",
				  "point": "/"
				}
			  },
			  {
				"mount": {
				  "create": {
					"options": [
					  "-L",
					  "SWAP"
					]
				  },
				  "device": "/dev/sda2",
				  "format": "swap",
				  "point": "none"
				}
			  }
			]
		  }
		},
		"ip_addresses": [
		  {
			"address": "192.168.1.5",
			"address_family": 4,
			"cidr": 29,
			"enabled": true,
			"gateway": "192.168.1.1",
			"netmask": "255.255.255.248",
			"network": "192.168.1.0",
			"public": false
		  }
		],
		"name": "node-name",
		"network_ports": [
		  {
			"data": {
			  "mac": "98:03:9b:48:de:bc"
			},
			"name": "eth0",
			"type": "data"
		  }
		],
		"plan_slug": "t1.small.x86",
		"state": "provisioning"
	  }
`
	CacherPartitionSizeInt = `
	{
		"id": "8978e7d4-1a55-4845-8a66-a5259236b104",
		"instance": {
		  "storage": {
			"disks": [
			  {
				"partitions": [
				  {
					"label": "BIOS",
					"number": 1,
					"size": 4096
				  }
				]
			  }
			]
		  }
		}
	  }
`
	CacherPartitionSizeString = `
    {
	  "id": "8978e7d4-1a55-4845-8a66-a5259236b104",
	  "instance": {
		"storage": {
		  "disks": [
			{
			  "partitions": [
				{
				  "size": "3333",
				  "label": "BIOS",
				  "number": 1
				}
			  ]
			}
		  ]
        }
	  }
	}
`
	CacherPartitionSizeBLower = `
    {
	  "id": "8978e7d4-1a55-4845-8a66-a5259236b104",
	  "instance": {
		"storage": {
		  "disks": [
			{
			  "partitions": [
				{
				  "size": "1000000b",
				  "label": "BIOS",
				  "number": 1
				}
			  ]
			}
		  ]
        }
	  }
	}
`
	TinkerbellDataModel = `
	{
		"id": "fde7c87c-d154-447e-9fce-7eb7bdec90c0",
		"metadata": "{\"components\":{\"id\":\"bc9ce39b-7f18-425b-bc7b-067914fa9786\",\"type\":\"DiskComponent\"},\"userdata\":\"#!/bin/bash\\necho \\\"Hello world!\\\"\",\"bonding_mode\":5,\"custom\":{\"preinstalled_operating_system_version\":{},\"private_subnets\":[]},\"facility\":{\"facility_code\":\"ewr1\",\"plan_slug\":\"c2.medium.x86\",\"plan_version_slug\":\"\"},\"instance\":{\"crypted_root_password\":\"redacted/\",\"operating_system\":{\"distro\":\"ubuntu\",\"slug\":\"ubuntu_18_04\",\"version\":\"18.04\"},\"storage\":{\"disks\":[{\"device\":\"/dev/sda\",\"partitions\":[{\"label\":\"BIOS\",\"number\":1,\"size\":4096},{\"label\":\"SWAP\",\"number\":2,\"size\":3993600},{\"label\":\"ROOT\",\"number\":3,\"size\":0}],\"wipe_table\":true}],\"filesystems\":[{\"mount\":{\"create\":{\"options\":[\"-L\",\"ROOT\"]},\"device\":\"/dev/sda3\",\"format\":\"ext4\",\"point\":\"/\"}},{\"mount\":{\"create\":{\"options\":[\"-L\",\"SWAP\"]},\"device\":\"/dev/sda2\",\"format\":\"swap\",\"point\":\"none\"}}]}},\"manufacturer\":{\"id\":\"\",\"slug\":\"\"},\"state\":\"\"}",
		"network": {
		  "interfaces": [
			{
			  "dhcp": {
				"arch": "x86_64",
				"hostname": "server001",
				"ip": {
				  "address": "192.168.1.5",
				  "gateway": "192.168.1.1",
				  "netmask": "255.255.255.248"
				},
				"lease_time": 86400,
				"mac": "ec:0d:9a:c0:01:0c"
			  },
			  "netboot": {
				"allow_pxe": true,
				"allow_workflow": true,
				"ipxe": {
				  "contents": "#!ipxe",
				  "url": "http://url/menu.ipxe"
				},
				"osie": {
				  "kernel": "vmlinuz-x86_64"
				}
			  }
			}
		  ]
		}
	  }
`
	TinkerbellNoMetadata = `
	{
		"id": "363115b0-f03d-4ce5-9a15-5514193d131a",
		"network": {
		  "interfaces": [
			{
			  "dhcp": {
				"arch": "x86_64",
				"hostname": "server001",
				"ip": {
				  "address": "192.168.1.5",
				  "gateway": "192.168.1.1",
				  "netmask": "255.255.255.248"
				},
				"lease_time": 86400,
				"mac": "ec:0d:9a:c0:01:0c"
			  },
			  "netboot": {
				"allow_pxe": true,
				"allow_workflow": true,
				"ipxe": {
				  "contents": "#!ipxe",
				  "url": "http://url/menu.ipxe"
				},
				"osie": {
				  "kernel": "vmlinuz-x86_64"
				}
			  }
			}
		  ]
		}
	  }
`
	TinkerbellKant = `
	{
		"id": "fde7c87c-d154-447e-9fce-7eb7bdec90c0",
		"metadata": "{\"components\":{\"id\":\"bc9ce39b-7f18-425b-bc7b-067914fa9786\",\"type\":\"DiskComponent\"},\"instance\":{\"facility\":\"sjc1\",\"hostname\":\"tink-provisioner\",\"id\":\"f955e31a-cab6-44d6-872c-9614c2024bb4\"},\"userdata\":\"#!/bin/bash\\n\\necho \\\"Hello world!\\\"\"}",
		"network": {
		  "interfaces": [
			{
			  "dhcp": {
				"arch": "x86_64",
				"hostname": "server001",
				"ip": {
				  "address": "192.168.1.5",
				  "gateway": "192.168.1.1",
				  "netmask": "255.255.255.248"
				},
				"lease_time": 86400,
				"mac": "ec:0d:9a:c0:01:0c"
			  },
			  "netboot": {
				"allow_pxe": true,
				"allow_workflow": true,
				"ipxe": {
				  "contents": "#!ipxe",
				  "url": "http://url/menu.ipxe"
				},
				"osie": {
				  "kernel": "vmlinuz-x86_64"
				}
			  }
			}
		  ]
		}
	  }
`
	TinkerbellKantEC2 = `
	{
		"id": "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
		"metadata": "{\"components\":{\"id\":\"bc9ce39b-7f18-425b-bc7b-067914fa9786\",\"type\":\"DiskComponent\"},\"instance\":{\"api_url\":\"https://metadata.packet.net\",\"class\":\"c3.small.x86\",\"customdata\":{},\"facility\":\"sjc1\",\"hostname\":\"tink-provisioner\",\"id\":\"7c9a5711-aadd-4fa0-8e57-789431626a27\",\"iqn\":\"iqn.2020-06.net.packet:device.7c9a5711\",\"network\":{\"addresses\":[{\"address\":\"139.175.86.114\",\"address_family\":4,\"cidr\":31,\"created_at\":\"2020-06-19T04:16:08Z\",\"enabled\":true,\"gateway\":\"139.175.86.113\",\"id\":\"99e15f8e-6eab-40db-9c6f-69a69ef9854f\",\"management\":true,\"netmask\":\"255.255.255.254\",\"network\":\"139.175.86.113\",\"parent_block\":{\"cidr\":31,\"href\":\"/ips/179580b0-3ae4-4fc0-8cbe-4f34174bebb4\",\"netmask\":\"255.255.255.254\",\"network\":\"139.175.86.113\"},\"public\":true},{\"address\":\"2604:1380:1000:ca00::7\",\"address_family\":6,\"cidr\":127,\"created_at\":\"2020-06-19T04:16:08Z\",\"enabled\":true,\"gateway\":\"2604:1380:1000:ca00::6\",\"id\":\"f4b24331-c6cf-4ae4-899b-e78f223b2c57\",\"management\":true,\"netmask\":\"ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe\",\"network\":\"2604:1380:1000:ca00::6\",\"parent_block\":{\"cidr\":56,\"href\":\"/ips/960aa63d-eeb6-410e-8242-1d6e2e7733fc\",\"netmask\":\"ffff:ffff:ffff:ff00:0000:0000:0000:0000\",\"network\":\"2604:1380:1000:ca00:0000:0000:0000:0000\"},\"public\":true},{\"address\":\"10.87.63.3\",\"address_family\":4,\"cidr\":31,\"created_at\":\"2020-06-19T04:16:08Z\",\"enabled\":true,\"gateway\":\"10.87.63.2\",\"id\":\"5cca13a9-43d0-45a6-9ed7-3d9e2fbf0e87\",\"management\":true,\"netmask\":\"255.255.255.254\",\"network\":\"10.87.63.2\",\"parent_block\":{\"cidr\":25,\"href\":\"/ips/7cde0a1b-d787-4a10-9c96-4049c7d5eeb3\",\"netmask\":\"255.255.255.128\",\"network\":\"10.87.63.0\"},\"public\":false}],\"bonding\":{\"link_aggregation\":null,\"mac\":\"b4:96:91:5f:ad:d8\",\"mode\":4},\"interfaces\":[{\"bond\":\"bond0\",\"mac\":\"b4:96:91:5f:ad:d8\",\"name\":\"eth0\"},{\"bond\":\"bond0\",\"mac\":\"b4:96:91:5f:ad:d9\",\"name\":\"eth1\"}]},\"operating_system\":{\"distro\":\"ubuntu\",\"image_tag\":\"f8f0331d31935319dfa8b6d551b5680840d7944f\",\"license_activation\":{\"state\":\"unlicensed\"},\"slug\":\"ubuntu_18_04\",\"version\":\"18.04\"},\"phone_home_url\":\"http://tinkerbell.sjc1.packet.net/phone-home\",\"plan\":\"c3.small.x86\",\"private_subnets\":[\"10.0.0.0/8\"],\"specs\":{\"cpus\":[{\"count\":1,\"type\":\"EPYC 3151 4 Core Processor @ 2.7GHz\"}],\"drives\":[{\"category\":\"boot\",\"count\":2,\"size\":\"240GB\",\"type\":\"SSD\"}],\"features\":{},\"memory\":{\"total\":\"16GB\"},\"nics\":[{\"count\":2,\"type\":\"10Gbps\"}]},\"ssh_keys\":[],\"storage\":{\"disks\":[{\"device\":\"/dev/sda\",\"partitions\":[{\"label\":\"BIOS\",\"number\":1,\"size\":4096},{\"label\":\"SWAP\",\"number\":2,\"size\":\"3993600\"},{\"label\":\"ROOT\",\"number\":3,\"size\":0}],\"wipeTable\":true}],\"filesystems\":[{\"mount\":{\"create\":{\"options\":[\"-L\",\"ROOT\"]},\"device\":\"/dev/sda3\",\"format\":\"ext4\",\"point\":\"/\"}},{\"mount\":{\"create\":{\"options\":[\"-L\",\"SWAP\"]},\"device\":\"/dev/sda2\",\"format\":\"swap\",\"point\":\"none\"}}]},\"switch_short_id\":\"68c7fa13\",\"tags\":[\"hello\",\"test\"],\"user_state_url\":\"http://tinkerbell.sjc1.packet.net/events\",\"volumes\":[]},\"userdata\":\"#!/bin/bash\\n\\necho \\\"Hello world!\\\"\"}",
		"network": {
		  "interfaces": [
			{
			  "dhcp": {
				"arch": "x86_64",
				"ip": {
				  "address": "192.168.1.5",
				  "gateway": "192.168.1.1",
				  "netmask": "255.255.255.248"
				},
				"mac": "b4:96:91:5f:af:c0"
			  },
			  "netboot": {
				"allow_pxe": true,
				"allow_workflow": true
			  }
			}
		  ]
		}
	  }
`
	TinkerbellKantEC2SpotEmpty = `
	{
		"id": "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
		"metadata": "{\"instance\":{\"spot\":{}}}",
		"network": {
		  "interfaces": [
			{
			  "dhcp": {
				"arch": "x86_64",
				"ip": {
				  "address": "192.168.1.5",
				  "gateway": "192.168.1.1",
				  "netmask": "255.255.255.248"
				},
				"mac": "b4:96:91:5f:af:c0"
			  },
			  "netboot": {
				"allow_pxe": true,
				"allow_workflow": true
			  }
			}
		  ]
		}
	  }
`
	TinkerbellKantEC2SpotWithTermination = `
	{
		"id": "0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94",
		"metadata": "{\"instance\":{\"spot\":{\"termination_time\":\"now\"}}}",
		"network": {
		  "interfaces": [
			{
			  "dhcp": {
				"arch": "x86_64",
				"ip": {
				  "address": "192.168.1.5",
				  "gateway": "192.168.1.1",
				  "netmask": "255.255.255.248"
				},
				"mac": "b4:96:91:5f:af:c0"
			  },
			  "netboot": {
				"allow_pxe": true,
				"allow_workflow": true
			  }
			}
		  ]
		}
	  }
`
	// tinkerbellFilterMetadata is used for testing the filterMetadata function and has the 'metadata' field represented as an object (as opposed to string).
	TinkerbellFilterMetadata = `{"id":"0eba0bf8-3772-4b4a-ab9f-6ebe93b90a94","metadata":{"components":{"id":"bc9ce39b-7f18-425b-bc7b-067914fa9786","type":"DiskComponent"},"instance":{"api_url":"https://metadata.packet.net","class":"c3.small.x86","customdata":{},"facility":"sjc1","hostname":"tink-provisioner","id":"7c9a5711-aadd-4fa0-8e57-789431626a27","iqn":"iqn.2020-06.net.packet:device.7c9a5711","network":{"addresses":[{"address":"139.175.86.114","address_family":4,"cidr":31,"created_at":"2020-06-19T04:16:08Z","enabled":true,"gateway":"139.175.86.113","id":"99e15f8e-6eab-40db-9c6f-69a69ef9854f","management":true,"netmask":"255.255.255.254","network":"139.175.86.113","parent_block":{"cidr":31,"href":"/ips/179580b0-3ae4-4fc0-8cbe-4f34174bebb4","netmask":"255.255.255.254","network":"139.175.86.113"},"public":true},{"address":"2604:1380:1000:ca00::7","address_family":6,"cidr":127,"created_at":"2020-06-19T04:16:08Z","enabled":true,"gateway":"2604:1380:1000:ca00::6","id":"f4b24331-c6cf-4ae4-899b-e78f223b2c57","management":true,"netmask":"ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe","network":"2604:1380:1000:ca00::6","parent_block":{"cidr":56,"href":"/ips/960aa63d-eeb6-410e-8242-1d6e2e7733fc","netmask":"ffff:ffff:ffff:ff00:0000:0000:0000:0000","network":"2604:1380:1000:ca00:0000:0000:0000:0000"},"public":true},{"address":"10.87.63.3","address_family":4,"cidr":31,"created_at":"2020-06-19T04:16:08Z","enabled":true,"gateway":"10.87.63.2","id":"5cca13a9-43d0-45a6-9ed7-3d9e2fbf0e87","management":true,"netmask":"255.255.255.254","network":"10.87.63.2","parent_block":{"cidr":25,"href":"/ips/7cde0a1b-d787-4a10-9c96-4049c7d5eeb3","netmask":"255.255.255.128","network":"10.87.63.0"},"public":false}],"bonding":{"link_aggregation":null,"mac":"b4:96:91:5f:ad:d8","mode":4},"interfaces":[{"bond":"bond0","mac":"b4:96:91:5f:ad:d8","name":"eth0"},{"bond":"bond0","mac":"b4:96:91:5f:ad:d9","name":"eth1"}]},"operating_system":{"distro":"ubuntu","image_tag":"f8f0331d31935319dfa8b6d551b5680840d7944f","license_activation":{"state":"unlicensed"},"slug":"ubuntu_18_04","version":"18.04"},"phone_home_url":"http://tinkerbell.sjc1.packet.net/phone-home","plan":"c3.small.x86","private_subnets":["10.0.0.0/8"],"specs":{"cpus":[{"count":1,"type":"EPYC 3151 4 Core Processor @ 2.7GHz"}],"drives":[{"category":"boot","count":2,"size":"240GB","type":"SSD"}],"features":{},"memory":{"total":"16GB"},"nics":[{"count":2,"type":"10Gbps"}]},"spot":{},"ssh_keys":[],"storage":{"disks":[{"device":"/dev/sda","partitions":[{"label":"BIOS","number":1,"size":4096},{"label":"SWAP","number":2,"size":"3993600"},{"label":"ROOT","number":3,"size":0}],"wipeTable":true}],"filesystems":[{"mount":{"create":{"options":["-L","ROOT"]},"device":"/dev/sda3","format":"ext4","point":"/"}},{"mount":{"create":{"options":["-L","SWAP"]},"device":"/dev/sda2","format":"swap","point":"none"}}]},"switch_short_id":"68c7fa13","tags":["hello","test"],"user_state_url":"http://tinkerbell.sjc1.packet.net/events","volumes":[]},"userdata":"#!/bin/bash\n\necho \"Hello world!\""},"network":{"interfaces":[{"dhcp":{"arch":"x86_64","ip":{"address":"192.168.1.5","gateway":"192.168.1.1","netmask":"255.255.255.248"},"mac":"b4:96:91:5f:af:c0"},"netboot":{"allow_pxe":true,"allow_workflow":true}}]}}`
)
