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

package tinkerbell

import (
	"github.com/tinkerbell/tinkerbell/api/v1alpha2/tinkerbell/bmc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=bmcs,scope=Namespaced,categories=tinkerbell,shortName=b,singular=bmc
// +kubebuilder:storageversion
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io=
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io/move=
// +kubebuilder:printcolumn:JSONPath=".status.conditions[?(@.type==\"Contactable\")].status",name=contactable,type=string,description="The contactable status of the bmc",priority=1
// +kubebuilder:printcolumn:JSONPath=".status.conditions[?(@.type==\"PowerState\")].status",name=power-state,type=string,description="The power state of the bmc's machine",priority=1

// BMC is the Schema for the bmcs API.
type BMC struct {
	metav1.TypeMeta   `json:""`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BMCSpec   `json:"spec,omitempty"`
	Status BMCStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BMCList contains a list of BMCs.
type BMCList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BMC `json:"items"`
}

// BMCSpec defines desired BMC state.
type BMCSpec struct {
	// Connection contains connection data for a Baseboard Management Controller.
	Connection bmc.Connection `json:"connection"`
}

// BMCStatus defines the observed state of BMC.
type BMCStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	Conditions []bmc.Condition `json:"conditions,omitempty"`
}
