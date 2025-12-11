// Package pipeline contains API GroupVersion definition for the Tinkerbell v1alpha2 API.
// +kubebuilder:object:generate=true
// +groupName=pipeline.tinkerbell.org
// +versionName:=v1alpha2
package pipeline

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is group version used to register these objects.
var GroupVersion = schema.GroupVersion{Group: "pipeline.tinkerbell.org", Version: "v1alpha2"}
