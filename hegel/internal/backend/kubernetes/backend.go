package kubernetes

import (
	"context"
	"errors"
	"fmt"

	"github.com/tinkerbell/hegel/internal/frontend/ec2"
	tinkv1 "github.com/tinkerbell/tink/api/v1alpha1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

var errNotFound = errors.New("no hardware found")

// Build the scheme as a package variable so we don't need to perform error checks.
var scheme = kubescheme.Scheme

func init() {
	utilruntime.Must(tinkv1.AddToScheme(scheme))
}

// Backend is a hardware Backend backed by a Backend cluster that contains hardware resources.
type Backend struct {
	client listerClient
	closer <-chan struct{}

	// WaitForCacheSync waits for the initial sync to be completed. Returns false if the cache
	// fails to sync.
	WaitForCacheSync func(context.Context) bool
}

// NewBackend creates a new Backend instance. It launches a goroutine to perform synchronization
// between the cluster and internal caches. Consumers can wait for the initial sync using WaitForCachesync().
// See k8s.io/Backend-go/tools/Backendcmd for constructing *rest.Config objects.
func NewBackend(ctx context.Context, cfg Config) (*Backend, error) {
	// If no client was specified, build one and configure the backend with it including waiting
	// for the caches to sync.
	if cfg.ClientConfig == nil {
		var err error
		cfg, err = loadConfig(cfg)
		if err != nil {
			return nil, err
		}
	}

	conf := func(opts *cluster.Options) {
		opts.Scheme = scheme
		if cfg.Namespace != "" {
			opts.Cache.DefaultNamespaces = map[string]cache.Config{cfg.Namespace: {}}
		}
	}

	clstr, err := cluster.New(cfg.ClientConfig, conf)
	if err != nil {
		return nil, fmt.Errorf("create cluster: %v", err)
	}

	err = clstr.GetFieldIndexer().IndexField(
		ctx,
		&tinkv1.Hardware{},
		hardwareIPAddrIndex,
		hardwareIPIndexFunc,
	)
	if err != nil {
		return nil, fmt.Errorf("register index: %v", err)
	}

	// TODO(chrisdoherty4) Stop panicing on error. This will likely require exposing Start in
	// some capacity and allowing the caller to handle the error.
	go func() {
		if err := clstr.Start(ctx); err != nil {
			panic(err)
		}
	}()

	return &Backend{
		closer:           ctx.Done(),
		client:           clstr.GetClient(),
		WaitForCacheSync: clstr.GetCache().WaitForCacheSync,
	}, nil
}

func loadConfig(cfg Config) (Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = cfg.Kubeconfig

	overrides := &clientcmd.ConfigOverrides{
		ClusterInfo: clientcmdapi.Cluster{
			Server: cfg.APIServerAddress,
		},
		Context: clientcmdapi.Context{
			Namespace: cfg.Namespace,
		},
	}

	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	config, err := loader.ClientConfig()
	if err != nil {
		return Config{}, err
	}
	cfg.ClientConfig = config

	return cfg, nil
}

// IsHealthy returns true until the context used to create the Backend is cancelled.
func (b *Backend) IsHealthy(context.Context) bool {
	select {
	case <-b.closer:
		return false
	default:
		return true
	}
}

// GetEC2InstanceByIP satisfies ec2.Client.
func (b *Backend) GetEC2Instance(ctx context.Context, ip string) (ec2.Instance, error) {
	hw, err := b.retrieveByIP(ctx, ip)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return ec2.Instance{}, ec2.ErrInstanceNotFound
		}

		return ec2.Instance{}, err
	}

	return toEC2Instance(hw), nil
}

func (b *Backend) retrieveByIP(ctx context.Context, ip string) (tinkv1.Hardware, error) {
	var hw tinkv1.HardwareList
	err := b.client.List(ctx, &hw, crclient.MatchingFields{
		hardwareIPAddrIndex: ip,
	})
	if err != nil {
		return tinkv1.Hardware{}, err
	}

	if len(hw.Items) == 0 {
		return tinkv1.Hardware{}, errNotFound
	}

	if len(hw.Items) > 1 {
		return tinkv1.Hardware{}, fmt.Errorf("multiple hardware found")
	}

	return hw.Items[0], nil
}

// listerClient lists Kubernetes resources using a sigs.k8s.io/controller-runtime Backend.
type listerClient interface {
	List(ctx context.Context, list crclient.ObjectList, opts ...crclient.ListOption) error
}

//nolint:cyclop // This function is just mapping data with a bunch of nil checks, it's not complex.
func toEC2Instance(hw tinkv1.Hardware) ec2.Instance {
	var i ec2.Instance

	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil {
		i.Metadata.InstanceID = hw.Spec.Metadata.Instance.ID
		i.Metadata.Hostname = hw.Spec.Metadata.Instance.Hostname
		i.Metadata.LocalHostname = hw.Spec.Metadata.Instance.Hostname
		i.Metadata.Tags = hw.Spec.Metadata.Instance.Tags

		if hw.Spec.Metadata.Instance.OperatingSystem != nil {
			i.Metadata.OperatingSystem.Slug = hw.Spec.Metadata.Instance.OperatingSystem.Slug
			i.Metadata.OperatingSystem.Distro = hw.Spec.Metadata.Instance.OperatingSystem.Distro
			i.Metadata.OperatingSystem.Version = hw.Spec.Metadata.Instance.OperatingSystem.Version
			i.Metadata.OperatingSystem.ImageTag = hw.Spec.Metadata.Instance.OperatingSystem.ImageTag
		}

		// Iterate over all IPs and set the first one for IPv4 and IPv6 as the values in the
		// instance metadata.
		for _, ip := range hw.Spec.Metadata.Instance.Ips {
			// Public IPv4
			if ip.Family == 4 && ip.Public && i.Metadata.PublicIPv4 == "" {
				i.Metadata.PublicIPv4 = ip.Address
			}

			// Private IPv4
			if ip.Family == 4 && !ip.Public && i.Metadata.LocalIPv4 == "" {
				i.Metadata.LocalIPv4 = ip.Address
			}

			// Public IPv6
			if ip.Family == 6 && i.Metadata.PublicIPv6 == "" {
				i.Metadata.PublicIPv6 = ip.Address
			}
		}
	}

	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Facility != nil {
		i.Metadata.Plan = hw.Spec.Metadata.Facility.PlanSlug
		i.Metadata.Facility = hw.Spec.Metadata.Facility.FacilityCode
	}

	if hw.Spec.UserData != nil {
		i.Userdata = *hw.Spec.UserData
	}

	// TODO(chrisdoherty4) Support public keys. The frontend doesn't handle public keys correctly
	// as it expects a single string and just outputs that key. Until we can support multiple keys
	// its not worth adding it to the metadata.
	//
	// https://github.com/tinkerbell/hegel/issues/165

	return i
}
