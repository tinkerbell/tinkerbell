package controllers

import (
	"context"
	"fmt"
	"time"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/bmclib/v2/bmc"
	"github.com/go-logr/logr"
)

// BMCClient represents a baseboard management controller client. It defines a set of methods to
// connect and interact with a BMC.
type BMCClient interface {
	// Close ends the connection with the bmc.
	Close(ctx context.Context) error

	// GetPowerState fetches the current power status of the bmc.
	GetPowerState(ctx context.Context) (string, error)

	// SetPowerState power controls the bmc to the input power state.
	SetPowerState(ctx context.Context, state string) (bool, error)

	// SetBootDevice sets the boot device on the bmc. Currently this sets the first boot device.
	// setPersistent, if true will set the boot device permanently. If false, sets one time boot.
	// efiBoot, if true passes efiboot options while setting boot device.
	SetBootDevice(ctx context.Context, bootDevice string, setPersistent, efiBoot bool) (bool, error)

	// SetVirtualMedia ejects existing virtual media and then if mediaUrl isn't empty, instructs
	// the bmc to download virtual media of the specified kind from mediaUrl. Returns true on success.
	SetVirtualMedia(ctx context.Context, kind, mediaUrl string) (bool, error)
	// GetMetadata returns the metadata of the bmc client.
	GetMetadata() bmc.Metadata
}

// BMCClientFactoryFunc defines a func that returns a BMCClient
type BMCClientFactoryFunc func(ctx context.Context, log logr.Logger, hostIP, port, username, password string) (BMCClient, error)

// NewBMCClientFactoryFunc returns a new BMCClientFactoryFunc. The timeout parameter determines the
// maximum time to probe for compatible interfaces.
func NewBMCClientFactoryFunc(timeout time.Duration) BMCClientFactoryFunc {
	// Initializes a bmclib client based on input host and credentials
	// Establishes a connection with the bmc with client.Open
	// Returns a BMCClient
	return func(ctx context.Context, log logr.Logger, hostIP, port, username, password string) (BMCClient, error) {
		client := bmclib.NewClient(hostIP, port, username, password)
		log = log.WithValues("host", hostIP, "port", port, "username", username)

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// TODO (pokearu): Make an option
		client.Registry.Drivers = client.Registry.PreferDriver("gofish")
		if err := client.Open(ctx); err != nil {
			md := client.GetMetadata()
			log.Info("Failed to open connection to BMC", "error", err, "providersAttempted", md.ProvidersAttempted, "successfulProvider", md.SuccessfulOpenConns)
			return nil, fmt.Errorf("failed to open connection to BMC: %v", err)
		}
		md := client.GetMetadata()
		log.Info("Connected to BMC", "providersAttempted", md.ProvidersAttempted, "successfulProvider", md.SuccessfulOpenConns)

		return client, nil
	}
}
