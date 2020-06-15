package main

import (
	"context"
	"os"
)

var fetcher dataFetch

const (
	cacherDataModel = `
	{
	  "id": "8978e7d4-1a55-4845-8a66-a5259236b104",
	  "arch": "x86_64",
	  "name": "node-name",
	  "state": "provisioning",
	  "allow_pxe": true,
	  "allow_workflow": true,
	  "plan_slug": "t1.small.x86",
	  "facility_code": "onprem",
      "efi_boot": false,
	  "instance": {
		"storage": {
		  "disks": [
			{
			  "device": "/dev/sda",
			  "wipeTable": true,
			  "partitions": [
				{
				  "size": 4096,
				  "label": "BIOS",
				  "number": 1
				},
				{
				  "size": "3993600",
				  "label": "SWAP",
				  "number": 2
				},
				{
				  "size": 0,
				  "label": "ROOT",
				  "number": 3
				}
			  ]
			}
		  ],
		  "filesystems": [
			{
			  "mount": {
				"point": "/",
				"create": {
				  "options": ["-L", "ROOT"]
				},
				"device": "/dev/sda3",
				"format": "ext4"
			  }
			},
			{
			  "mount": {
				"point": "none",
				"create": {
				  "options": ["-L", "SWAP"]
				},
				"device": "/dev/sda2",
				"format": "swap"
			  }
			}
		  ]
		},
		"crypted_root_password": "$6$qViImWbWFfH/a4pq$s1bpFFXMpQj1eQbHWsruLy6/",
		"operating_system_version": {
		  "distro": "ubuntu",
		  "version": "16.04",
		  "os_slug": "ubuntu_16_04"
		}
	  },
	  "ip_addresses": [
		{
		  "cidr": 29,
		  "public": false,
		  "address": "192.168.1.5",
		  "enabled": true,
		  "gateway": "192.168.1.1",
		  "netmask": "255.255.255.248",
		  "network": "192.168.1.0",
		  "address_family": 4
		}
	  ],
	  "network_ports": [
		{
		  "data": {
			"mac": "98:03:9b:48:de:bc"
		  },
		  "name": "eth0",
		  "type": "data"
		}
	  ]
	}
`
	cacherPartitionSizeInt = `
    {
	  "id": "8978e7d4-1a55-4845-8a66-a5259236b104",
	  "instance": {
		"storage": {
		  "disks": [
			{
			  "partitions": [
				{
				  "size": 4096,
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
	cacherPartitionSizeString = `
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
	cacherPartitionSizeWhitespace = `
    {
	  "id": "8978e7d4-1a55-4845-8a66-a5259236b104",
	  "instance": {
		"storage": {
		  "disks": [
			{
			  "partitions": [
				{
				  "size": "  1234   ",
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
	cacherPartitionSizeK = `
    {
	  "id": "8978e7d4-1a55-4845-8a66-a5259236b104",
	  "instance": {
		"storage": {
		  "disks": [
			{
			  "partitions": [
				{
				  "size": "24K",
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
	cacherPartitionSizeKB = `
    {
	  "id": "8978e7d4-1a55-4845-8a66-a5259236b104",
	  "instance": {
		"storage": {
		  "disks": [
			{
			  "partitions": [
				{
				  "size": "24Kb",
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
	cacherPartitionSizeM = `
    {
	  "id": "8978e7d4-1a55-4845-8a66-a5259236b104",
	  "instance": {
		"storage": {
		  "disks": [
			{
			  "partitions": [
				{
				  "size": "3m",
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
	tinkerbellDataModel = `
	{
	  "id":"fde7c87c-d154-447e-9fce-7eb7bdec90c0",
	  "network":{
		 "interfaces":[
			{
			   "dhcp":{
				  "mac":"ec:0d:9a:c0:01:0c",
				  "ip":{
					 "address":"192.168.1.5",
					 "netmask":"255.255.255.248",
					 "gateway":"192.168.1.1"
				  },
				  "hostname":"server001",
				  "lease_time":86400,
				  "name_servers": [],
				  "time_servers": [],
				  "arch":"x86_64",
				  "uefi":false
			   },
			   "netboot":{
				  "allow_pxe":true,
				  "allow_workflow":true,
				  "ipxe":{
					 "url":"http://url/menu.ipxe",
					 "contents":"#!ipxe"
				  },
				  "osie":{
					 "kernel":"vmlinuz-x86_64",
					 "initrd":"",
					 "base_url":""
				  }
			   }
			}
		 ]
	  },
	  "metadata":{
		 "state":"",
		 "bonding_mode":5,
		 "manufacturer":{
			"id":"",
			"slug":""
		 },
		 "instance":{
			"storage": {
			  "disks": [
				{
				  "device": "/dev/sda",
				  "wipeTable": true,
				  "partitions": [
					{
					  "size": 4096,
					  "label": "BIOS",
					  "number": 1
					},
					{
					  "size": 3993600,
					  "label": "SWAP",
					  "number": 2
					},
					{
					  "size": 0,
					  "label": "ROOT",
					  "number": 3
					}
				  ]
				}
			  ],
			  "filesystems": [
				{
				  "mount": {
					"point": "/",
					"create": {
					  "options": ["-L", "ROOT"]
					},
					"device": "/dev/sda3",
					"format": "ext4"
				  }
				},
				{
				  "mount": {
					"point": "none",
					"create": {
					  "options": ["-L", "SWAP"]
					},
					"device": "/dev/sda2",
					"format": "swap"
				  }
				}
			  ]
			},
			"crypted_root_password": "$6$qViImWbWFfH/a4pq$s1bpFFXMpQj1eQbHWsruLy6/",
			"operating_system_version": {
			  "distro": "ubuntu",
			  "version": "16.04",
			  "os_slug": "ubuntu_16_04"
			}
         },
		 "custom":{
			"preinstalled_operating_system_version":{},
			"private_subnets":[]
		 },
		 "facility":{
			"plan_slug":"c2.medium.x86",
			"plan_version_slug":"",
			"facility_code":"ewr1"
		 }
	  }
	}
`
)

type dataFetch interface {
	GetByIP(ctx context.Context, s *server, userIP string) ([]byte, error)
}

type dataFetcher struct{}
type dataFetcherMock struct{}

func (d dataFetcher) GetByIP(ctx context.Context, s *server, userIP string) ([]byte, error) {
	return getByIP(ctx, s, userIP)
}

func (d dataFetcherMock) GetByIP(ctx context.Context, s *server, userIP string) ([]byte, error) {
	var hw []byte
	dataModelVersion := os.Getenv("DATA_MODEL_VERSION")
	switch dataModelVersion {
	case "1":
		hw = []byte(tinkerbellDataModel)
	default:
		hw = []byte(cacherDataModel)
	}

	return exportHardware(hw)
}

func init() {
	fetcher = dataFetcher{}
}
