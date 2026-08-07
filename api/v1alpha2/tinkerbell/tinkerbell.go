// Package tinkerbell contains API GroupVersion definition for the Tinkerbell v1alpha2 API.
// +kubebuilder:object:generate=true
// +groupName=tinkerbell.org
// +versionName:=v1alpha2
package tinkerbell

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is group version used to register these objects.
var GroupVersion = schema.GroupVersion{Group: "tinkerbell.org", Version: "v1alpha2"}

// SimpleReference
// +kubebuilder:validation:XValidation:rule="(has(self.name) && self.name != \"\") == (has(self.namespace) && self.namespace != \"\")",message="name and namespace must both be specified or both be empty"
type SimpleReference struct {
	// Name of the object.
	Name string `json:"name,omitempty"`

	// Namespace where the object resides.
	Namespace string `json:"namespace,omitempty"`
}
