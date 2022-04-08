package hardware

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"

	cacherClient "github.com/packethost/cacher/client"
	"github.com/packethost/cacher/protos/cacher"
	"github.com/packethost/pkg/env"
	"github.com/pkg/errors"
	tinkClient "github.com/tinkerbell/tink/client"
	tpkg "github.com/tinkerbell/tink/pkg"
	tink "github.com/tinkerbell/tink/protos/hardware"
)

var (
	dataModelVersion = env.Get("DATA_MODEL_VERSION")
	facility         = flag.String("facility", env.Get("HEGEL_FACILITY", "onprem"), "The facility we are running in (mostly to connect to cacher)")
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
func NewCacherClient(cc cacher.CacherClient) (Client, error) {
	var hg Client

	switch dataModelVersion {
	case "1":
		return nil, errors.New("NewCacherClient is only valid for the cacher data Model")
	default:
		hg = clientCacher{client: cc}
	}

	return hg, nil
}

// NewClient returns a new hardware Client, configured appropriately according to the mode (Cacher or Tink) Hegel is running in.
func NewClient() (Client, error) {
	var hg Client

	switch dataModelVersion {
	case "1":
		tc, err := tinkClient.TinkHardwareClient()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create the tink client")
		}
		hg = clientTinkerbell{client: tc}
	default:
		cc, err := cacherClient.New(*facility)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create the cacher client")
		}
		hg = clientCacher{client: cc}
	}

	return hg, nil
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
		return "", fmt.Errorf("hwID is %T, not a string", hwID)
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
