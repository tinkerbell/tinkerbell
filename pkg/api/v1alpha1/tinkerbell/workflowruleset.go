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

type WorkflowRuleSetSpec struct {
	Rules                 []string     `json:"rules,omitempty"`
	AddAttributesAsLabels bool         `json:"addAttributesAsLabels,omitempty"`
	WorkerTemplateName    string       `json:"workerTemplateName,omitempty"`
	WorkflowNamespace     string       `json:"workflowNamespace,omitempty"`
	Workflow              WorkflowSpec `json:"workflow,omitempty"`
}

type WorkflowRuleSetStatus struct{}
