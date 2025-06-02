package grpc

import (
	"context"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

type AutoReadCreator interface {
	WorkflowRuleSetReader
	WorkflowCreator
}

type WorkflowRuleSetReader interface {
	ReadWorkflowRuleSets(ctx context.Context) ([]tinkerbell.WorkflowRuleSet, error)
}

type WorkflowCreator interface {
	CreateWorkflow(ctx context.Context, wf *tinkerbell.Workflow) error
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
	ReadCreator AutoReadCreator
}

// AutoDiscovery is a struct that contains the auto discovery configuration.
// Auto Discovery is defined as automatically creating a Hardware Object for an
// Agent that does not have a Workflow or a Hardware Object assigned to it.
// The Namespace defines the namespace to use when creating the Hardware Object.
// An empty namespace will cause all Hardware Objects to be created in the same
// namespace as the Tink Server.
type AutoDiscovery struct {
	Enabled   bool
	Namespace string
}
