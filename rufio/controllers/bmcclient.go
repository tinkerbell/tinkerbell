package controllers

import (
	"context"
	"fmt"

	"github.com/bmc-toolbox/bmclib"
)

// NewBMCClientFactoryFunc returns a new BMCClientFactoryFunc
func NewBMCClientFactoryFunc(ctx context.Context) BMCClientFactoryFunc {
	// Initializes a bmclib client based on input host and credentials
	// Establishes a connection with the bmc with client.Open
	// Returns a BMCClient
	return func(ctx context.Context, hostIP, port, username, password string) (BMCClient, error) {
		client := bmclib.NewClient(hostIP, port, username, password)

		// TODO (pokearu): Make an option
		client.Registry.Drivers = client.Registry.PreferDriver("gofish")
		if err := client.Open(ctx); err != nil {
			return nil, fmt.Errorf("failed to open connection to BMC: %v", err)
		}
		return client, nil
	}
}
