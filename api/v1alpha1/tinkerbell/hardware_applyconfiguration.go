package tinkerbell

// HardwareApplyConfiguration is a minimal hand-written implementation of
// runtime.ApplyConfiguration for Hardware, sufficient for the Server-Side
// Apply patches to status.attributes.inBand (tink-server) and
// status.attributes.outOfBand (Rufio). controller-runtime's client.Apply /
// SubResourceWriter.Apply want an object satisfying an internal
// "applyConfiguration" interface (GetName/GetNamespace/GetKind/GetAPIVersion,
// all returning *string) - the shape client-gen's apply-configuration-gen
// produces for built-in types (e.g. corev1.SecretApplyConfiguration). This
// repo has no such codegen wired up for its custom CRDs, and generating a
// full one for Hardware is out of scope for the single field each writer
// owns, so this is hand-written instead and shared by both writers.
//
// +kubebuilder:object:generate=false
type HardwareApplyConfiguration struct {
	Kind       *string                           `json:"kind,omitempty"`
	APIVersion *string                           `json:"apiVersion,omitempty"`
	Metadata   HardwareApplyMetadata             `json:"metadata,omitempty"`
	Status     *HardwareStatusApplyConfiguration `json:"status,omitempty"`
}

// +kubebuilder:object:generate=false
type HardwareApplyMetadata struct {
	Name      *string `json:"name,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
}

// HardwareStatusApplyConfiguration only carries the one path a given writer
// owns. status.attributes is modeled one level deeper than the leaf so the
// applied patch names only that leaf: SSA merges maps by key, so the sibling
// subtree owned by the other writer's field manager stays untouched rather
// than being pruned by this apply.
//
// +kubebuilder:object:generate=false
type HardwareStatusApplyConfiguration struct {
	Attributes *HardwareAttributesApplyConfiguration `json:"attributes,omitempty"`
}

// HardwareAttributesApplyConfiguration carries only the leaf a given writer
// sets - tink-server sets InBand, Rufio sets OutOfBand, never both in the
// same apply. InBand/OutOfBand are the concrete API type (not a further
// apply-configuration wrapper): each writer always applies its whole
// sub-object atomically, never a partial deep-merge within it.
//
// +kubebuilder:object:generate=false
type HardwareAttributesApplyConfiguration struct {
	InBand    *Attributes `json:"inBand,omitempty"`
	OutOfBand *Attributes `json:"outOfBand,omitempty"`
}

func (h *HardwareApplyConfiguration) IsApplyConfiguration()  {}
func (h *HardwareApplyConfiguration) GetKind() *string       { return h.Kind }
func (h *HardwareApplyConfiguration) GetAPIVersion() *string { return h.APIVersion }
func (h *HardwareApplyConfiguration) GetName() *string       { return h.Metadata.Name }
func (h *HardwareApplyConfiguration) GetNamespace() *string  { return h.Metadata.Namespace }
