package kube

import (
	"context"
	"fmt"
	"strings"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *Backend) ReadAll(ctx context.Context, agentID string) ([]v1alpha1.Workflow, error) {
	stored := &v1alpha1.WorkflowList{}
	err := b.cluster.GetClient().List(ctx, stored, &client.MatchingFields{
		WorkflowByAgentID: agentID,
	})
	if err != nil {
		return nil, err
	}
	return stored.Items, nil
}

func (b *Backend) Read(ctx context.Context, workflowID, namespace string) (*v1alpha1.Workflow, error) {
	workflowNamespace, workflowName, found := strings.Cut(workflowID, "/")
	if !found {
		workflowName = workflowID
		workflowNamespace = namespace
	}

	wflw := &v1alpha1.Workflow{}
	err := b.cluster.GetClient().Get(ctx, types.NamespacedName{Name: workflowName, Namespace: workflowNamespace}, wflw)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow. id: %s, name: %s, namespace: %s, err: %w", workflowID, workflowName, workflowNamespace, err)
	}
	return wflw, nil
}

func (b *Backend) Update(ctx context.Context, wf *v1alpha1.Workflow) error {
	if err := b.cluster.GetClient().Status().Update(ctx, wf); err != nil {
		return fmt.Errorf("failed to update workflow %s: %w", wf.Name, err)
	}

	return nil
}

func (b *Backend) ReadWorkflowRuleSets(ctx context.Context) ([]v1alpha1.WorkflowRuleSet, error) {
	stored := &v1alpha1.WorkflowRuleSetList{}
	// TODO: add pagination.
	opts := &client.ListOptions{}
	err := b.cluster.GetClient().List(ctx, stored, opts)
	if err != nil {
		return nil, err
	}

	return stored.Items, nil
}

func (b *Backend) CreateWorkflow(ctx context.Context, wf *v1alpha1.Workflow) error {
	if err := b.cluster.GetClient().Create(ctx, wf); err != nil {
		return fmt.Errorf("failed to create workflow %s: %w", wf.Name, err)
	}

	return nil
}

func (b *Backend) ReadHardware(ctx context.Context, id, namespace string) (*v1alpha1.Hardware, error) {
	hw := &v1alpha1.HardwareList{}
	if err := b.cluster.GetClient().List(ctx, hw, &client.MatchingFields{
		HardwareByAgentID: id,
	}); err != nil {
		return nil, fmt.Errorf("failed to get hardware %s/%s: %w", namespace, id, err)
	}
	if len(hw.Items) == 0 {
		err := hardwareNotFoundError{name: id, namespace: ternary(b.Namespace == "", "all namespaces", b.Namespace)}

		return nil, err
	}

	if len(hw.Items) > 1 {
		// This is unexpected, as we should not have multiple hardware objects with the same agent ID.
		return nil, &foundMultipleHardwareError{id: id, namespace: namespace, count: len(hw.Items)}
	}

	return &hw.Items[0], nil
}

func (b *Backend) CreateHardware(ctx context.Context, hw *v1alpha1.Hardware) error {
	if err := b.cluster.GetClient().Create(ctx, hw); err != nil {
		return fmt.Errorf("failed to create hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	return nil
}
