package kube

import (
	"context"
	"testing"

	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// readers is a cluster whose cached client and API reader see different objects.
type readers struct {
	cluster.Cluster
	cached client.Client
	live   client.Reader
}

func (r readers) GetClient() client.Client    { return r.cached }
func (r readers) GetAPIReader() client.Reader { return r.live }

// TestReadWorkflow verifies reads come from the API server, not a cache that hasn't caught up,
// and stay within the backend namespace.
func TestReadWorkflow(t *testing.T) {
	rs := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(rs); err != nil {
		t.Fatal(err)
	}
	meta := metav1.ObjectMeta{Name: "wf", Namespace: "tink"}
	// The cache has the Workflow before the Tink Controller populated its Tasks.
	stale := &v1alpha1.Workflow{ObjectMeta: meta}
	current := &v1alpha1.Workflow{
		ObjectMeta: meta,
		Status: v1alpha1.WorkflowStatus{Tasks: []v1alpha1.Task{{
			ID:      "task-1",
			Actions: []v1alpha1.Action{{ID: "action-1"}},
		}}},
	}

	tests := map[string]struct {
		backendNamespaces []string
		cached, live      client.Client
		wantNotFound      bool
	}{
		"stale cache": {
			cached: fake.NewClientBuilder().WithScheme(rs).WithObjects(stale).Build(),
			live:   fake.NewClientBuilder().WithScheme(rs).WithObjects(current).Build(),
		},
		"missing from cache": {
			cached: fake.NewClientBuilder().WithScheme(rs).Build(),
			live:   fake.NewClientBuilder().WithScheme(rs).WithObjects(current).Build(),
		},
		"outside the backend namespace": {
			backendNamespaces: []string{"other"},
			cached:            fake.NewClientBuilder().WithScheme(rs).Build(),
			live:              fake.NewClientBuilder().WithScheme(rs).WithObjects(current).Build(),
			wantNotFound:      true,
		},
		"inside the backend namespace": {
			backendNamespaces: []string{"tink"},
			cached:            fake.NewClientBuilder().WithScheme(rs).Build(),
			live:              fake.NewClientBuilder().WithScheme(rs).WithObjects(current).Build(),
		},
		"inside one of several backend namespaces": {
			backendNamespaces: []string{"other", "tink"},
			cached:            fake.NewClientBuilder().WithScheme(rs).Build(),
			live:              fake.NewClientBuilder().WithScheme(rs).WithObjects(current).Build(),
		},
		"deleted": {
			cached:       fake.NewClientBuilder().WithScheme(rs).WithObjects(stale).Build(),
			live:         fake.NewClientBuilder().WithScheme(rs).Build(),
			wantNotFound: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			b := &Backend{Namespaces: tt.backendNamespaces, cluster: readers{cached: tt.cached, live: tt.live}}
			got, err := b.ReadWorkflow(context.Background(), "wf", "tink")
			if tt.wantNotFound {
				if !apierrors.IsNotFound(err) {
					t.Fatalf("expected a not found error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Status.Tasks) != 1 || got.Status.Tasks[0].Actions[0].ID != "action-1" {
				t.Fatalf("got Tasks %+v, want the API server's version with action-1", got.Status.Tasks)
			}
		})
	}
}
