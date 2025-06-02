// Package api contains API Schema definitions for the Tinkerbell and BMC v1alpha1 API groups.
// +kubebuilder:object:generate=true
// +groupName=tinkerbell.org
// +versionName:=v1alpha1
package api

import (
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilderTinkerbell = &scheme.Builder{GroupVersion: tinkerbell.GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToSchemeTinkerbell = SchemeBuilderTinkerbell.AddToScheme

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilderBMC = &scheme.Builder{GroupVersion: bmc.GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToSchemeBMC = SchemeBuilderBMC.AddToScheme
)

func init() {
	SchemeBuilderTinkerbell.Register(&tinkerbell.Hardware{}, &tinkerbell.HardwareList{})
	SchemeBuilderTinkerbell.Register(&tinkerbell.Template{}, &tinkerbell.TemplateList{})
	SchemeBuilderTinkerbell.Register(&tinkerbell.Workflow{}, &tinkerbell.WorkflowList{})
	SchemeBuilderTinkerbell.Register(&tinkerbell.WorkflowRuleSet{}, &tinkerbell.WorkflowRuleSetList{})

	SchemeBuilderBMC.Register(&bmc.Job{}, &bmc.JobList{})
	SchemeBuilderBMC.Register(&bmc.Machine{}, &bmc.MachineList{})
	SchemeBuilderBMC.Register(&bmc.Task{}, &bmc.TaskList{})
}
