package factory

import (
	"fmt"

	"github.com/tinkerbell/rufio/pkg/bmc/client"
	"github.com/tinkerbell/rufio/pkg/bmc/client/bmclib"
)

const (
	BMCLib = "bmclib"
)

func GetBMCClient(clientType string) (client.BMCClient, error) {
	switch clientType {
	case BMCLib:
		return bmclib.NewBMCLibClient(), nil
	default:
		return nil, fmt.Errorf("unknown bmc client type")
	}
}
