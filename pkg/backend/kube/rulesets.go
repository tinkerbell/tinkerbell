package kube

import (
	"context"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *Backend) ListWorkflowRuleSets(ctx context.Context, opts data.ReadListOptions) ([]v1alpha1.WorkflowRuleSet, error) {
	list := &v1alpha1.WorkflowRuleSetList{}
	lo := []client.ListOption{}
	if opts.InNamespace != "" {
		lo = append(lo, client.InNamespace(opts.InNamespace))
	}
	err := b.cluster.GetClient().List(ctx, list, lo...)
	if err != nil {
		return nil, err
	}

	return list.Items, nil
}
