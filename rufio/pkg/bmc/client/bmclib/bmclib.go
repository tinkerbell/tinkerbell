package bmclib

import (
	"context"
	"fmt"

	"github.com/bmc-toolbox/bmclib"

	"github.com/tinkerbell/rufio/pkg/bmc/client"
)

// BMCLibClient utilizes bmclib to interact with a baseboard management controller.
type BMCLibClient struct {
	client *bmclib.Client
}

func NewBMCLibClient() client.BMCClient {
	return &BMCLibClient{}
}

// InitClient is used to initilize a bmclib client based on input host and credentials.
func (bc *BMCLibClient) InitClient(hostIP, port, username, password string) {
	client := bmclib.NewClient(hostIP, port, username, password)

	// TODO (pokearu): Make an option
	client.Registry.Drivers = client.Registry.PreferDriver("gofish")
	bc.client = client
}

// OpenConnection establishes a connection with the bmc.
func (bc *BMCLibClient) OpenConnection(ctx context.Context) error {
	if bc.client == nil {
		return fmt.Errorf("bmclib client not initialized")
	}

	err := bc.client.Open(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to BMC: %v", err)
	}

	return nil
}

// CloseConnection ends the connection with the bmc.
func (bc *BMCLibClient) CloseConnection(ctx context.Context) error {
	if bc.client == nil {
		return fmt.Errorf("bmclib client not initialized")
	}

	err := bc.client.Close(ctx)
	if err != nil {
		return fmt.Errorf("failed to close connection to BMC: %v", err)
	}

	return nil
}

// GetPowerStatus fetches the current power status of the bmc.
func (bc *BMCLibClient) GetPowerStatus(ctx context.Context) (string, error) {
	if bc.client == nil {
		return "", fmt.Errorf("bmclib client not initialized")
	}

	powerState, err := bc.client.GetPowerState(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get power status: %v", err)
	}

	return powerState, nil
}

// SetPowerState power controls the bmc to the input power state.
func (bc *BMCLibClient) SetPowerState(ctx context.Context, state string) error {
	if bc.client == nil {
		return fmt.Errorf("bmclib client not initialized")
	}

	_, err := bc.client.SetPowerState(ctx, state)
	if err != nil {
		return fmt.Errorf("failed to set power state %s: %v", state, err)
	}

	return nil
}

// SetFirstBootDevice sets the first boot device on the bmc.
// setPersistent, if true will set the boot device permanently. If false, sets one time boot.
// efiBoot, if true passes efiboot options while setting boot device.
func (bc *BMCLibClient) SetFirstBootDevice(ctx context.Context, bootDevice string, setPersistent, efiBoot bool) error {
	if bc.client == nil {
		return fmt.Errorf("bmclib client not initialized")
	}

	_, err := bc.client.SetBootDevice(ctx, bootDevice, setPersistent, efiBoot)
	if err != nil {
		return fmt.Errorf("failed to set boot device %s: %v", bootDevice, err)
	}

	return nil
}
