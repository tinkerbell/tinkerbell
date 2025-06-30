package grpc

import (
	"context"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

type AutoEnrollmentReadCreator interface {
	WorkflowRuleSetReader
	WorkflowCreator
}

type AutoDiscoveryReadCreator interface {
	HardwareReader
	HardwareCreator
}

type WorkflowRuleSetReader interface {
	ReadWorkflowRuleSets(ctx context.Context) ([]tinkerbell.WorkflowRuleSet, error)
}

type WorkflowCreator interface {
	CreateWorkflow(ctx context.Context, wf *tinkerbell.Workflow) error
}

type HardwareReader interface {
	ReadHardware(ctx context.Context, id, namespace string) (*tinkerbell.Hardware, error)
}

type HardwareCreator interface {
	CreateHardware(ctx context.Context, hw *tinkerbell.Hardware) error
}

type AutoCapabilities struct {
	Enrollment AutoEnrollment
	Discovery  AutoDiscovery
}

// AutoEnrollmentE is a struct that contains the auto enrollment configuration.
// Auto Enrollment is defined as automatically running a Workflow for an Agent that
// does not have a Workflow assigned to it. The Agent may or may not have a Hardware
// Object defined.
type AutoEnrollment struct {
	Enabled     bool
	ReadCreator AutoEnrollmentReadCreator
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
	AutoDiscoveryReadCreator
}
