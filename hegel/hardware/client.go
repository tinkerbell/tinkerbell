package hardware

import (
	"context"

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
}

// Hardware is the interface for Cacher/Tink hardware types.
type Hardware interface {
	Export() ([]byte, error)
	ID() (string, error)
}

// ClientConfig is the configuration used by the NewClient func. Field requirements are based on the value of Model.
type ClientConfig struct {
	// Model defines the client implementation that is constructed.
	// Required.
	Model datamodel.DataModel

	// KubeAPI is the URL of the Kube API the Kubernetes client talks to.
	// Optional
	KubeAPI string

	// Kuberconfig is a path to a Kubeconfig file used by the Kubernetes client.
	// Optional
	Kubeconfig string

	// KubeNamespace is a namespace override to have Hegel use for reading resources.
	// Optional
	KubeNamespace string
}

// NewClient returns a new hardware Client, configured appropriately according to the mode (Cacher or Tink) Hegel is running in.
func NewClient(config ClientConfig) (Client, error) {
	switch config.Model {
	case datamodel.Kubernetes:
		config, err := NewKubernetesClientConfig(config.Kubeconfig, config.KubeAPI, config.KubeNamespace)
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
	}

	return nil, errors.New("unknown data model")
}
