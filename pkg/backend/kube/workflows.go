package kube

import (
	"context"
	"fmt"
	"strings"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *Backend) CreateWorkflow(ctx context.Context, w *v1alpha1.Workflow) error {
	if err := b.cluster.GetClient().Create(ctx, w); err != nil {
		return fmt.Errorf("failed to create workflow %s: %w", w.Name, err)
	}

	return nil
}

func (b *Backend) ReadWorkflow(ctx context.Context, name, namespace string, opts data.ReadListOptions) (*v1alpha1.Workflow, error) {
	workflowNamespace, workflowName, found := strings.Cut(name, "/")
	if !found {
		workflowName = name
		workflowNamespace = namespace
	}

	wflw := &v1alpha1.Workflow{}
	err := b.cluster.GetClient().Get(ctx, types.NamespacedName{Name: workflowName, Namespace: workflowNamespace}, wflw)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow. id: %s, name: %s, namespace: %s, err: %w", name, workflowName, workflowNamespace, err)
	}
	return wflw, nil
}

func (b *Backend) ListWorkflows(ctx context.Context, namespace string, opts data.ReadListOptions) ([]v1alpha1.Workflow, error) {
	stored := &v1alpha1.WorkflowList{}
	los := []client.ListOption{}
	if namespace != "" {
		los = append(los, client.InNamespace(namespace))
	}
	if opts.ByAgentID != "" {
		los = append(los, client.MatchingFields{WorkflowAgentIDIndex: opts.ByAgentID})
	}
	if err := b.cluster.GetClient().List(ctx, stored, los...); err != nil {
		return nil, fmt.Errorf("failed to list workflows in namespace %s: %w", namespace, err)
	}

	return stored.Items, nil
}

func (b *Backend) UpdateWorkflow(ctx context.Context, wf *v1alpha1.Workflow, opts data.UpdateOptions) error {
	if opts.StatusOnly {
		// Only update the status subresource of the workflow. This is used by the tinkerbell server to update the workflow status without having to worry about conflicts with the controller which may be updating the workflow spec at the same time.
		if err := b.cluster.GetClient().Status().Update(ctx, wf); err != nil {
			return fmt.Errorf("failed to update workflow status %s: %w", wf.Name, err)
		}
		return nil
	}
	if err := b.cluster.GetClient().Update(ctx, wf); err != nil {
		return fmt.Errorf("failed to update workflow %s: %w", wf.Name, err)
	}

	return nil
}

func (b *Backend) DeleteWorkflow(ctx context.Context, name, namespace string) error {
	return nil
}
