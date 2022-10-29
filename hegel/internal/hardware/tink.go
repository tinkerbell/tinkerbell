package hardware

import (
	"context"
	"encoding/json"

	tinkpkg "github.com/tinkerbell/tink/pkg"
	"github.com/tinkerbell/tink/protos/hardware"
)

type Tinkerbell struct {
	*hardware.Hardware
}

type clientTinkerbell struct {
	client hardware.HardwareServiceClient
}

// All retrieves all the pieces of hardware stored in Cacher.
func (hg clientTinkerbell) IsHealthy(ctx context.Context) bool {
	_, err := hg.client.All(ctx, &hardware.Empty{})
	return err == nil
}

// ByIP retrieves from Tink the piece of hardware with the specified IP.
func (hg clientTinkerbell) ByIP(ctx context.Context, ip string) (Hardware, error) {
	in := &hardware.GetRequest{
		Ip: ip,
	}
	hw, err := hg.client.ByIP(ctx, in)
	if err != nil {
		return nil, err
	}
	return &Tinkerbell{hw}, nil
}

// Export formats the piece of hardware to be returned in responses to clients.
func (hw *Tinkerbell) Export() ([]byte, error) {
	return json.Marshal(tinkpkg.HardwareWrapper(*hw))
}

// ID returns the hardware ID.
func (hw *Tinkerbell) ID() (string, error) {
	return hw.Id, nil
}
