// Package kube is a backend implementation that uses the Tinkerbell CRDs to get DHCP data.
package kube

import (
	"context"
	"errors"
	"fmt"

	"github.com/tinkerbell/tinkerbell/api/tinkerbell/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/scale/scheme"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// TODO(jacobweinstock): think about whether all methods should return the v1alpha1 objects and then
// let the consumers of this package convert them to the data objects.

const tracerName = "github.com/tinkerbell/tinkerbell"

var errNotFound = errors.New("no hardware found")

// ErrInstanceNotFound indicates an instance could not be found for the given identifier.
var ErrInstanceNotFound = errors.New("instance not found")

// Backend is a backend implementation that uses the Tinkerbell CRDs to get DHCP data.
type Backend struct {
	cluster cluster.Cluster
	// ConfigFilePath is the path to a kubernetes config file (kubeconfig).
	ConfigFilePath string
	// APIURL is the Kubernetes API URL.
	APIURL string
	// Namespace is an override for the Namespace the kubernetes client will watch.
	// The default is the Namespace the pod is running in.
	Namespace string
	// ClientConfig is a Kubernetes client config. If specified, it will be used instead of
	// constructing a client using the other configuration in this object. Optional.
	ClientConfig *rest.Config
	// Indexes to register
	Indexes map[IndexType]Index
}

type Index struct {
	Obj          client.Object
	Field        string
	ExtractValue client.IndexerFunc
}

// NewBackend returns a controller-runtime cluster.Cluster with the Tinkerbell runtime
// scheme registered, and indexers for:
// * Hardware by MAC address
// * Hardware by IP address
//
// Callers must instantiate the client-side cache by calling Start() before use.
func NewBackend(cfg Backend, opts ...cluster.Option) (*Backend, error) {
	if cfg.ClientConfig == nil {
		b, err := loadConfig(cfg)
		if err != nil {
			return nil, err
		}
		cfg = b
	}
	rs := runtime.NewScheme()

	if err := scheme.AddToScheme(rs); err != nil {
		return nil, err
	}

	if err := v1alpha1.AddToScheme(rs); err != nil {
		return nil, err
	}
	conf := func(o *cluster.Options) {
		o.Scheme = rs
		if cfg.Namespace != "" {
			o.Cache.DefaultNamespaces = map[string]cache.Config{cfg.Namespace: {}}
		}
	}
	opts = append(opts, conf)
	// remove nils from opts
	sanitizedOpts := make([]cluster.Option, 0, len(opts))
	for _, opt := range opts {
		if opt != nil {
			sanitizedOpts = append(sanitizedOpts, opt)
		}
	}
	c, err := cluster.New(cfg.ClientConfig, sanitizedOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create new cluster config: %w", err)
	}

	for _, i := range cfg.Indexes {
		if err := c.GetFieldIndexer().IndexField(context.Background(), i.Obj, i.Field, i.ExtractValue); err != nil {
			return nil, fmt.Errorf("failed to setup indexer(%s): %w", i.Field, err)
		}
	}

	return &Backend{
		cluster:        c,
		ConfigFilePath: cfg.ConfigFilePath,
		APIURL:         cfg.APIURL,
		Namespace:      cfg.Namespace,
		ClientConfig:   cfg.ClientConfig,
	}, nil
}

func loadConfig(cfg Backend) (Backend, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = cfg.ConfigFilePath

	overrides := &clientcmd.ConfigOverrides{
		ClusterInfo: clientcmdapi.Cluster{
			Server: cfg.APIURL,
		},
		Context: clientcmdapi.Context{
			Namespace: cfg.Namespace,
		},
	}

	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	config, err := loader.ClientConfig()
	if err != nil {
		return Backend{}, err
	}
	cfg.ClientConfig = config

	return cfg, nil
}

// Start starts the client-side cache.
func (b *Backend) Start(ctx context.Context) error {
	return b.cluster.Start(ctx)
}

func NewFileRestConfig(kubeconfigPath, namespace string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfigPath

	overrides := &clientcmd.ConfigOverrides{
		ClusterInfo: clientcmdapi.Cluster{
			Server: "",
		},
		Context: clientcmdapi.Context{
			Namespace: namespace,
		},
	}
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	return loader.ClientConfig()
}
