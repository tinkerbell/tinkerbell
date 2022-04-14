package hardware

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/pkg/errors"
	tinkv1alpha1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	tink "github.com/tinkerbell/tink/pkg/controllers"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Client = &KubernetesClient{}

// KubernetesClient is a hardware client backed by a KubernetesClient cluster that contains hardware resources.
type KubernetesClient struct {
	client           ListerClient
	close            func()
	closeM           *sync.RWMutex
	waitForCacheSync func(context.Context) bool
}

// NewKubernetesClientOrDie creates a new KubernetesClient client. It panics upon error.
func NewKubernetesClientOrDie(config KubernetesClientConfig) *KubernetesClient {
	client, err := NewKubernetesClient(config)
	if err != nil {
		panic(err)
	}
	return client
}

// NewKubernetesClient creates a new KubernetesClient client instance. It launches a goroutine to perform synchronization
// between the cluster and internal caches. Consumers can wait for the initial sync using WaitForCachesync().
// See k8s.io/client-go/tools/clientcmd for constructing *rest.Config objects.
func NewKubernetesClient(config KubernetesClientConfig) (*KubernetesClient, error) {
	opts := tink.GetServerOptions()
	opts.Namespace = config.Namespace

	// Use a manager from the tink project so we can take advantage of the indexes and caching it configures.
	// Once started, we don't really need any of the manager capabilities hence we don't store it in the
	// KubernetesClient
	manager, err := tink.NewManager(config.Config, opts)
	if err != nil {
		return nil, err
	}

	managerCtx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := manager.Start(managerCtx); err != nil {
			panic(err)
		}
	}()

	client := NewKubernetesClientWithClient(manager.GetClient())
	client.close = cancel
	client.waitForCacheSync = manager.GetCache().WaitForCacheSync

	return client, nil
}

// ListerClient lists Kubernetes resources using a sigs.k8s.io/controller-runtime client.
type ListerClient interface {
	List(ctx context.Context, list crclient.ObjectList, opts ...crclient.ListOption) error
}

// NewKubernetesClientWithClient creates a new KubernetesClient instance that uses client to find resources. The
// Close() and WaitForCacheSync() methods of the returned client are noops.
func NewKubernetesClientWithClient(client ListerClient) *KubernetesClient {
	return &KubernetesClient{
		client:           client,
		close:            func() {},
		closeM:           &sync.RWMutex{},
		waitForCacheSync: func(context.Context) bool { return true },
	}
}

// Close stops synchronization with the cluster. Subsequent calls to all of k's methods, excluding IsHealthy(), have
// undefined behavior.
func (k *KubernetesClient) Close() {
	k.closeM.Lock()
	defer k.closeM.Unlock()

	// nil indicates Close() has been called for other methods.
	k.close()
	k.close = nil
}

// WaitForCacheSync waits for the internal client cache to synchronize.
func (k *KubernetesClient) WaitForCacheSync(ctx context.Context) bool {
	return k.waitForCacheSync(ctx)
}

// IsHealthy returns true until k.Close() is called. It is thread safe.
func (k *KubernetesClient) IsHealthy(context.Context) bool {
	k.closeM.RLock()
	defer k.closeM.RUnlock()
	return k.close != nil
}

// ByIP retrieves a hardware resource associated with ip.
func (k *KubernetesClient) ByIP(ctx context.Context, ip string) (Hardware, error) {
	var hw tinkv1alpha1.HardwareList
	err := k.client.List(ctx, &hw, crclient.MatchingFields{
		tink.HardwareIPAddrIndex: ip,
	})
	if err != nil {
		return nil, err
	}

	if len(hw.Items) == 0 {
		return nil, fmt.Errorf("no hardware with ip '%v'", ip)
	}

	if len(hw.Items) > 1 {
		names := make([]string, len(hw.Items))
		for i, item := range hw.Items {
			names[i] = item.Name
		}
		return nil, fmt.Errorf("multiple hardware with ip '%v': [%v]", ip, strings.Join(names, ", "))
	}

	return &hardware{hw.Items[0]}, nil
}

// Watch is unimplemented.
func (k *KubernetesClient) Watch(context.Context, string) (Watcher, error) {
	return nil, errors.New("kubernetes client: watch is unimplemented")
}

// KuberneteSClientConfig used by the NewKubernetesClient function family.
type KubernetesClientConfig struct {
	*rest.Config
	Namespace string
}

// NewKubernetesClientConfig loads the kubeconfig overriding it with kubeAPI.
func NewKubernetesClientConfig(kubeconfig, kubeAPI string) (KubernetesClientConfig, error) {
	kubeClientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{
			ExplicitPath: kubeconfig,
		},
		&clientcmd.ConfigOverrides{
			ClusterInfo: clientcmdapi.Cluster{
				Server: kubeAPI,
			},
		},
	)

	config, err := kubeClientCfg.ClientConfig()
	if err != nil {
		return KubernetesClientConfig{}, err
	}

	namespace, _, err := kubeClientCfg.Namespace()
	if err != nil {
		return KubernetesClientConfig{}, err
	}

	return KubernetesClientConfig{
		Config:    config,
		Namespace: namespace,
	}, nil
}

type hardware struct {
	tinkv1alpha1.Hardware
}

func (h hardware) Export() ([]byte, error) {
	return json.Marshal(h.Hardware.Spec)
}

func (h hardware) ID() (string, error) {
	return h.Spec.Metadata.Instance.ID, nil
}
