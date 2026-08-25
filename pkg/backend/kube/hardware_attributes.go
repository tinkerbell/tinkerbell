package kube

import (
	"context"
	"fmt"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// inBandFieldOwner is the Server-Side Apply field manager for
// status.attributes.inBand, distinct from Rufio's "machine-controller" field
// manager for the sibling status.attributes.outOfBand subtree.
const inBandFieldOwner = "tink-server"

// ApplyHardwareInBandAttributes issues a Server-Side Apply patch for
// status.attributes.inBand, scoped to only that path so it never conflicts
// with Rufio's writes to the sibling status.attributes.outOfBand subtree.
func (b *Backend) ApplyHardwareInBandAttributes(ctx context.Context, name, namespace string, attrs *v1alpha1.Attributes) error {
	apiVersion := v1alpha1.GroupVersion.String()
	kind := "Hardware"
	applyConfig := &v1alpha1.HardwareApplyConfiguration{
		Kind:       &kind,
		APIVersion: &apiVersion,
		Metadata: v1alpha1.HardwareApplyMetadata{
			Name:      &name,
			Namespace: &namespace,
		},
		Status: &v1alpha1.HardwareStatusApplyConfiguration{
			Attributes: &v1alpha1.HardwareAttributesApplyConfiguration{
				InBand: attrs,
			},
		},
	}
	if err := b.cluster.GetClient().Status().Apply(ctx, applyConfig,
		client.FieldOwner(inBandFieldOwner),
		client.ForceOwnership,
	); err != nil {
		return fmt.Errorf("failed to apply hardware %s/%s status.attributes.inBand: %w", namespace, name, err)
	}

	return nil
}
