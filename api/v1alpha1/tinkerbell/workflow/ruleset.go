package workflow

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=rulesets,scope=Namespaced,categories=tinkerbell,shortName=wrs,singular=ruleset
// +kubebuilder:storageversion
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io=
// +kubebuilder:metadata:labels=clusterctl.cluster.x-k8s.io/move=

// RuleSet is the Schema for the RuleSets API.
type RuleSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RuleSetSpec   `json:"spec,omitempty"`
	Status RuleSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RuleSetList contains a list of RuleSet.
type RuleSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RuleSet `json:"items"`
}

// RuleSetSpec defines the Rules, options, and Workflow to be created on rules match.
type RuleSetSpec struct {
	// Rules is a list of Quamina patterns used to match against the attributes of an Agent.
	// See https://github.com/timbray/quamina/blob/main/PATTERNS.md for more information on the required format.
	// All rules are combined using the OR operator.
	// If any rule matches, the corresponding Workflow will be created.
	Rules []string `json:"rules,omitempty"`
	// Workflow holds the data used to configure the created Workflow.
	Workflow RuleSetWorkflow `json:"workflow,omitempty"`
}

// RuleSetWorkflow defines the RuleSetWorkflow to be created when a rule matches.
type RuleSetWorkflow struct {
	// Disabled indicates whether the Workflow will be enabled or not when created.
	// +optional
	Disabled *bool `json:"disabled,omitempty"`
	// TemplateRef is the name of the Template to use for the Workflow.
	// Namespace is the namespace in which the Workflow will be created.
	Namespace string `json:"namespace,omitempty"`
	// AddAttributes indicates if the Agent attributes should be added as an Annotation in the created Workflow.
	// +optional
	AddAttributes bool `json:"addAttributes,omitempty"`
	// Template is the Template specific configuration to use when creating the Workflow.
	Template TemplateConfig `json:"template,omitempty"`
}

// TemplateConfig defines the Template specific configuration to use when creating the Workflow.
type TemplateConfig struct {
	// AgentValue is the Go template value used in the TemplateRef for the Task[].worker value.
	// For example: "device_id" or "worker_id".
	AgentValue string `json:"agentValue,omitempty"`
	// KVs are a mapping of key/value pairs usable in the referenced Template.
	// +optional
	KVs map[string]string `json:"kvs,omitempty"`
	// Ref is the name of an existing in cluster Template object to use in the Workflow.
	Ref string `json:"ref,omitempty"`
}

type RuleSetStatus struct{}
