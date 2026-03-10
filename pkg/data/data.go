package data

// ReadListOptions holds all the parameters that can be used to query an object.
// This lives here so that it can be shared between interfaces and implementations.
type ReadListOptions struct {
	ByName      string
	ByAgentID   string
	InNamespace string
	Hardware    HardwareReadOptions
}

// HardwareReadOptions holds all the parameters that can be used to query Hardware objects.
type HardwareReadOptions struct {
	ByMACAddress string
	ByIPAddress  string
	ByInstanceID string
}

// UpdateOptions holds all the parameters that can be used to update an object.
type UpdateOptions struct {
	StatusOnly bool

	// PatchFrom, when non-nil, signals that the backend should compute a merge-patch
	// between this original object and the modified object passed to the Update call.
	// The caller is expected to pass a DeepCopy taken before any mutations.
	// The concrete type must be compatible with the backend (e.g. client.Object for the kube backend).
	PatchFrom any

	// RawPatch, when non-nil, signals that the backend should apply a raw patch.
	RawPatch []byte

	// RawPatchType specifies the patch strategy. Supported Kubernetes patch types:
	//   - "application/json-patch+json"            (JSON Patch, RFC 6902: array of {op, path, value} operations)
	//   - "application/merge-patch+json"           (JSON Merge Patch, RFC 7386: partial JSON merged into the object)
	//   - "application/strategic-merge-patch+json" (Strategic Merge Patch: Kubernetes-specific, merges arrays by key)
	//   - "application/apply-patch+yaml"           (Server-Side Apply: field ownership tracking, requires fieldManager)
	RawPatchType string
}
