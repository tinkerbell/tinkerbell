package kube

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinkerbell/tinkerbell/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *Backend) ReadAll(ctx context.Context, workerID string) ([]v1alpha1.Workflow, error) {
	stored := &v1alpha1.WorkflowList{}
	err := b.cluster.GetClient().List(ctx, stored, &client.MatchingFields{
		WorkflowByNonTerminalState: workerID,
	})
	if err != nil {
		return nil, err
	}
	wfs := []v1alpha1.Workflow{}
	for _, wf := range stored.Items {
		// If the current assigned or running action is assigned to the requested worker, include it
		if wf.Status.Tasks[wf.GetCurrentTaskIndex()].WorkerAddr == workerID {
			wfs = append(wfs, wf)
		}
	}
	return wfs, nil
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

func (b *Backend) Write(ctx context.Context, wf *v1alpha1.Workflow) error {
	if err := b.cluster.GetClient().Status().Update(ctx, wf); err != nil {
		return fmt.Errorf("failed to update workflow %s: %w", wf.Name, err)
	}

	return nil
}
