// Package tinkerbell contains API GroupVersion definition for the Tinkerbell v1alpha1 API.
// +kubebuilder:object:generate=true
// +groupName=tinkerbell.org
// +versionName:=v1alpha1
package tinkerbell

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is group version used to register these objects.
var GroupVersion = schema.GroupVersion{Group: "tinkerbell.org", Version: "v1alpha1"}

var (
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion,
		&Hardware{}, &HardwareList{},
		&Template{}, &TemplateList{},
		&Workflow{}, &WorkflowList{},
		&WorkflowRuleSet{}, &WorkflowRuleSetList{},
	)
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}
