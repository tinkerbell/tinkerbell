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
}
