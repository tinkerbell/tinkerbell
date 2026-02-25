// Package backend provides tootles-specific backend logic for converting Hardware resources
// into EC2 and Hack instance metadata formats.
package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/tootles/internal/frontend/ec2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const tracerName = "github.com/tinkerbell/tinkerbell"

// HardwareReader is the interface required to read Hardware objects.
type HardwareReader interface {
	ReadHardware(ctx context.Context, id, namespace string, opts data.ReadListOptions) (*v1alpha1.Hardware, error)
}

// Backend provides tootles instance metadata by reading Hardware via a HardwareReader.
type Backend struct {
	reader HardwareReader
}

// New creates a new Backend wrapping the given HardwareReader.
func New(reader HardwareReader) *Backend {
	return &Backend{reader: reader}
}

// GetHackInstance returns a HackInstance for the hardware associated with the given IP.
func (b *Backend) GetHackInstance(ctx context.Context, ip string) (data.HackInstance, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "tootles.backend.GetHackInstance")
	defer span.End()

	hw, err := b.reader.ReadHardware(ctx, "", "", data.ReadListOptions{
		Hardware: data.HardwareReadOptions{ByIPAddress: ip},
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return data.HackInstance{}, err
	}

	span.SetStatus(codes.Ok, "")

	return toHackInstance(*hw)
}

// GetEC2Instance returns an Ec2Instance for the hardware associated with the given IP.
func (b *Backend) GetEC2Instance(ctx context.Context, ip string) (data.Ec2Instance, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "tootles.backend.GetEC2Instance")
	defer span.End()

	hw, err := b.reader.ReadHardware(ctx, "", "", data.ReadListOptions{
		Hardware: data.HardwareReadOptions{ByIPAddress: ip},
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return data.Ec2Instance{}, toInstanceNotFoundErr(err)
	}

	span.SetStatus(codes.Ok, "")

	return toEC2Instance(*hw), nil
}

// GetEC2InstanceByInstanceID returns an Ec2Instance for the hardware with the given instance ID.
func (b *Backend) GetEC2InstanceByInstanceID(ctx context.Context, instanceID string) (data.Ec2Instance, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "tootles.backend.GetEC2InstanceByInstanceID")
	defer span.End()

	hw, err := b.reader.ReadHardware(ctx, "", "", data.ReadListOptions{
		Hardware: data.HardwareReadOptions{ByInstanceID: instanceID},
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return data.Ec2Instance{}, toInstanceNotFoundErr(err)
	}

	span.SetStatus(codes.Ok, "")

	return toEC2Instance(*hw), nil
}

// toHackInstance converts a Tinkerbell Hardware resource to a HackInstance by marshalling and
// unmarshalling. This works because the Hardware resource has historical roots that align with
// the HackInstance struct that is derived from the rootio action.
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

func toEC2Instance(hw v1alpha1.Hardware) data.Ec2Instance {
	var i data.Ec2Instance

	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Instance != nil {
		i.Metadata.InstanceID = hw.Spec.Metadata.Instance.ID
		i.Metadata.Hostname = hw.Spec.Metadata.Instance.Hostname
		i.Metadata.LocalHostname = hw.Spec.Metadata.Instance.Hostname
		i.Metadata.Tags = hw.Spec.Metadata.Instance.Tags

		if hw.Spec.Metadata.Instance.OperatingSystem != nil {
			i.Metadata.OperatingSystem.Slug = hw.Spec.Metadata.Instance.OperatingSystem.Slug
			i.Metadata.OperatingSystem.Distro = hw.Spec.Metadata.Instance.OperatingSystem.Distro
			i.Metadata.OperatingSystem.Version = hw.Spec.Metadata.Instance.OperatingSystem.Version
			i.Metadata.OperatingSystem.ImageTag = hw.Spec.Metadata.Instance.OperatingSystem.ImageTag
		}

		// Iterate over all IPs and set the first one for IPv4 and IPv6 as the values in the
		// instance metadata.
		for _, ip := range hw.Spec.Metadata.Instance.Ips {
			// Public IPv4
			if ip.Family == 4 && ip.Public && i.Metadata.PublicIPv4 == "" {
				i.Metadata.PublicIPv4 = ip.Address
			}

			// Private IPv4
			if ip.Family == 4 && !ip.Public && i.Metadata.LocalIPv4 == "" {
				i.Metadata.LocalIPv4 = ip.Address
			}

			// Public IPv6
			if ip.Family == 6 && i.Metadata.PublicIPv6 == "" {
				i.Metadata.PublicIPv6 = ip.Address
			}
		}
	}

	if hw.Spec.Metadata != nil && hw.Spec.Metadata.Facility != nil {
		i.Metadata.Plan = hw.Spec.Metadata.Facility.PlanSlug
		i.Metadata.Facility = hw.Spec.Metadata.Facility.FacilityCode
	}

	if hw.Spec.UserData != nil {
		i.Userdata = *hw.Spec.UserData
	}

	return i
}

// notFounder is implemented by errors that indicate a resource was not found.
type notFounder interface {
	NotFound() bool
}

// toInstanceNotFoundErr checks if err indicates a not-found condition and, if so,
// wraps it as ec2.ErrInstanceNotFound so the frontend handles it correctly.
func toInstanceNotFoundErr(err error) error {
	if isNotFound(err) {
		return fmt.Errorf("%w: %w", ec2.ErrInstanceNotFound, err)
	}
	return err
}

func isNotFound(err error) bool {
	var nf notFounder
	return err != nil && errors.As(err, &nf) && nf.NotFound()
}
