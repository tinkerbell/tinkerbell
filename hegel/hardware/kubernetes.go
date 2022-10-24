package hardware

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

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
		return nil, fmt.Errorf("multiple hardware with ip '%v'", ip)
	}

	return FromK8sTinkHardware(&hw.Items[0]), nil
}

// KuberneteSClientConfig used by the NewKubernetesClient function family.
type KubernetesClientConfig struct {
	*rest.Config
	Namespace string
}

// NewKubernetesClientConfig loads the kubeconfig overriding it with kubeAPI.
func NewKubernetesClientConfig(kubeconfig, kubeAPI, namespace string) (KubernetesClientConfig, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfig

	overrides := &clientcmd.ConfigOverrides{
		ClusterInfo: clientcmdapi.Cluster{
			Server: kubeAPI,
		},
		Context: clientcmdapi.Context{
			Namespace: namespace,
		},
	}

	kubeClientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	config, err := kubeClientCfg.ClientConfig()
	if err != nil {
		return KubernetesClientConfig{}, err
	}

	namespace, _, err = kubeClientCfg.Namespace()
	if err != nil {
		return KubernetesClientConfig{}, err
	}

	return KubernetesClientConfig{
		Config:    config,
		Namespace: namespace,
	}, nil
}

// FromK8sTinkHardware creates an K8sHardware from tinkHardware.
func FromK8sTinkHardware(tinkHardware *tinkv1alpha1.Hardware) *K8sHardware {
	hw := &K8sHardware{
		Hardware: tinkHardware,
		Metadata: K8sHardwareMetadata{
			Userdata:   tinkHardware.Spec.UserData,
			Vendordata: tinkHardware.Spec.VendorData,
			Instance: K8sHardwareMetadataInstance{
				ID:        tinkHardware.Spec.Metadata.Instance.ID,
				Hostname:  tinkHardware.Spec.Metadata.Instance.Hostname,
				Plan:      tinkHardware.Spec.Metadata.Facility.PlanSlug,
				Factility: tinkHardware.Spec.Metadata.Facility.FacilityCode,
				Tags:      tinkHardware.Spec.Metadata.Instance.Tags,
				SSHKeys:   tinkHardware.Spec.Metadata.Instance.SSHKeys,
				OperatingSystem: K8sHardwareMetadataInstanceOperatingSystem{
					Slug:     tinkHardware.Spec.Metadata.Instance.OperatingSystem.Slug,
					Distro:   tinkHardware.Spec.Metadata.Instance.OperatingSystem.Distro,
					Version:  tinkHardware.Spec.Metadata.Instance.OperatingSystem.Version,
					ImageTag: tinkHardware.Spec.Metadata.Instance.OperatingSystem.ImageTag,
				},
			},
		},
	}
	// appending all hardware IPs to the array of Addresses within the Network sub-struct
	for _, ip := range tinkHardware.Spec.Metadata.Instance.Ips {
		hw.Metadata.Instance.Network.Addresses = append(
			hw.Metadata.Instance.Network.Addresses,
			K8sHardwareMetadataInstanceNetworkAddress{
				Address:       ip.Address,
				AddressFamily: ip.Family,
				Public:        ip.Public,
			},
		)
	}

	// appending all disk devices to the array of Disks within the Metadata struct
	for _, disk := range tinkHardware.Spec.Disks {
		hw.Metadata.Instance.Disks = append(
			hw.Metadata.Instance.Disks,
			K8sHardwareDisk{
				Device: disk.Device,
			},
		)
	}

	// appending all network interface info to the array of interfaces within the Metadata sub-struct
	for _, networkInterface := range tinkHardware.Spec.Interfaces {
		if networkInterface.DHCP != nil {
			hw.Metadata.Gateway = networkInterface.DHCP.IP.Gateway
			hw.Metadata.Interfaces = append(
				hw.Metadata.Interfaces,
				K8sNetworkInterface{
					MAC:     networkInterface.DHCP.MAC,
					Address: networkInterface.DHCP.IP.Address,
					Family:  networkInterface.DHCP.IP.Family,
					Netmask: networkInterface.DHCP.IP.Netmask,
				},
			)
		}
	}
	return hw
}

// K8sHardware satisfies the Export() requirements of the EC2 Metadata filter handling.
type K8sHardware struct {
	Hardware *tinkv1alpha1.Hardware `json:"-"`
	Metadata K8sHardwareMetadata    `json:"metadata,omitempty"`
}

// Exprot returns a JSON representation of h.
func (h K8sHardware) Export() ([]byte, error) {
	return json.Marshal(h)
}

// ID retrieves the instance ID.
func (h K8sHardware) ID() (string, error) {
	return h.Metadata.Instance.ID, nil
}

type K8sHardwareMetadata struct {
	Userdata   *string                     `json:"userdata,omitempty"`
	Vendordata *string                     `json:"vendordata,omitempty"`
	Instance   K8sHardwareMetadataInstance `json:"instance,omitempty"`
	//+optional
	Interfaces []K8sNetworkInterface `json:"interfaces,omitempty"`
	Gateway    string                `json:"gateway,omitempty"`
}

type K8sNetworkInterface struct {
	//+optional
	MAC     string `json:"mac,omitempty"`
	Address string `json:"address,omitempty"`
	Netmask string `json:"netmask,omitempty"`
	Family  int64  `json:"family,omitempty"`
}

type K8sHardwareMetadataInstance struct {
	Disks           []K8sHardwareDisk                          `json:"disks,omitempty"`
	ID              string                                     `json:"id,omitempty"`
	Hostname        string                                     `json:"hostname,omitempty"`
	Plan            string                                     `json:"plan,omitempty"`
	Factility       string                                     `json:"factility,omitempty"`
	Tags            []string                                   `json:"tags,omitempty"`
	SSHKeys         []string                                   `json:"ssh_keys,omitempty"`
	OperatingSystem K8sHardwareMetadataInstanceOperatingSystem `json:"operating_system,omitempty"`
	Network         K8sHardwareMetadataInstanceNetwork         `json:"network,omitempty"`
}

type K8sHardwareMetadataInstanceOperatingSystem struct {
	Slug     string `json:"slug,omitempty"`
	Distro   string `json:"distro,omitempty"`
	Version  string `json:"version,omitempty"`
	ImageTag string `json:"image_tag,omitempty"`
}

type K8sHardwareMetadataInstanceNetwork struct {
	Addresses []K8sHardwareMetadataInstanceNetworkAddress `json:"addresses,omitempty"`
}

type K8sHardwareMetadataInstanceNetworkAddress struct {
	AddressFamily int64  `json:"address_family,omitempty"`
	Address       string `json:"address,omitempty"`
	Public        bool   `json:"public,omitempty"`
}

type K8sHardwareDisk struct {
	//+optional
	Device string `json:"device,omitempty"`
}
