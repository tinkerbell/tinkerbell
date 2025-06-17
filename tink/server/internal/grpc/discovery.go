package grpc

import (
	"context"
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Discover will create a Hardware object for an ID if it does not already exist.
// The attrs will be used to populate the Hardware object.
// If the Hardware object already exists, it will not be modified.
// If the Hardware object is created, it will be created in the namespace defined in the AutoDiscovery configuration.
func (h *Handler) Discover(ctx context.Context, id string, attrs *data.AgentAttributes) (*v1alpha1.Hardware, error) {
	ns := h.AutoCapabilities.Discovery.Namespace
	hwName := fmt.Sprintf("discovery-%s", id)
	journal.Log(ctx, "Discovering hardware", "id", id, "hardwareName", hwName, "namespace", ns)

	// Check if Hardware object already exists
	existing, err := h.AutoCapabilities.Discovery.ReadHardware(ctx, hwName, ns)
	if err == nil {
		// Hardware object already exists, do not modify
		journal.Log(ctx, "Hardware object already exists, skipping creation")
		return existing, nil
	}

	if !apierrors.IsNotFound(err) {
		// Unexpected error occurred while checking for existing hardware
		journal.Log(ctx, "Error checking for existing hardware object", "error", err)
		return nil, fmt.Errorf("failed to check for existing hardware object %s/%s: %w", ns, hwName, err)
	}

	// Hardware object does not exist, create it
	journal.Log(ctx, "Hardware object does not exist, creating new one")

	// Create new Hardware object with basic metadata
	hw := &v1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hwName,
			Namespace: ns,
			Labels: map[string]string{
				"tinkerbell.org/auto-discovered": "true",
			},
		},
		Spec: v1alpha1.HardwareSpec{},
	}

	if hw.Annotations == nil {
		hw.Annotations = make(map[string]string)
	}

	if a, err := json.Marshal(attrs); err == nil && attrs != nil {
		hw.Annotations[attributesAnnotation] = string(a)
	}

	// Populate Hardware object with discovered attributes
	updateHardware(hw, attrs)
	journal.Log(ctx, "Populated hardware object with discovered attributes", "hardware", hw)

	// Create the Hardware object in the cluster
	if err := h.AutoCapabilities.Discovery.CreateHardware(ctx, hw); err != nil {
		journal.Log(ctx, "Error creating hardware object", "error", err)
		return nil, fmt.Errorf("failed to create hardware object %s/%s: %w", ns, hwName, err)
	}

	journal.Log(ctx, "Hardware object created successfully", "hardwareName", hwName, "namespace", ns)
	return hw, nil
}

func updateHardware(hw *v1alpha1.Hardware, attrs *data.AgentAttributes) {
	if hw == nil || attrs == nil {
		return
	}

	// Add disks if they exist in the attributes
	for _, disk := range attrs.BlockDevices {
		if disk != nil {
			if disk.Size != nil && *disk.Size != "" {
				hw.Spec.Disks = append(hw.Spec.Disks, v1alpha1.Disk{
					Device: fmt.Sprintf("/dev/%s", *disk.Name),
				})
			}
		}
	}

	// Add network interfaces if they exist in the attributes
	for _, iface := range attrs.NetworkInterfaces {
		if iface != nil {
			if iface.Mac != nil && *iface.Mac != "" {
				hw.Spec.Interfaces = append(hw.Spec.Interfaces, v1alpha1.Interface{
					DHCP: &v1alpha1.DHCP{
						MAC: *iface.Mac,
					},
				})
			}
		}
	}
}
