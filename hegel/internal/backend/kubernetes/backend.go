package kubernetes

import (
	"context"
	"fmt"
	"sync"

	"github.com/tinkerbell/hegel/internal/datamodel"
	"github.com/tinkerbell/hegel/internal/frontend/ec2"
	tinkv1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	tinkcontrollers "github.com/tinkerbell/tink/pkg/controllers"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Backend is a hardware Backend backed by a Backend cluster that contains hardware resources.
type Backend struct {
	client           ListerClient
	close            func()
	closeM           *sync.RWMutex
	waitForCacheSync func(context.Context) bool
}

// NewOrDie creates a new Backend Backend. It panics upon error.
func NewOrDie(config Config) *Backend {
	backend, err := New(config)
	if err != nil {
		panic(err)
	}
	return backend
}

// NewBackend creates a new Backend Backend instance. It launches a goroutine to perform synchronization
// between the cluster and internal caches. Consumers can wait for the initial sync using WaitForCachesync().
// See k8s.io/Backend-go/tools/Backendcmd for constructing *rest.Config objects.
func New(config Config) (*Backend, error) {
	opts := tinkcontrollers.GetServerOptions()
	opts.Namespace = config.Namespace

	// Use a manager from the tink project so we can take advantage of the indexes and caching it configures.
	// Once started, we don't really need any of the manager capabilities hence we don't store it in the
	// Backend
	manager, err := tinkcontrollers.NewManager(config.Config, opts)
	if err != nil {
		return nil, err
	}

	managerCtx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := manager.Start(managerCtx); err != nil {
			panic(err)
		}
	}()

	backend := NewBackendWithClient(manager.GetClient())
	backend.close = cancel
	backend.waitForCacheSync = manager.GetCache().WaitForCacheSync

	return backend, nil
}

// ListerClient lists Kubernetes resources using a sigs.k8s.io/controller-runtime Backend.
type ListerClient interface {
	List(ctx context.Context, list crclient.ObjectList, opts ...crclient.ListOption) error
}

// NewBackendWithClient creates a new Backend instance that uses Backend to find resources.
// The Close() and WaitForCacheSync() methods of the returned Backend are noops.
func NewBackendWithClient(client ListerClient) *Backend {
	return &Backend{
		client:           client,
		close:            func() {},
		closeM:           &sync.RWMutex{},
		waitForCacheSync: func(context.Context) bool { return true },
	}
}

// Close stops synchronization with the cluster. Subsequent calls to all of k's methods, excluding IsHealthy(), have
// undefined behavior.
func (b *Backend) Close() {
	b.closeM.Lock()
	defer b.closeM.Unlock()

	// nil indicates Close() has been called for other methods.
	b.close()
	b.close = nil
}

// WaitForCacheSync waits for the internal Backend cache to synchronize.
func (b *Backend) WaitForCacheSync(ctx context.Context) bool {
	return b.waitForCacheSync(ctx)
}

// IsHealthy returns true until k.Close() is called. It is thread safe.
func (b *Backend) IsHealthy(context.Context) bool {
	b.closeM.RLock()
	defer b.closeM.RUnlock()
	return b.close != nil
}

// GetEC2InstanceByIP satisfies ec2.Client.
func (b *Backend) GetEC2Instance(ctx context.Context, ip string) (ec2.Instance, error) {
	hw, err := b.retrieveByIP(ctx, ip)
	if err != nil {
		return ec2.Instance{}, err
	}

	return toEC2Instance(hw), nil
}

func (b *Backend) retrieveByIP(ctx context.Context, ip string) (tinkv1.Hardware, error) {
	var hw tinkv1.HardwareList
	err := b.client.List(ctx, &hw, crclient.MatchingFields{
		tinkcontrollers.HardwareIPAddrIndex: ip,
	})
	if err != nil {
		return tinkv1.Hardware{}, err
	}

	if len(hw.Items) == 0 {
		return tinkv1.Hardware{}, fmt.Errorf("no hardware with ip '%v'", ip)
	}

	if len(hw.Items) > 1 {
		return tinkv1.Hardware{}, fmt.Errorf("multiple hardware with ip '%v'", ip)
	}

	return hw.Items[0], nil
}

func (b *Backend) GetDataModel() datamodel.DataModel {
	return datamodel.Kubernetes
}

func toEC2Instance(tinkv1.Hardware) ec2.Instance {
	return ec2.Instance{}
}
