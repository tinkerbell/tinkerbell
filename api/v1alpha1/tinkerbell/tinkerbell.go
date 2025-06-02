// Package tinkerbell contains API GroupVersion definition for the Tinkerbell v1alpha1 API.
// +kubebuilder:object:generate=true
// +groupName=tinkerbell.org
// +versionName:=v1alpha1
package tinkerbell

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is group version used to register these objects.
var GroupVersion = schema.GroupVersion{Group: "tinkerbell.org", Version: "v1alpha1"}
