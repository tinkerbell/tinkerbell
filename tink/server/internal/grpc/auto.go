package grpc

type AutoCapabilities struct {
	Enrollment AutoEnrollment
	Discovery  AutoDiscovery
}

// AutoEnrollment is a struct that contains the auto enrollment configuration.
// Auto Enrollment is defined as automatically running a Workflow for an Agent that
// does not have a Workflow assigned to it. The Agent may or may not have a Hardware
// Object defined.
type AutoEnrollment struct {
	Enabled bool

	WorkflowRuleSetLister
	WorkflowCreator
}

// AutoDiscovery is a struct that contains the auto discovery configuration.
// Auto Discovery is defined as automatically creating a Hardware Object for an
// Agent that does not have a Workflow or a Hardware Object assigned to it.
// The Namespace defines the namespace to use when creating the Hardware Object.
// An empty namespace will cause all Hardware Objects to be created in the same
// namespace as the Tink Server.
type AutoDiscovery struct {
	// Enabled defines whether auto discovery is enabled.
	Enabled bool
	// Namespace defines the namespace to use when creating the Hardware Object.
	Namespace string
	// This sets the value of the tinkerbell.Hardware.Spec.Auto.EnrollmentEnabled field.
	// If this is true, then auto enrollment will create Workflows for this Hardware.
	EnrollmentEnabled bool

	HardwareCreator
	HardwareReader
}
