package kube

import (
	"context"
	"encoding/json"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1"
	"github.com/tinkerbell/tinkerbell/data"
)

// GetHackInstance returns a hack.Instance by calling the getByIP method and converting the result.
// This is a method that the Hegel service uses.
func (b *Backend) GetHackInstance(ctx context.Context, ip string) (data.HackInstance, error) {
	hw, err := b.hwByIP(ctx, ip)
	if err != nil {
		return data.HackInstance{}, err
	}

	return toHackInstance(*hw)
}

// toHackInstance converts a Tinkerbell Hardware resource to a hack.Instance by marshalling and
// unmarshalling. This works because the Hardware resource has historical roots that align with
// the hack.Instance struct that is derived from the rootio action. See the hack frontend for more
// details.
func toHackInstance(hw v1alpha1.Hardware) (data.HackInstance, error) {
	marshalled, err := json.Marshal(hw.Spec)
	if err != nil {
		return data.HackInstance{}, err
	}

	var i data.HackInstance
	if err := json.Unmarshal(marshalled, &i); err != nil {
		return data.HackInstance{}, err
	}

	return i, nil
}
