package tinkerbell_test

import (
	"testing"

	"github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell"
	"github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell/bmc"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAddToSchemeRegistersResourcesAndLists(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := tinkerbell.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	objects := []runtime.Object{
		&tinkerbell.Hardware{}, &tinkerbell.HardwareList{},
		&tinkerbell.Policy{}, &tinkerbell.PolicyList{},
		&tinkerbell.Task{}, &tinkerbell.TaskList{},
		&tinkerbell.Workflow{}, &tinkerbell.WorkflowList{},
	}
	for _, obj := range objects {
		gvks, unversioned, err := scheme.ObjectKinds(obj)
		if err != nil {
			t.Errorf("ObjectKinds(%T) error = %v", obj, err)
			continue
		}
		if unversioned {
			t.Errorf("ObjectKinds(%T) unexpectedly reports an unversioned type", obj)
		}
		if len(gvks) != 1 || gvks[0].GroupVersion() != tinkerbell.GroupVersion {
			t.Errorf("ObjectKinds(%T) = %v, want one kind in %s", obj, gvks, tinkerbell.GroupVersion)
		}
	}
}

func TestAddToSchemeComposesWithBMC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := tinkerbell.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := bmc.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, gvk := range []struct {
		object runtime.Object
		group  string
		kind   string
	}{
		{&tinkerbell.Hardware{}, tinkerbell.GroupVersion.Group, "Hardware"},
		{&tinkerbell.Policy{}, tinkerbell.GroupVersion.Group, "Policy"},
		{&tinkerbell.Task{}, tinkerbell.GroupVersion.Group, "Task"},
		{&tinkerbell.Workflow{}, tinkerbell.GroupVersion.Group, "Workflow"},
		{&bmc.Job{}, bmc.GroupVersion.Group, "Job"},
	} {
		kinds, _, err := scheme.ObjectKinds(gvk.object)
		if err != nil {
			t.Fatalf("ObjectKinds(%T): %v", gvk.object, err)
		}
		if len(kinds) != 1 || kinds[0].Group != gvk.group || kinds[0].Version != "v1alpha2" || kinds[0].Kind != gvk.kind {
			t.Errorf("ObjectKinds(%T) = %v", gvk.object, kinds)
		}
	}
}
