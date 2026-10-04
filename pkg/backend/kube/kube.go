// Package kube is a backend implementation that uses the Tinkerbell CRDs to get DHCP data.
package kube

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// TODO(jacobweinstock): think about whether all methods should return the v1alpha1 objects and then
// let the consumers of this package convert them to the data objects.

const tracerName = "github.com/tinkerbell/tinkerbell"

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
	Indexes       map[IndexType]Index
	dynamicClient dynamic.Interface
	// HardwareReferenceAllowListRules and HardwareReferenceDenyListRules are the Quamina rules
	// ResolveReferences applies. An empty deny list denies every reference.
	HardwareReferenceAllowListRules []string
	HardwareReferenceDenyListRules  []string
	// HardwareTemplating renders the templates in Hardware specs for Smee,
	// Tootles and Workflow rendering.
	HardwareTemplating bool
	// Logger receives background rendering errors. Optional.
	Logger logr.Logger
	store  *renderStore
	// QPS is the maximum queries per second to the Kubernetes API server.
	// If set to 0, defaults to 5. Negative values disable rate limiting.
	QPS float32
	// Burst is the maximum burst for throttle in the Kubernetes client.
	// If set to 0, defaults to 10. Negative values disable burst limiting.
	Burst int
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

	if err := tinkerbell.AddToScheme(rs); err != nil {
		return nil, err
	}

	if err := bmc.AddToScheme(rs); err != nil {
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

	dc, err := dynamic.NewForConfig(cfg.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	b := &Backend{
		cluster:                         c,
		ConfigFilePath:                  cfg.ConfigFilePath,
		APIURL:                          cfg.APIURL,
		Namespace:                       cfg.Namespace,
		ClientConfig:                    cfg.ClientConfig,
		dynamicClient:                   dc,
		HardwareReferenceAllowListRules: cfg.HardwareReferenceAllowListRules,
		HardwareReferenceDenyListRules:  cfg.HardwareReferenceDenyListRules,
		HardwareTemplating:              cfg.HardwareTemplating,
		Logger:                          cfg.Logger,
	}
	if b.HardwareTemplating {
		b.store, err = b.newRenderStore()
		if err != nil {
			return nil, err
		}
	}

	return b, nil
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
		return Backend{}, fmt.Errorf("failed to load client config: %w", err)
	}
	config.QPS = cfg.QPS
	config.Burst = cfg.Burst
	cfg.ClientConfig = config

	return cfg, nil
}

// Start starts the client-side cache and, with Hardware templating, background
// rendering.
func (b *Backend) Start(ctx context.Context) error {
	if b.store == nil {
		return b.cluster.Start(ctx)
	}
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return b.cluster.Start(ctx) })
	g.Go(func() error { return b.store.Start(ctx, renderWorkers) })
	return g.Wait()
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

func ternary[T any](condition bool, valueIfTrue, valueIfFalse T) T {
	if condition {
		return valueIfTrue
	}
	return valueIfFalse
}
