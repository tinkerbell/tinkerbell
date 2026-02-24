package kube

import (
	"net"
	"net/http"
	"testing"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// mustParseCIDR is a helper function for tests to parse CIDR strings
func mustParseCIDR(cidr string) *net.IPNet {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return ipnet
}

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
				IndexType("one"):              {Obj: &v1alpha1.Hardware{}, Field: MACAddrIndex, ExtractValue: MACAddrs},
				IndexType("duplicate of one"): {Obj: &v1alpha1.Hardware{}, Field: MACAddrIndex, ExtractValue: MACAddrs},
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

func newWorkingClient(t *testing.T) client.WithWatch {
	t.Helper()
	ct := fake.NewClientBuilder()

	rs := runtime.NewScheme()
	if err := scheme.AddToScheme(rs); err != nil {
		t.Fatal(err)
	}
	if err := api.AddToSchemeTinkerbell(rs); err != nil {
		t.Fatal(err)
	}

	ct = ct.WithScheme(rs)
	ct = ct.WithRuntimeObjects(&v1alpha1.HardwareList{})
	ct = ct.WithIndex(&v1alpha1.Hardware{}, IPAddrIndex, func(client.Object) []string {
		var list []string
		return list
	})

	return ct.Build()
}
