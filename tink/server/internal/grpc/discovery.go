package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	discoveryPrefix = "discovery-"
)

// Discover will create a Hardware object for an ID if it does not already exist.
// The attrs will be used to populate the Hardware object.
// If the Hardware object already exists, it will not be modified.
// If the Hardware object is created, it will be created in the namespace defined in the AutoDiscovery configuration.
func (h *Handler) Discover(ctx context.Context, agentID string, attrs *data.AgentAttributes) (*v1alpha1.Hardware, error) {
	ns := h.AutoCapabilities.Discovery.Namespace
	hwName, err := makeValidName(agentID, discoveryPrefix)
	if err != nil {
		journal.Log(ctx, "Error making discovery ID a valid Kubernetes name", "error", err)
		return nil, fmt.Errorf("failed to make discovery ID %s a valid Kubernetes name: %w", agentID, err)
	}
	journal.Log(ctx, "Discovering hardware", "agentID", agentID, "hardwareName", hwName, "namespace", ns)

	// Check if Hardware object already exists
	existing, err := h.hardware(ctx, agentID)
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
		Spec: v1alpha1.HardwareSpec{
			AgentID: agentID,
			Auto: v1alpha1.AutoCapabilities{
				EnrollmentEnabled: h.AutoCapabilities.Discovery.EnrollmentEnabled,
			},
		},
	}

	if hw.Annotations == nil {
		hw.Annotations = make(map[string]string)
	}

	if a, err := json.Marshal(attrs); err == nil && attrs != nil {
		hw.Annotations[attributesAnnotation] = string(a)
	}

	// Populate Hardware object with discovered attributes
	updateHardware(ctx, hw, attrs)
	journal.Log(ctx, "Populated hardware object with discovered attributes", "hardware", hw)

	// Create the Hardware object in the cluster
	if err := h.AutoCapabilities.Discovery.CreateHardware(ctx, hw); err != nil {
		journal.Log(ctx, "Error creating hardware object", "error", err)
		return nil, fmt.Errorf("failed to create hardware object %s/%s: %w", ns, hwName, err)
	}

	// If we created the Hardware object we use the h.AutoCapabilities.Discovery.EnrollmentEnabled field to set
	// the enrollmentEnabled field in the Hardware spec. This is the value that gets persisted in the cluster.
	// In order to allow auto enrollment to create Workflows for this Hardware, we set this field to true here.
	// This only sets the field in memory so that auto enrollment will create a Workflow.
	// This enables a one-time auto enrollment for this Hardware object.
	// If the desired behavior is to always have auto enrollment enabled for discovered Hardware,
	// then the user should edit the Hardware object after creation.
	hw.Spec.Auto.EnrollmentEnabled = true

	journal.Log(ctx, "Hardware object created successfully", "hardwareName", hwName, "namespace", ns)
	return hw, nil
}

func updateHardware(ctx context.Context, hw *v1alpha1.Hardware, attrs *data.AgentAttributes) {
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
				// validate MAC address format
				if _, err := net.ParseMAC(*iface.Mac); err != nil {
					journal.Log(ctx, "Invalid MAC address format", "mac", *iface.Mac)
					continue
				}
				hw.Spec.Interfaces = append(hw.Spec.Interfaces, v1alpha1.Interface{
					DHCP: &v1alpha1.DHCP{
						MAC: *iface.Mac,
					},
				})
			}
		}
	}
}
