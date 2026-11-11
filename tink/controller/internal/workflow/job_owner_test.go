package workflow

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestCreateJobOwnerReference checks the owner reference of a created Job against a Workflow without
// TypeMeta, which is how a typed client returns it unless it comes from the manager's cache. The
// owner reference must still name the Workflow's kind and group version.
func TestCreateJobOwnerReference(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := bmc.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	hw := &v1alpha1.Hardware{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hardware", Namespace: "default"},
		Spec: v1alpha1.HardwareSpec{
			BMCRef: &v1.TypedLocalObjectReference{Name: "test-bmc", Kind: "machine.bmc.tinkerbell.org"},
		},
	}
	wf := &v1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workflow", Namespace: "default", UID: "workflow-uid"},
		Spec:       v1alpha1.WorkflowSpec{HardwareRef: hw.Name},
		Status: v1alpha1.WorkflowStatus{
			BootOptions: v1alpha1.BootOptionsStatus{Jobs: map[string]v1alpha1.JobStatus{}},
		},
	}
	cc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(hw).Build()
	s := &state{client: cc, workflow: wf}

	if _, err := s.createJob(context.Background(), []bmc.Action{{PowerAction: toPtr(bmc.PowerOn)}}, jobNameNetboot); err != nil {
		t.Fatalf("createJob() error = %v", err)
	}

	job := &bmc.Job{}
	if err := cc.Get(context.Background(), client.ObjectKey{Name: jobNameNetboot.String(), Namespace: wf.Namespace}, job); err != nil {
		t.Fatalf("failed to get the created Job: %v", err)
	}
	want := []metav1.OwnerReference{{
		APIVersion: v1alpha1.GroupVersion.String(),
		Kind:       "Workflow",
		Name:       wf.Name,
		UID:        wf.UID,
		Controller: toPtr(true),
	}}
	if diff := cmp.Diff(want, job.OwnerReferences); diff != "" {
		t.Errorf("unexpected owner references (-want +got):\n%s", diff)
	}
}
