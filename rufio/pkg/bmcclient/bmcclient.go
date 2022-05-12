package bmcclient

import (
	"context"
	"fmt"

	"github.com/bmc-toolbox/bmclib"
)

// BMCClient represents a baseboard management controller client.
// It defines a set of methods to connect and interact with a BMC.
type BMCClient interface {
	OpenConnection(ctx context.Context) error
	CloseConnection(ctx context.Context) error
	GetPowerStatus(ctx context.Context) (string, error)
	SetPowerState(ctx context.Context, state string) error
	SetBootDevice(ctx context.Context, bootDevice string, setPersistent, efiBoot bool) error
}

// BMCClientFactory defines a func that returns a BMCClient
type BMCClientFactory func(hostIP, port, username, password string) BMCClient

// BMCLibClient utilizes bmclib to interact with a baseboard management controller.
type BMCLibClient struct {
	client *bmclib.Client
}

// NewBMCLibClient initializes a bmclib client based on input host and credentials.
// Returns a BMCLibClient
func NewBMCLibClient(hostIP, port, username, password string) BMCClient {
	client := bmclib.NewClient(hostIP, port, username, password)

	// TODO (pokearu): Make an option
	client.Registry.Drivers = client.Registry.PreferDriver("gofish")
	return &BMCLibClient{
		client: client,
	}
}

// OpenConnection establishes a connection with the bmc.
func (bc *BMCLibClient) OpenConnection(ctx context.Context) error {
	err := bc.client.Open(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to BMC: %v", err)
	}

	return nil
}

// CloseConnection ends the connection with the bmc.
func (bc *BMCLibClient) CloseConnection(ctx context.Context) error {
	err := bc.client.Close(ctx)
	if err != nil {
		return fmt.Errorf("failed to close connection to BMC: %v", err)
	}

	return nil
}

// GetPowerStatus fetches the current power status of the bmc.
func (bc *BMCLibClient) GetPowerStatus(ctx context.Context) (string, error) {
	powerState, err := bc.client.GetPowerState(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get power status: %v", err)
	}

	return powerState, nil
}

// SetPowerState power controls the bmc to the input power state.
func (bc *BMCLibClient) SetPowerState(ctx context.Context, state string) error {
	_, err := bc.client.SetPowerState(ctx, state)
	if err != nil {
		return fmt.Errorf("failed to set power state %s: %v", state, err)
	}

	return nil
}

// SetBootDevice sets the boot device on the bmc.
// Currently this sets the first boot device.
// setPersistent, if true will set the boot device permanently. If false, sets one time boot.
// efiBoot, if true passes efiboot options while setting boot device.
func (bc *BMCLibClient) SetBootDevice(ctx context.Context, bootDevice string, setPersistent, efiBoot bool) error {
	_, err := bc.client.SetBootDevice(ctx, bootDevice, setPersistent, efiBoot)
	if err != nil {
		return fmt.Errorf("failed to set boot device %s: %v", bootDevice, err)
	}

	return nil
}
