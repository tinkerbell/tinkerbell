package kube

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/data"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

func TestNewBackend(t *testing.T) {
	tests := map[string]struct {
		conf      *rest.Config
		opt       cluster.Option
		idxs      map[IndexType]Index
		shouldErr bool
	}{
		"no config": {
			shouldErr: false,
			opt: func(o *cluster.Options) {
				o.NewClient = func(*rest.Config, client.Options) (client.Client, error) {
					return newWorkingClient(t), nil
				}
				o.MapperProvider = func(*rest.Config, *http.Client) (meta.RESTMapper, error) {
					return newWorkingClient(t).RESTMapper(), nil
				}
				o.NewCache = func(*rest.Config, cache.Options) (cache.Cache, error) {
					return &informertest.FakeInformers{Scheme: newWorkingClient(t).Scheme()}, nil
				}
			},
		},
		"failed index field": {
			shouldErr: true,
			conf:      new(rest.Config),
			opt: func(o *cluster.Options) {
				cl := fake.NewClientBuilder().Build()
				o.NewClient = func(*rest.Config, client.Options) (client.Client, error) {
					return cl, nil
				}
				o.MapperProvider = func(*rest.Config, *http.Client) (meta.RESTMapper, error) {
					return cl.RESTMapper(), nil
				}
			},
			idxs: map[IndexType]Index{
				IndexType("one"):              {Obj: &tinkerbell.Hardware{}, Field: MACAddrIndex, ExtractValue: MACAddrs},
				IndexType("duplicate of one"): {Obj: &tinkerbell.Hardware{}, Field: MACAddrIndex, ExtractValue: MACAddrs},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			b, err := NewBackend(Backend{ClientConfig: tt.conf, APIURL: "localhost", Indexes: tt.idxs}, tt.opt)
			if tt.shouldErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.shouldErr && err != nil {
				t.Fatal(err)
			}
			if !tt.shouldErr && b == nil {
				t.Fatal("expected backend")
			}
		})
	}
}

