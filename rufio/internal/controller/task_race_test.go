package controller_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestTaskReconcileReplacedTask covers a Task deleted and replaced by one of the same name while its
// reconcile runs the BMC action, as the Job reconciler does with a Task left by a deleted Job. The old
// reconcile's status patch must not land on the replacement: the replacement's Job would skip its
// action, or fail from its predecessor's error.
func TestTaskReconcileReplacedTask(t *testing.T) {
	tests := map[string]struct {
		provider *testProvider
		started  bool
	}{
		"start":      {provider: &testProvider{PowerSetOK: true}},
		"failure":    {provider: &testProvider{ErrPowerStateSet: errors.New("failed to set power state")}},
		"completion": {provider: &testProvider{Powerstate: "on"}, started: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			secret := createSecret()
			task := createTask("PowerOn", getAction("PowerOn"), secret)
			if tt.started {
				started := metav1.NewTime(time.Now().Add(-10 * time.Second))
				task.Status.StartTime = &started
			}
			replacement := createTask(task.Name, getAction("PowerOn"), secret)
			key := client.ObjectKeyFromObject(task)

			var patchErr error
			cluster := interceptor.NewClient(newClientBuilder().WithObjects(task, secret).Build(), interceptor.Funcs{
				// Replace the Task once its action ran, right before the old reconcile patches its status.
				SubResourcePatch: func(ctx context.Context, c client.Client, subResource string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
					if err := c.Delete(ctx, replacement.DeepCopy()); err != nil {
						return err
					}
					if err := c.Create(ctx, replacement.DeepCopy()); err != nil {
						return err
					}
					patchErr = c.SubResource(subResource).Patch(ctx, obj, patch, opts...)
					return patchErr
				},
			})
			reconciler := controller.NewTaskReconciler(cluster, newTestClient(tt.provider))

			if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err == nil {
				t.Fatal("expected the status patch on the replaced Task to fail")
			}
			if !apierrors.IsConflict(patchErr) {
				t.Fatalf("expected a conflict on the status patch, got: %v", patchErr)
			}

			var got bmc.Task
			if err := cluster.Get(context.Background(), key, &got); err != nil {
				t.Fatalf("expected nil err, got: %v", err)
			}
			if diff := cmp.Diff(bmc.TaskStatus{}, got.Status); diff != "" {
				t.Fatalf("the old reconcile patched the replacement's status (-want +got):\n%s", diff)
			}
		})
	}
}
