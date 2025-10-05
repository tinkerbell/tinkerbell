package noop

import (
	"context"
	"errors"
	"net"

	"github.com/tinkerbell/tinkerbell/pkg/data"
)

var errAlways = errors.New("noop backend always returns an error")

type Backend struct{}

func (n Backend) GetByMac(context.Context, net.HardwareAddr) (data.Hardware, error) {
	return data.Hardware{}, errAlways
}

func (n Backend) GetByIP(context.Context, net.IP) (data.Hardware, error) {
	return data.Hardware{}, errAlways
}

// GetHackInstance exists to satisfy the hack.Client interface. It is not implemented.
func (n Backend) GetHackInstance(context.Context, string) (data.HackInstance, error) {
	return data.HackInstance{}, errAlways
}

// GetEC2Instance exists to satisfy the ec2.Client interface. It is not implemented.
func (n Backend) GetEC2Instance(context.Context, string) (data.Ec2Instance, error) {
	return data.Ec2Instance{}, errAlways
}
