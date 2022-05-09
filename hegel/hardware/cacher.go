package hardware

import (
	"context"
	"encoding/json"

	"github.com/packethost/cacher/protos/cacher"
	"github.com/pkg/errors"
	"github.com/tinkerbell/hegel/datamodel"
)

type clientCacher struct {
	client cacher.CacherClient
}

type Cacher struct {
	*cacher.Hardware
}

type watcherCacher struct {
	client cacher.Cacher_WatchClient
}

// NewCacherClient returns a new hardware Client, configured to use a provided cacher Client
// This function is primarily used for testing.
func NewCacherClient(cc cacher.CacherClient, model datamodel.DataModel) (Client, error) {
	if model != "" {
		return nil, errors.New("NewCacherClient is only valid for the cacher data model")
	}

	return clientCacher{client: cc}, nil
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

// Recv receives a piece of hardware from the Cacher watch client.
func (w *watcherCacher) Recv() (Hardware, error) {
	hw, err := w.client.Recv()
	if err != nil {
		return nil, err
	}
	return &Cacher{hw}, nil
}
