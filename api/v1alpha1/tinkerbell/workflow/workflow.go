// Package workflow contains API GroupVersion definition for the Tinkerbell v1alpha1 API.
// +kubebuilder:object:generate=true
// +groupName=workflow.tinkerbell.org
// +versionName:=v1alpha1
package workflow

import "k8s.io/apimachinery/pkg/runtime/schema"

var GroupVersion = schema.GroupVersion{Group: "workflow.tinkerbell.org", Version: "v1alpha1"}
