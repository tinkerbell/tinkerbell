/*
Copyright 2025 Tinkerbell.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package bmc contains API GroupVersion definition for the BMC v1alpha2 API.
// +kubebuilder:object:generate=true
// +groupName=bmc.tinkerbell.org
// +versionName:=v1alpha2
package bmc

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion = schema.GroupVersion{Group: "bmc.tinkerbell.org", Version: "v1alpha2"}

	// schemeBuilder is used to add go types to the GroupVersionKind scheme.
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = schemeBuilder.AddToScheme
)

// addKnownTypes adds the set of types defined in this package to the supplied scheme.
// This returns nil in order to comply with the expected function signature in runtime.NewSchemeBuilder.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, []runtime.Object{}...)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// Connection contains connection data for a Baseboard Management Controller.
type Connection struct {
	// AuthSecretRef is the SecretReference that contains authentication information of the Machine.
	// The Secret must contain username and password keys. This is optional as it is not required when using
	// the RPC provider.
	// +optional
	AuthSecretRef SecretReference `json:"authSecretRef"`

	// Host is the host IP address or hostname of the Machine.
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// InsecureTLS specifies trusted TLS connections.
	InsecureTLS bool `json:"insecureTLS"`

	// ProviderOptions contains provider specific options.
	// +optional
	ProviderOptions *ProviderOptions `json:"providerOptions,omitempty"`
}

type ConditionType string

type ConditionStatus string

const (
	ConditionTypeMachineContactable ConditionType = "Contactable"
	ConditionTypeMachinePowerState  ConditionType = "PowerState"
	ConditionTypeCompleted          ConditionType = "Completed"
	ConditionTypeFailed             ConditionType = "Failed"
	ConditionTypeRunning            ConditionType = "Running"

	ConditionStatusTrue    ConditionStatus = "True"
	ConditionStatusFalse   ConditionStatus = "False"
	ConditionStatusUnknown ConditionStatus = "Unknown"
	ConditionStatusOn      ConditionStatus = "On"
	ConditionStatusOff     ConditionStatus = "Off"
)

type Condition struct {
	// Type of the Job condition.
	Type ConditionType `json:"type"`

	// Status is the status of the Job condition.
	// Can be True or False.
	Status ConditionStatus `json:"status"`

	// Message represents human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`

	// LastUpdateTime of the condition.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// ObservedGeneration is the generation of the Machine that was last observed by the controller.
	// It is used to determine if the condition is up to date with the latest changes.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// HasCondition checks if the cType condition is present with status cStatus on a bmt.
func HasConditionStatus(existingConditions []Condition, ct ConditionType, cs ConditionStatus) bool {
	for _, c := range existingConditions {
		if c.Type == ct {
			return c.Status == cs
		}
	}

	return false
}

// SetCondition applies the condition to the resource's status. If the condition already exists, it is updated.
// This is a generic function that works with BMC, Operation, and Job types.
func SetCondition(existingConditions []Condition, toAdd Condition) []Condition {
	if existingConditions == nil {
		existingConditions = []Condition{toAdd}
		return existingConditions
	}

	for i, c := range existingConditions {
		if c.Type == toAdd.Type {
			existingConditions[i] = toAdd
			return existingConditions
		}
	}

	existingConditions = append(existingConditions, toAdd)
	return existingConditions
}
