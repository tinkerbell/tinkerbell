package data

// ReadListOptions holds all the parameters that can be used to query an object.
// This lives here so that it can be shared between interfaces and implementations.
type ReadListOptions struct {
	ByName      string
	ByAgentID   string
	InNamespace string
	Hardware    HardwareReadOptions
}

type HardwareReadOptions struct {
	ByMACAddress string
	ByIPAddress  string
	ByInstanceID string
}

type UpdateOptions struct {
	StatusOnly bool
}
