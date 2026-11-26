// Package tinkerbell contains API GroupVersion definition for the Tinkerbell v1alpha2 API.
// +kubebuilder:object:generate=true
// +groupName=tinkerbell.org
// +versionName:=v1alpha2
package tinkerbell

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is group version used to register these objects.
var GroupVersion = schema.GroupVersion{Group: "tinkerbell.org", Version: "v1alpha2"}

var (
	// SchemeBuilder is used to add API types to a runtime scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds this group's types to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion,
		&Hardware{}, &HardwareList{},
		&Policy{}, &PolicyList{},
		&Task{}, &TaskList{},
		&Workflow{}, &WorkflowList{},
	)
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}

// SimpleReference identifies an object by name in the same namespace as the referring object.
type SimpleReference struct {
	// Name of the object.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`
}
