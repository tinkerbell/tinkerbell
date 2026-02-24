package kube

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	"github.com/tinkerbell/tinkerbell/pkg/data"
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

func TestReadHardwareByIP(t *testing.T) {
	tests := map[string]struct {
		hwObject   []tinkerbell.Hardware
		want       *tinkerbell.Hardware
		shouldErr  bool
		failToList bool
	}{
		"empty hardware list":    {shouldErr: true, hwObject: []tinkerbell.Hardware{}},
		"more than one hardware": {shouldErr: true, hwObject: []tinkerbell.Hardware{hwObject1, hwObject2}},
		"fail to list hardware":  {shouldErr: true, failToList: true},
		"good data": {
			hwObject: []tinkerbell.Hardware{hwObject1},
			want:     &hwObject1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rs := runtime.NewScheme()
			if err := scheme.AddToScheme(rs); err != nil {
				t.Fatal(err)
			}
			if err := api.AddToSchemeTinkerbell(rs); err != nil {
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
			got, err := b.ReadHardware(context.Background(), "", "", data.ReadListOptions{Hardware: data.HardwareReadOptions{ByIPAddress: "172.16.10.100"}})
			if tc.shouldErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(got.Spec, tc.want.Spec); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestReadHardwareByMac(t *testing.T) {
	tests := map[string]struct {
		hwObject   []tinkerbell.Hardware
		want       *tinkerbell.Hardware
		shouldErr  bool
		failToList bool
	}{
		"empty hardware list":    {shouldErr: true},
		"more than one hardware": {shouldErr: true, hwObject: []tinkerbell.Hardware{hwObject1, hwObject2}},
		"fail to list hardware":  {shouldErr: true, failToList: true},
		"good data": {
			hwObject: []tinkerbell.Hardware{hwObject1},
			want:     &hwObject1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rs := runtime.NewScheme()
			if err := scheme.AddToScheme(rs); err != nil {
				t.Fatal(err)
			}
			if err := api.AddToSchemeTinkerbell(rs); err != nil {
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
			got, err := b.ReadHardware(context.Background(), "", "", data.ReadListOptions{Hardware: data.HardwareReadOptions{ByMACAddress: "3c:ec:ef:4c:4f:54"}})
			if tc.shouldErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(got.Spec, tc.want.Spec); diff != "" {
				t.Fatal(diff)
			}
		})
	}
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
					Arch:       "x86_64",
					Hostname:   "sm01",
					DomainName: "example.com",
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
