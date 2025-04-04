package tinkerbell

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func init() {
	SchemeBuilder.Register(&WorkflowRuleSet{}, &WorkflowRuleSetList{})
}

// +kubebuilder:subresource:status
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=workflowrulesets,scope=Namespaced,categories=tinkerbell,shortName=wrs,singular=workflowruleset
// +kubebuilder:storageversion

// Workflow is the Schema for the Workflows API.
type WorkflowRuleSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowRuleSetSpec   `json:"spec,omitempty"`
	Status WorkflowRuleSetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkflowRuleSetList contains a list of WorkflowRuleSet.
type WorkflowRuleSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkflowRuleSet `json:"items"`
}

// WorkflowRuleSetSpec defines the Rules, options, and Workflow to be created on rules match.
type WorkflowRuleSetSpec struct {
	// Rules is a list of quamina rules to match against the attributes of an Agent.
	// See https://github.com/timbray/quamina/blob/main/PATTERNS.md for more information on the required format.
	Rules []string `json:"rules,omitempty"`
	// AddAttributesAsLabels indicates if the attributes should be added as labels to created Workflows.
	AddAttributesAsLabels bool `json:"addAttributesAsLabels,omitempty"`
	// AgentTemplateValue is the Go template value used in a Template for the Task[].worker value.
	AgentTemplateValue string `json:"agentTemplateValue,omitempty"`
	// WorkflowNamespace is the namespace in which the Workflow will be created.
	WorkflowNamespace string `json:"workflowNamespace,omitempty"`
	// Workflow is the Workflow to be created.
	Workflow WorkflowSpec `json:"workflow,omitempty"`
}

type WorkflowRuleSetStatus struct{}
