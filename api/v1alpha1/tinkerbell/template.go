package tinkerbell

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TemplateState represents the template state.
type TemplateState string

const (
	// TemplateError represents a template that is in an error state.
	TemplateError = TemplateState("Error")

	// TemplateReady represents a template that is in a ready state.
	TemplateReady = TemplateState("Ready")
)

// TemplateSpec defines the desired state of Template.
type TemplateSpec struct {
	// +optional
	Data *string `json:"data,omitempty"`

	// SecretRef is an optional reference to a Kubernetes Secret whose data
	// will be available in the template as `.secret.<key>`.
	// All keys from the referenced secret will be exposed to the template.
	// The secret must exist in the same namespace as the Template.
	// +optional
	SecretRef *TemplateSecretReference `json:"secretRef,omitempty"`
}

// TemplateSecretReference defines a reference to a Kubernetes Secret for use in Templates.
// The secret must exist in the same namespace as the Template.
type TemplateSecretReference struct {
	// Name is the name of the secret in the same namespace as the Template.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// TemplateStatus defines the observed state of Template.
type TemplateStatus struct {
	State TemplateState `json:"state,omitempty"`
}

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=templates,scope=Namespaced,categories=tinkerbell,shortName=tpl,singular=template
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:JSONPath=".status.state",name=State,type=string

// Template is the Schema for the Templates API.
type Template struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TemplateSpec   `json:"spec,omitempty"`
	Status TemplateStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TemplateList contains a list of Templates.
type TemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Template `json:"items"`
}