func TestToDHCPData(t *testing.T) {
	tests := map[string]struct {
		in        *tinkerbell.DHCP
		want      *data.DHCP
		shouldErr bool
	}{
		"nil input": {
			in:        nil,
			shouldErr: true,
		},
		"no mac": {
			in:        &tinkerbell.DHCP{},
			shouldErr: true,
		},
		"bad mac": {
			in:        &tinkerbell.DHCP{MAC: "bad"},
			shouldErr: true,
		},
		"no ip": {
			in:        &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:ff", IP: &tinkerbell.IP{}},
			shouldErr: true,
		},
		"no subnet": {
			in:        &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:ff", IP: &tinkerbell.IP{Address: "192.168.2.4"}},
			shouldErr: true,
		},
		"v1alpha1.IP == nil": {
			in:        &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:ff", IP: nil},
			shouldErr: true,
		},
		"bad gateway": {
			in:        &tinkerbell.DHCP{MAC: "aa:bb:cc:dd:ee:ff", IP: &tinkerbell.IP{Address: "192.168.2.4", Netmask: "255.255.254.0", Gateway: "bad"}},
			shouldErr: true,
		},
		"one bad nameserver": {
			in: &tinkerbell.DHCP{
				MAC:         "00:00:00:00:00:04",
				NameServers: []string{"1.1.1.1", "bad"},
				IP: &tinkerbell.IP{
					Address: "192.168.2.4",
					Netmask: "255.255.0.0",
					Gateway: "192.168.2.1",
				},
			},
			want: &data.DHCP{
				SubnetMask:     net.IPv4Mask(255, 255, 0, 0),
				DefaultGateway: netip.MustParseAddr("192.168.2.1"),
				NameServers:    []net.IP{net.IPv4(1, 1, 1, 1)},
				IPAddress:      netip.MustParseAddr("192.168.2.4"),
				MACAddress:     net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x04},
			},
		},
		"full": {
			in: &tinkerbell.DHCP{
				MAC:         "00:00:00:00:00:04",
				Hostname:    "test",
				LeaseTime:   3600,
				NameServers: []string{"1.1.1.1"},
				IP: &tinkerbell.IP{
					Address: "192.168.1.4",
					Netmask: "255.255.255.0",
					Gateway: "192.168.1.1",
				},
			},
			want: &data.DHCP{
				SubnetMask:     net.IPv4Mask(255, 255, 255, 0),
				DefaultGateway: netip.MustParseAddr("192.168.1.1"),
				NameServers:    []net.IP{net.IPv4(1, 1, 1, 1)},
				Hostname:       "test",
				LeaseTime:      3600,
				IPAddress:      netip.MustParseAddr("192.168.1.4"),
				MACAddress:     net.HardwareAddr{0x00, 0x00, 0x00, 0x00, 0x00, 0x04},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := toDHCPData(tt.in)
			if tt.shouldErr && err == nil {
				t.Fatal("expected error")
			}
			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(netip.Addr{})); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestToNetbootData(t *testing.T) {
	tests := map[string]struct {
		in        *tinkerbell.Netboot
		want      *data.Netboot
		shouldErr bool
	}{
		"nil input":    {in: nil, shouldErr: true},
		"bad ipxe url": {in: &tinkerbell.Netboot{IPXE: &tinkerbell.IPXE{URL: "bad"}}, shouldErr: true},
		"successful":   {in: &tinkerbell.Netboot{IPXE: &tinkerbell.IPXE{URL: "http://example.com/ipxe.ipxe"}}, want: &data.Netboot{IPXEScriptURL: &url.URL{Scheme: "http", Host: "example.com", Path: "/ipxe.ipxe"}}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := toNetbootData(tt.in, "")
			if tt.shouldErr && err == nil {
				t.Fatal("expected error")
			}
			if diff := cmp.Diff(got, tt.want, cmpopts.IgnoreUnexported(netip.Addr{})); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestGetByIP(t *testing.T) {
	tests := map[string]struct {
		hwObject    []tinkerbell.Hardware
		wantDHCP    *data.DHCP
		wantNetboot *data.Netboot
		shouldErr   bool
		failToList  bool
	}{
		"empty hardware list":    {shouldErr: true, hwObject: []tinkerbell.Hardware{}},
		"more than one hardware": {shouldErr: true, hwObject: []tinkerbell.Hardware{hwObject1, hwObject2}},
		"bad dhcp data":          {shouldErr: true, hwObject: []tinkerbell.Hardware{badDHCPObject2}},
		"bad netboot data":       {shouldErr: true, hwObject: []tinkerbell.Hardware{badNetbootObject2}},
		"fail to list hardware":  {shouldErr: true, failToList: true},
		"good data": {hwObject: []tinkerbell.Hardware{hwObject1}, wantDHCP: &data.DHCP{
			MACAddress:     net.HardwareAddr{0x3c, 0xec, 0xef, 0x4c, 0x4f, 0x54},
			IPAddress:      netip.MustParseAddr("172.16.10.100"),
			SubnetMask:     []byte{0xff, 0xff, 0xff, 0x00},
			DefaultGateway: netip.MustParseAddr("255.255.255.0"),
			NameServers: []net.IP{
				{0x1, 0x1, 0x1, 0x1},
			},
			Hostname:  "sm01",
			LeaseTime: 86400,
			Arch:      "x86_64",
		}, wantNetboot: &data.Netboot{
			AllowNetboot: true,
			IPXEScriptURL: &url.URL{
				Scheme: "http",
				Host:   "netboot.xyz",
			},
			Facility: "onprem",
		}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rs := runtime.NewScheme()
			if err := scheme.AddToScheme(rs); err != nil {
				t.Fatal(err)
			}
			if err := tinkerbell.AddToScheme(rs); err != nil {
				t.Fatal(err)
			}

			ct := fake.NewClientBuilder()
			if !tc.failToList {
				ct = ct.WithScheme(rs)
				ct = ct.WithRuntimeObjects(&tinkerbell.HardwareList{})
				ct = ct.WithIndex(&tinkerbell.Hardware{}, IPAddrIndex, func(client.Object) []string {
					var list []string
					for _, elem := range tc.hwObject {
						list = append(list, elem.Spec.Interfaces[0].DHCP.IP.Address)
					}
					return list
				})
			}
			if len(tc.hwObject) > 0 {
				t.Logf("%+v", tc.hwObject[0].Spec.Interfaces[0].DHCP)
				t.Logf("%+v", tc.hwObject[0].Spec.Interfaces[0].DHCP.IP)
				ct = ct.WithLists(&tinkerbell.HardwareList{Items: tc.hwObject})
			}
			cl := ct.Build()

			fn := func(o *cluster.Options) {
				o.NewClient = func(*rest.Config, client.Options) (client.Client, error) {
					return cl, nil
				}
				o.MapperProvider = func(*rest.Config, *http.Client) (meta.RESTMapper, error) {
					return cl.RESTMapper(), nil
				}
				o.NewCache = func(*rest.Config, cache.Options) (cache.Cache, error) {
					return &informertest.FakeInformers{Scheme: cl.Scheme()}, nil
				}
			}
			rc := new(rest.Config)
			b, err := NewBackend(Backend{ClientConfig: rc}, fn)
			if err != nil {
				t.Fatal(err)
			}

			go b.Start(context.Background())
			gotDHCP, gotNetboot, err := b.GetByIP(context.Background(), net.IPv4(172, 16, 10, 100))
			if tc.shouldErr && err == nil {
				t.Log(err)
				t.Fatal("expected error")
			}

			if diff := cmp.Diff(gotDHCP, tc.wantDHCP, cmpopts.IgnoreUnexported(netip.Addr{})); diff != "" {
				t.Fatal(diff)
			}

			if diff := cmp.Diff(gotNetboot, tc.wantNetboot); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestGetByMac(t *testing.T) {
	tests := map[string]struct {
		hwObject    []tinkerbell.Hardware
		wantDHCP    *data.DHCP
		wantNetboot *data.Netboot
		shouldErr   bool
		failToList  bool
	}{
		"empty hardware list":    {shouldErr: true},
		"more than one hardware": {shouldErr: true, hwObject: []tinkerbell.Hardware{hwObject1, hwObject2}},
		"bad dhcp data":          {shouldErr: true, hwObject: []tinkerbell.Hardware{badDHCPObject}},
		"bad netboot data":       {shouldErr: true, hwObject: []tinkerbell.Hardware{badNetbootObject}},
		"fail to list hardware":  {shouldErr: true, failToList: true},
		"good data": {hwObject: []tinkerbell.Hardware{hwObject1}, wantDHCP: &data.DHCP{
			MACAddress:     net.HardwareAddr{0x3c, 0xec, 0xef, 0x4c, 0x4f, 0x54},
			IPAddress:      netip.MustParseAddr("172.16.10.100"),
			SubnetMask:     []byte{0xff, 0xff, 0xff, 0x00},
			DefaultGateway: netip.MustParseAddr("255.255.255.0"),
			NameServers: []net.IP{
				{0x1, 0x1, 0x1, 0x1},
			},
			Hostname:  "sm01",
			LeaseTime: 86400,
			Arch:      "x86_64",
		}, wantNetboot: &data.Netboot{
			AllowNetboot: true,
			IPXEScriptURL: &url.URL{
				Scheme: "http",
				Host:   "netboot.xyz",
			},
			Facility: "onprem",
		}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rs := runtime.NewScheme()
			if err := scheme.AddToScheme(rs); err != nil {
				t.Fatal(err)
			}
			if err := tinkerbell.AddToScheme(rs); err != nil {
				t.Fatal(err)
			}

			ct := fake.NewClientBuilder()
			if !tc.failToList {
				ct = ct.WithScheme(rs)
				ct = ct.WithRuntimeObjects(&tinkerbell.HardwareList{})
				ct = ct.WithIndex(&tinkerbell.Hardware{}, MACAddrIndex, func(client.Object) []string {
					var list []string
					for _, elem := range tc.hwObject {
						list = append(list, elem.Spec.Interfaces[0].DHCP.MAC)
					}
					return list
				})
			}
			if len(tc.hwObject) > 0 {
				t.Logf("%+v", tc.hwObject[0].Spec.Interfaces[0].DHCP)
				t.Logf("%+v", tc.hwObject[0].Spec.Interfaces[0].DHCP.MAC)
				ct = ct.WithLists(&tinkerbell.HardwareList{Items: tc.hwObject})
			}
			cl := ct.Build()

			fn := func(o *cluster.Options) {
				o.NewClient = func(*rest.Config, client.Options) (client.Client, error) {
					return cl, nil
				}
				o.MapperProvider = func(*rest.Config, *http.Client) (meta.RESTMapper, error) {
					return cl.RESTMapper(), nil
				}
				o.NewCache = func(*rest.Config, cache.Options) (cache.Cache, error) {
					return &informertest.FakeInformers{Scheme: cl.Scheme()}, nil
				}
			}
			rc := new(rest.Config)
			b, err := NewBackend(Backend{ClientConfig: rc}, fn)
			if err != nil {
				t.Fatal(err)
			}

			go b.Start(context.Background())
			gotDHCP, gotNetboot, err := b.GetByMac(context.Background(), net.HardwareAddr{0x3c, 0xec, 0xef, 0x4c, 0x4f, 0x54})
			if tc.shouldErr && err == nil {
				t.Log(err)
				t.Fatal("expected error")
			}

			if diff := cmp.Diff(gotDHCP, tc.wantDHCP, cmpopts.IgnoreUnexported(netip.Addr{})); diff != "" {
				t.Fatal(diff)
			}

			if diff := cmp.Diff(gotNetboot, tc.wantNetboot); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func newWorkingClient(t *testing.T) client.WithWatch {
	t.Helper()
	ct := fake.NewClientBuilder()

	rs := runtime.NewScheme()
	if err := scheme.AddToScheme(rs); err != nil {
		t.Fatal(err)
	}
	if err := tinkerbell.AddToScheme(rs); err != nil {
		t.Fatal(err)
	}

	ct = ct.WithScheme(rs)
	ct = ct.WithRuntimeObjects(&tinkerbell.HardwareList{})
	ct = ct.WithIndex(&tinkerbell.Hardware{}, IPAddrIndex, func(client.Object) []string {
		var list []string
		return list
	})

	return ct.Build()
}

var hwObject1 = tinkerbell.Hardware{
	TypeMeta: v1.TypeMeta{
		Kind:       "Hardware",
		APIVersion: "tinkerbell.org/v1alpha1",
	},
	ObjectMeta: v1.ObjectMeta{
		Name:      "machine1",
		Namespace: "default",
	},
	Spec: tinkerbell.HardwareSpec{
		Metadata: &tinkerbell.HardwareMetadata{
			Facility: &tinkerbell.MetadataFacility{
				FacilityCode: "onprem",
			},
		},
		Interfaces: []tinkerbell.Interface{
			{
				Netboot: &tinkerbell.Netboot{
					AllowPXE:      &[]bool{true}[0],
					AllowWorkflow: &[]bool{true}[0],
					IPXE: &tinkerbell.IPXE{
						URL: "http://netboot.xyz",
					},
				},
				DHCP: &tinkerbell.DHCP{
					Arch:     "x86_64",
					Hostname: "sm01",
					IP: &tinkerbell.IP{
						Address: "172.16.10.100",
						Gateway: "172.16.10.1",
						Netmask: "255.255.255.0",
					},
					LeaseTime:   86400,
					MAC:         "3c:ec:ef:4c:4f:54",
					NameServers: []string{"1.1.1.1"},
					UEFI:        true,
				},
			},
		},
	},
}

var hwObject2 = tinkerbell.Hardware{
	TypeMeta: v1.TypeMeta{
		Kind:       "Hardware",
		APIVersion: "tinkerbell.org/v1alpha1",
	},
	ObjectMeta: v1.ObjectMeta{
		Name:      "machine2",
		Namespace: "default",
	},
	Spec: tinkerbell.HardwareSpec{
		Interfaces: []tinkerbell.Interface{
			{
				Netboot: &tinkerbell.Netboot{
					AllowPXE:      &[]bool{true}[0],
					AllowWorkflow: &[]bool{true}[0],
					IPXE: &tinkerbell.IPXE{
						URL: "http://netboot.xyz",
					},
				},
				DHCP: &tinkerbell.DHCP{
					Arch:     "x86_64",
					Hostname: "sm01",
					IP: &tinkerbell.IP{
						Address: "172.16.10.101",
						Gateway: "172.16.10.1",
						Netmask: "255.255.255.0",
					},
					LeaseTime:   86400,
					MAC:         "3c:ec:ef:4c:4f:55",
					NameServers: []string{"1.1.1.1"},
					UEFI:        true,
				},
			},
		},
		Metadata: &tinkerbell.HardwareMetadata{
			Facility: &tinkerbell.MetadataFacility{
				FacilityCode: "ewr2",
			},
		},
	},
}

var badDHCPObject = tinkerbell.Hardware{
	TypeMeta: v1.TypeMeta{
		Kind:       "Hardware",
		APIVersion: "tinkerbell.org/v1alpha1",
	},
	ObjectMeta: v1.ObjectMeta{
		Name:      "machine2",
		Namespace: "default",
	},
	Spec: tinkerbell.HardwareSpec{
		Interfaces: []tinkerbell.Interface{
			{
				Netboot: &tinkerbell.Netboot{
					AllowPXE:      &[]bool{true}[0],
					AllowWorkflow: &[]bool{true}[0],
					IPXE: &tinkerbell.IPXE{
						URL: "http://netboot.xyz",
					},
				},
				DHCP: &tinkerbell.DHCP{
					Arch:     "x86_64",
					Hostname: "sm01",
					IP: &tinkerbell.IP{
						Address: "172.16.10.100",
						Gateway: "bad-address",
						Netmask: "255.255.255.0",
					},
					LeaseTime:   86400,
					MAC:         "3c:ec:ef:4c:4f:54",
					NameServers: []string{"1.1.1.1"},
					UEFI:        true,
				},
			},
		},
	},
}

var badDHCPObject2 = tinkerbell.Hardware{
	TypeMeta: v1.TypeMeta{
		Kind:       "Hardware",
		APIVersion: "tinkerbell.org/v1alpha1",
	},
	ObjectMeta: v1.ObjectMeta{
		Name:      "machine2",
		Namespace: "default",
	},
	Spec: tinkerbell.HardwareSpec{
		Interfaces: []tinkerbell.Interface{
			{
				Netboot: &tinkerbell.Netboot{
					AllowPXE:      &[]bool{true}[0],
					AllowWorkflow: &[]bool{true}[0],
					IPXE: &tinkerbell.IPXE{
						URL: "http://netboot.xyz",
					},
				},
				DHCP: &tinkerbell.DHCP{
					Arch:     "x86_64",
					Hostname: "sm01",
					IP: &tinkerbell.IP{
						Address: "172.16.10.100",
						Gateway: "bad-address",
						Netmask: "255.255.255.0",
					},
					LeaseTime:   86400,
					MAC:         "3c:ec:ef:4c:4f:55",
					NameServers: []string{"1.1.1.1"},
					UEFI:        true,
				},
			},
		},
	},
}

var badNetbootObject = tinkerbell.Hardware{
	TypeMeta: v1.TypeMeta{
		Kind:       "Hardware",
		APIVersion: "tinkerbell.org/v1alpha1",
	},
	ObjectMeta: v1.ObjectMeta{
		Name:      "machine2",
		Namespace: "default",
	},
	Spec: tinkerbell.HardwareSpec{
		Interfaces: []tinkerbell.Interface{
			{
				Netboot: &tinkerbell.Netboot{
					IPXE: &tinkerbell.IPXE{
						URL: "bad-url",
					},
				},
				DHCP: &tinkerbell.DHCP{
					Hostname: "sm01",
					IP: &tinkerbell.IP{
						Address: "172.16.10.101",
						Gateway: "172.16.10.1",
						Netmask: "255.255.255.0",
					},
					LeaseTime:   86400,
					MAC:         "3c:ec:ef:4c:4f:54",
					NameServers: []string{"1.1.1.1"},
				},
			},
		},
	},
}

var badNetbootObject2 = tinkerbell.Hardware{
	TypeMeta: v1.TypeMeta{
		Kind:       "Hardware",
		APIVersion: "tinkerbell.org/v1alpha1",
	},
	ObjectMeta: v1.ObjectMeta{
		Name:      "machine2",
		Namespace: "default",
	},
	Spec: tinkerbell.HardwareSpec{
		Interfaces: []tinkerbell.Interface{
			{
				Netboot: &tinkerbell.Netboot{
					IPXE: &tinkerbell.IPXE{
						URL: "bad-url",
					},
				},
				DHCP: &tinkerbell.DHCP{
					Hostname: "sm01",
					IP: &tinkerbell.IP{
						Address: "172.16.10.100",
						Gateway: "172.16.10.1",
						Netmask: "255.255.255.0",
					},
					LeaseTime:   86400,
					MAC:         "3c:ec:ef:4c:4f:54",
					NameServers: []string{"1.1.1.1"},
				},
			},
		},
	},
}
