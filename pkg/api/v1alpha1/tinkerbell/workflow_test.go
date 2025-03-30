package tinkerbell

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetCondition(t *testing.T) {
	tests := map[string]struct {
		ExistingConditions []WorkflowCondition
		WantConditions     []WorkflowCondition
		Condition          WorkflowCondition
	}{
		"update existing condition": {
			ExistingConditions: []WorkflowCondition{
				{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionTrue},
				{Type: ToggleAllowNetbootFalse, Status: metav1.ConditionTrue},
			},
			WantConditions: []WorkflowCondition{
				{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionFalse},
				{Type: ToggleAllowNetbootFalse, Status: metav1.ConditionTrue},
			},
			Condition: WorkflowCondition{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionFalse},
		},
		"append new condition": {
			ExistingConditions: []WorkflowCondition{
				{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionTrue},
			},
			WantConditions: []WorkflowCondition{
				{Type: ToggleAllowNetbootTrue, Status: metav1.ConditionTrue},
				{Type: ToggleAllowNetbootFalse, Status: metav1.ConditionFalse},
			},
			Condition: WorkflowCondition{Type: ToggleAllowNetbootFalse, Status: metav1.ConditionFalse},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w := &WorkflowStatus{
				Conditions: tt.ExistingConditions,
			}
			w.SetCondition(tt.Condition)
			if !cmp.Equal(tt.WantConditions, w.Conditions) {
				t.Errorf("SetCondition() mismatch (-want +got):\n%s", cmp.Diff(tt.WantConditions, w.Conditions))
			}
		})
	}
}
