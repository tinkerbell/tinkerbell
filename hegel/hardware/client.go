package hardware

import (
	"context"
	"encoding/json"

	cacherClient "github.com/packethost/cacher/client"
	"github.com/packethost/cacher/protos/cacher"
	"github.com/pkg/errors"
	"github.com/tinkerbell/hegel/datamodel"
	tinkClient "github.com/tinkerbell/tink/client"
	tpkg "github.com/tinkerbell/tink/pkg"
	tink "github.com/tinkerbell/tink/protos/hardware"
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

type clientCacher struct {
	client cacher.CacherClient
}

type clientTinkerbell struct {
	client tink.HardwareServiceClient
}

type Cacher struct {
	*cacher.Hardware
}

type Tinkerbell struct {
	*tink.Hardware
}

type watcherCacher struct {
	client cacher.Cacher_WatchClient
}

type watcherTinkerbell struct {
	client tink.HardwareService_DeprecatedWatchClient
}

// NewCacherClient returns a new hardware Client, configured to use a provided cacher Client
// This function is primarily used for testing.
func NewCacherClient(cc cacher.CacherClient, model datamodel.DataModel) (Client, error) {
	if model != "" {
		return nil, errors.New("NewCacherClient is only valid for the cacher data model")
	}

	return clientCacher{client: cc}, nil
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

	if v.Model == datamodel.Kubernetes {
		if v.KubeAPI == "" {
			return errors.New("kubernetes data model: kube api url is required")
		}

		if v.Kubeconfig == "" {
			return errors.New("kubernetes data model: kubeconfig path is required")
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
		tc, err := tinkClient.TinkHardwareClient()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create the tink client")
		}
		return clientTinkerbell{client: tc}, nil

	default:
		cc, err := cacherClient.New(config.Facility)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create the cacher client")
		}
		return clientCacher{client: cc}, nil
	}
}

func (hg clientCacher) IsHealthy(ctx context.Context) bool {
	_, err := hg.client.All(ctx, &cacher.Empty{})
	return err == nil
}

// ByIP retrieves from Cacher the piece of hardware with the specified IP.
func (hg clientCacher) ByIP(ctx context.Context, ip string) (Hardware, error) {
	in := &cacher.GetRequest{
		IP: ip,
	}
	hw, err := hg.client.ByIP(ctx, in)
	if err != nil {
		return nil, err
	}
	return &Cacher{hw}, nil
}

// Watch returns a Cacher watch client on the hardware with the specified ID.
func (hg clientCacher) Watch(ctx context.Context, id string) (Watcher, error) {
	in := &cacher.GetRequest{
		ID: id,
	}
	w, err := hg.client.Watch(ctx, in)
	if err != nil {
		return nil, err
	}
	return &watcherCacher{w}, nil
}

// All retrieves all the pieces of hardware stored in Cacher.
func (hg clientTinkerbell) IsHealthy(ctx context.Context) bool {
	_, err := hg.client.All(ctx, &tink.Empty{})
	return err == nil
}

// ByIP retrieves from Tink the piece of hardware with the specified IP.
func (hg clientTinkerbell) ByIP(ctx context.Context, ip string) (Hardware, error) {
	in := &tink.GetRequest{
		Ip: ip,
	}
	hw, err := hg.client.ByIP(ctx, in)
	if err != nil {
		return nil, err
	}
	return &Tinkerbell{hw}, nil
}

// Watch returns a Tink watch client on the hardware with the specified ID.
func (hg clientTinkerbell) Watch(ctx context.Context, id string) (Watcher, error) {
	in := &tink.GetRequest{
		Id: id,
	}
	w, err := hg.client.DeprecatedWatch(ctx, in)
	if err != nil {
		return nil, err
	}
	return &watcherTinkerbell{w}, nil
}

// Export formats the piece of hardware to be returned in responses to clients.
func (hw *Cacher) Export() ([]byte, error) {
	exported := &ExportedCacher{}

	err := json.Unmarshal([]byte(hw.JSON), exported)
	if err != nil {
		return nil, err
	}
	return json.Marshal(exported)
}

// ID returns the hardware ID.
func (hw *Cacher) ID() (string, error) {
	hwJSON := make(map[string]interface{})
	err := json.Unmarshal([]byte(hw.JSON), &hwJSON)
	if err != nil {
		return "", err
	}

	hwID := hwJSON["id"]
	id, ok := hwID.(string)
	if !ok {
		return "", errors.Errorf("hwID is %T, not a string", hwID)
	}

	return id, err
}

// Export formats the piece of hardware to be returned in responses to clients.
func (hw *Tinkerbell) Export() ([]byte, error) {
	return json.Marshal(tpkg.HardwareWrapper(*hw))
}

// ID returns the hardware ID.
func (hw *Tinkerbell) ID() (string, error) {
	return hw.Id, nil
}

// Recv receives a piece of hardware from the Cacher watch client.
func (w *watcherCacher) Recv() (Hardware, error) {
	hw, err := w.client.Recv()
	if err != nil {
		return nil, err
	}
	return &Cacher{hw}, nil
}

// Recv receives a piece of hardware from the Tink watch client.
func (w *watcherTinkerbell) Recv() (Hardware, error) {
	hw, err := w.client.Recv()
	if err != nil {
		return nil, err
	}
	return &Tinkerbell{hw}, nil
}
