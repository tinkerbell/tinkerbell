package client

import "context"

// BMCClient represents a baseboard management controller client.
// It defines a set of methods to connect and interact with a BMC.
type BMCClient interface {
	InitClient(hostIP, port, username, password string)
	OpenConnection(ctx context.Context) error
	CloseConnection(ctx context.Context) error
	GetPowerStatus(ctx context.Context) (string, error)
	SetPowerState(ctx context.Context, state string) error
	SetFirstBootDevice(ctx context.Context, bootDevice string, setPersistent, efiBoot bool) error
}
