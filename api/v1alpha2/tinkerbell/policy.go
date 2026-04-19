package tinkerbell

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io=
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io/move=
// +kubebuilder:resource:path=policies,scope=Namespaced,categories=tinkerbell,shortName=pol,singular=policy
// +kubebuilder:storageversion

// Policy is the Schema for the Tinkerbell Policy API.
type Policy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// PolicyList contains a list of Policies.
type PolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Policy `json:"items"`
}

// PolicySpec defines the desired state of all Policies.
type PolicySpec struct {
	// Rules defines the different types of rules that are applied for a Policy.
	Rules Rules `json:"rules,omitempty"`
}

// Rules defines the different types of rules that are applied for a Policy.
type Rules struct {
	// WorkflowAutoCreation defines rules for automatically creating Workflows based on Agent attributes.
	// Multiple rules in this array are combined with OR logic (any rule can match).
	// Within each rule, fields are combined with AND logic (all specified fields must match).
	// If any rule matches, the corresponding Workflow will be created.
	// +optional
	WorkflowAutoCreation []WorkflowRule `json:"workflowAutoCreation,omitempty"`

	// ReferenceAccess defines rules for granting access to References based on object pattern matching.
	ReferenceAccess *ReferenceRules `json:"referenceAccess,omitempty"`
}

type ReferenceRules struct {
	// Hardware defines rules for granting Hardware objects access to References.
	// Multiple rules in this array are combined with OR logic (any rule can match).
	// Within each rule, fields are combined with AND logic (all specified fields must match).
	// If any rule matches, the Hardware templating will have access to the Reference object(s) through the corresponding Hardware object.
	// +optional
	Hardware []AccessRule `json:"hardware,omitempty"`

	// Task defines rules for granting Task objects access to References.
	// Multiple rules in this array are combined with OR logic (any rule can match).
	// Within each rule, fields are combined with AND logic (all specified fields must match).
	// If any rule matches, the Task templating will have access to the Reference object(s) through the corresponding Task object.
	// +optional
	Task []AccessRule `json:"task,omitempty"`
}

// WorkflowRule defines the Rules, options, and Workflow to be created on rules match.
type WorkflowRule struct {
	// Rules is a list of Quamina patterns used to match against the attributes of an Agent.
	// See https://github.com/timbray/quamina/blob/main/PATTERNS.md for more information on the required format.
	// Multiple rules in this array are combined with OR logic (any rule can match).
	// Within each rule, fields are combined with AND logic (all specified fields must match).
	// If any rule matches, the corresponding Workflow will be created.
	Rules []AgentAttributes `json:"rules,omitempty"`

	// Config holds the configuration used to create the Workflow when a rule matches.
	Config WorkflowConfig `json:"config,omitempty"`
}

// WorkflowConfig defines the Workflow to be created when a rule matches.
type WorkflowConfig struct {
	// Namespace is the namespace in which the Workflow will be created.
	Namespace string `json:"namespace,omitempty"`

	// AddAttributes indicates if the Agent attributes should be added as an Annotation in the created Workflow.
	// +optional
	AddAttributes bool `json:"addAttributes,omitempty"`

	// Disabled indicates whether the Workflow will be enabled or not when created.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`

	// Extra holds any additional configuration values that will be added to all Tasks in the created Workflow.
	Extra *Extra `json:"extra,omitempty"`

	// TimeoutSeconds is the duration in seconds before the Workflow times out.
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty"`

	// TaskRef is the Task to use for the Workflow.
	TaskRef SimpleReference `json:"taskRef,omitempty"`
}

// AccessRule represents a Quamina pattern for matching reference access events.
// Multiple fields within a single rule are combined with AND logic.
// Use an array of AccessRule to combine multiple rules with OR logic.
type AccessRule struct {
	// Source matches the object making the reference.
	// +optional
	Source *SourcePattern `json:"source,omitempty"`

	// Reference matches the object being referenced.
	// +optional
	Reference *ReferencePattern `json:"reference,omitempty"`
}

// SourcePattern matches attributes of the source Hardware object.
type SourcePattern struct {
	// Name matches the name of the Hardware object.
	// Multiple values are combined with OR logic.
	// +optional
	Name FieldPattern `json:"name,omitempty"`

	// Namespace matches the namespace of the Hardware object.
	// Multiple values are combined with OR logic.
	// +optional
	Namespace FieldPattern `json:"namespace,omitempty"`
}

// ReferencePattern matches attributes of the referenced Kubernetes object.
type ReferencePattern struct {
	// Name matches the name of the referenced object.
	// Multiple values are combined with OR logic.
	// +optional
	Name FieldPattern `json:"name,omitempty"`

	// Namespace matches the namespace of the referenced object.
	// Multiple values are combined with OR logic.
	// +optional
	Namespace FieldPattern `json:"namespace,omitempty"`

	// Group matches the API group of the referenced object.
	// Multiple values are combined with OR logic.
	// +optional
	Group FieldPattern `json:"group,omitempty"`

	// Version matches the API version of the referenced object.
	// Multiple values are combined with OR logic.
	// +optional
	Version FieldPattern `json:"version,omitempty"`

	// Resource matches the resource type (plural) of the referenced object.
	// Multiple values are combined with OR logic.
	// +optional
	Resource FieldPattern `json:"resource,omitempty"`
}

// PatternValue can hold a string, number, or boolean for exact matching.
// PatternValue represents a Quamina pattern value.
// It can be a plain value (string, number, boolean) for exact match,
// or an object with pattern operators like prefix, suffix, wildcard, etc.
// Examples:
//   - "value" (exact match string)
//   - 123 (exact match number)
//   - true (exact match boolean)
//
// +kubebuilder:pruning:PreserveUnknownFields
// +kubebuilder:validation:XPreserveUnknownFields
type PatternValue apiextensionsv1.JSON

// FieldPattern represents all possible pattern values for a single field.
// Multiple values in the array are combined with OR logic (any value can match).
type FieldPattern []PatternValue
