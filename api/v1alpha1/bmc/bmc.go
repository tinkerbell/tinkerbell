/*
Copyright 2022 Tinkerbell.

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

// Package bmc contains API GroupVersion definition for the BMC v1alpha1 API.
// +kubebuilder:object:generate=true
// +groupName=bmc.tinkerbell.org
// +versionName:=v1alpha1
package bmc

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GroupVersion = schema.GroupVersion{Group: "bmc.tinkerbell.org", Version: "v1alpha1"}

var (
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion,
		&Job{}, &JobList{},
		&Machine{}, &MachineList{},
		&Task{}, &TaskList{},
	)
	metav1.AddToGroupVersion(s, GroupVersion)
	return nil
}

// Hub is a marker function to indicate that this v1alpha1 spec is a Hub.
// See https://book.kubebuilder.io/multiversion-tutorial/conversion-concepts
func (m *Machine) Hub() {}

// Hub is a marker function to indicate that this v1alpha1 spec is a Hub.
// See https://book.kubebuilder.io/multiversion-tutorial/conversion-concepts
func (t *Task) Hub() {}

// Hub is a marker function to indicate that this v1alpha1 spec is a Hub.
// See https://book.kubebuilder.io/multiversion-tutorial/conversion-concepts
func (j *Job) Hub() {}
