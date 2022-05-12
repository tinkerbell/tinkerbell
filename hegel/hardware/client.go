package hardware

import (
	"context"

	cacher "github.com/packethost/cacher/client"
	"github.com/pkg/errors"
	"github.com/tinkerbell/hegel/datamodel"
	tink "github.com/tinkerbell/tink/client"
)

// Client defines the behaviors for interacting with hardware data providers.
type Client interface {
	// IsHealthy reports whether the client is connected and can retrieve hardware data from the data provider.
	IsHealthy(ctx context.Context) bool

	// ByIP retrieves hardware data by its IP address.
	ByIP(ctx context.Context, ip string) (Hardware, error)

	// Watch creates a subscription to a hardware identified by id such that updates to the hardware data are
	// pushed to the stream.
	Watch(ctx context.Context, id string) (Watcher, error)
}

// Hardware is the interface for Cacher/Tink hardware types.
type Hardware interface {
	Export() ([]byte, error)
	ID() (string, error)
}

// Watcher is the interface for Cacher/Tink watch client types.
type Watcher interface {
	Recv() (Hardware, error)
}

// ClientConfig is the configuration used by the NewClient func. Field requirements are based on the value of Model.
type ClientConfig struct {
	// Model defines the client implementation that is constructed.
	// Required.
	Model datamodel.DataModel

	// Facility is used by the Cache client.
	// Required for datamodel.Cacher.
	Facility string

	// KubeAPI is the URL of the Kube API the Kubernetes client talks to.
	// Required for datamodel.Kubernetes.
	KubeAPI string

	// Kuberconfig is a path to a Kubeconfig file used by the Kubernetes client.
	// Required for datamodel.Kubernetes.
	Kubeconfig string
}

func (v ClientConfig) validate() error {
	if v.Model == datamodel.Cacher {
		if v.Facility == "" {
			return errors.New("cacher data model: factility is required")
		}
	}

	return nil
}

// NewClient returns a new hardware Client, configured appropriately according to the mode (Cacher or Tink) Hegel is running in.
func NewClient(config ClientConfig) (Client, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	switch config.Model {
	case datamodel.Kubernetes:
		config, err := NewKubernetesClientConfig(config.Kubeconfig, config.KubeAPI)
		if err != nil {
			return nil, errors.Errorf("loading kubernetes config: %v", err)
		}

		kubeclient, err := NewKubernetesClient(config)
		if err != nil {
			return nil, errors.Wrap(err, "creating kubernetes hardware client")
		}
		kubeclient.WaitForCacheSync(context.Background())

		return kubeclient, nil

	case datamodel.TinkServer:
		tc, err := tink.TinkHardwareClient()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create the tink client")
		}
		return clientTinkerbell{client: tc}, nil

	default:
		cc, err := cacher.New(config.Facility)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create the cacher client")
		}
		return clientCacher{client: cc}, nil
	}
}
