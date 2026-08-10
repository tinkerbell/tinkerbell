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
	applyConfig := &hardwareStatusApplyConfiguration{
		Kind:       &kind,
		APIVersion: &apiVersion,
		Metadata: hardwareApplyMetadata{
			Name:      &name,
			Namespace: &namespace,
		},
		Status: &hardwareStatusApplyConfigurationStatus{
			Attributes: &hardwareAttributesApplyConfiguration{
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

// hardwareStatusApplyConfiguration is a minimal hand-written implementation of
// runtime.ApplyConfiguration for Hardware, sufficient for the Server-Side
// Apply patch to status.attributes.inBand above. Mirrors the equivalent type
// rufio/internal/controller/inventory.go defines for status.attributes.outOfBand:
// this repo has no apply-configuration-gen wired up for its custom CRDs, so a
// generated type isn't available for the single field each writer owns.
type hardwareStatusApplyConfiguration struct {
	Kind       *string                                 `json:"kind,omitempty"`
	APIVersion *string                                 `json:"apiVersion,omitempty"`
	Metadata   hardwareApplyMetadata                   `json:"metadata,omitempty"`
	Status     *hardwareStatusApplyConfigurationStatus `json:"status,omitempty"`
}

type hardwareApplyMetadata struct {
	Name      *string `json:"name,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
}

// hardwareStatusApplyConfigurationStatus only carries the one path this writer
// owns. status.attributes is modeled one level deeper than the leaf so the
// applied patch names only inBand: SSA merges maps by key, so the sibling
// outOfBand subtree (owned by Rufio's "machine-controller" field manager)
// stays untouched rather than being pruned by this apply.
type hardwareStatusApplyConfigurationStatus struct {
	Attributes *hardwareAttributesApplyConfiguration `json:"attributes,omitempty"`
}

// hardwareAttributesApplyConfiguration carries only the in-band subtree.
// InBand is the concrete API type (not a further apply-configuration
// wrapper): this writer always applies the whole sub-object atomically, never
// a partial deep-merge within it.
type hardwareAttributesApplyConfiguration struct {
	InBand *v1alpha1.Attributes `json:"inBand,omitempty"`
}

func (h *hardwareStatusApplyConfiguration) IsApplyConfiguration()  {}
func (h *hardwareStatusApplyConfiguration) GetKind() *string       { return h.Kind }
func (h *hardwareStatusApplyConfiguration) GetAPIVersion() *string { return h.APIVersion }
func (h *hardwareStatusApplyConfiguration) GetName() *string       { return h.Metadata.Name }
func (h *hardwareStatusApplyConfiguration) GetNamespace() *string  { return h.Metadata.Namespace }
