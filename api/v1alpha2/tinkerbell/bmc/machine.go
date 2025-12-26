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

package bmc

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=machines,scope=Namespaced,categories=tinkerbell,shortName=m,singular=machine
// +kubebuilder:storageversion
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io=
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io/move=
// +kubebuilder:printcolumn:JSONPath=".status.conditions[?(@.type==\"Contactable\")].status",name=contactable,type=string,description="The contactable status of the machine",priority=1
// +kubebuilder:printcolumn:JSONPath=".status.conditions[?(@.type==\"PowerState\")].status",name=power-state,type=string,description="The power state of the machine",priority=1

// Machine is the Schema for the machines API.
type Machine struct {
	metav1.TypeMeta   `json:""`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSpec   `json:"spec,omitempty"`
	Status MachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MachineList contains a list of Machines.
type MachineList struct {
	metav1.TypeMeta `json:""`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Machine `json:"items"`
}

// MachineSpec defines desired machine state.
type MachineSpec struct {
	// Connection contains connection data for a Baseboard Management Controller.
	Connection Connection `json:"connection"`
}

// MachineStatus defines the observed state of Machine.
type MachineStatus struct {
	// Conditions represents the latest available observations of an object's current state.
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
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
