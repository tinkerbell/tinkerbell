package kube

import (
	"context"
	"fmt"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *Backend) CreateWorkflow(ctx context.Context, w *v1alpha1.Workflow) error {
	if err := b.cluster.GetClient().Create(ctx, w); err != nil {
		return fmt.Errorf("failed to create workflow %s: %w", w.Name, err)
	}

	return nil
}

// ReadWorkflow returns the Workflow as currently stored by the API server, not as cached,
// so the result is never stale. A Workflow outside b.Namespaces is reported as not found.
func (b *Backend) ReadWorkflow(ctx context.Context, name, namespace string) (*v1alpha1.Workflow, error) {
	if !b.Watches(namespace) {
		return nil, apierrors.NewNotFound(v1alpha1.GroupVersion.WithResource("workflows").GroupResource(), name)
	}
	wflw := &v1alpha1.Workflow{}
	if err := b.cluster.GetAPIReader().Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, wflw); err != nil {
		return nil, fmt.Errorf("failed to get workflow %s/%s: %w", namespace, name, err)
	}
	return wflw, nil
}

func (b *Backend) ListWorkflows(ctx context.Context, opts data.WorkflowFilter) ([]v1alpha1.Workflow, error) {
	stored := &v1alpha1.WorkflowList{}
	los := []client.ListOption{}
	if opts.InNamespace != "" {
		los = append(los, client.InNamespace(opts.InNamespace))
	}
	if opts.ByAgentID != "" {
		los = append(los, client.MatchingFields{WorkflowAgentIDIndex: opts.ByAgentID})
	}
	if err := b.cluster.GetClient().List(ctx, stored, los...); err != nil {
		return nil, fmt.Errorf("failed to list workflows in namespace %s: %w", opts.InNamespace, err)
	}

	return stored.Items, nil
}

func (b *Backend) UpdateWorkflow(ctx context.Context, wf *v1alpha1.Workflow, opts data.UpdateOptions) error {
	cc := b.cluster.GetClient()

	if p, err := patchFromOpts(opts); err != nil {
		return fmt.Errorf("invalid patch options for workflow %s: %w", wf.Name, err)
	} else if p != nil {
		if opts.StatusOnly {
			if err := cc.Status().Patch(ctx, wf, p); err != nil {
				return fmt.Errorf("failed to patch workflow status %s: %w", wf.Name, err)
			}
			return nil
		}
		if err := cc.Patch(ctx, wf, p); err != nil {
			return fmt.Errorf("failed to patch workflow %s: %w", wf.Name, err)
		}
		return nil
	}

	if opts.StatusOnly {
		// Only update the status subresource of the workflow. This is used by the tinkerbell server to update the workflow status without having to worry about conflicts with the controller which may be updating the workflow spec at the same time.
		if err := cc.Status().Update(ctx, wf); err != nil {
			return fmt.Errorf("failed to update workflow status %s: %w", wf.Name, err)
		}
		return nil
	}
	if err := cc.Update(ctx, wf); err != nil {
		return fmt.Errorf("failed to update workflow %s: %w", wf.Name, err)
	}

	return nil
}
