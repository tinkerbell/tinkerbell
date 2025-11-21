package tinkerbell

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetCondition(t *testing.T) {
	tests := map[string]struct {
		ExistingConditions []PipelineCondition
		WantConditions     []PipelineCondition
		Condition          PipelineCondition
	}{
		"update existing condition": {
			ExistingConditions: []PipelineCondition{
				{Type: ToggleNetbootTrue, Status: metav1.ConditionTrue},
				{Type: ToggleNetbootFalse, Status: metav1.ConditionTrue},
			},
			WantConditions: []PipelineCondition{
				{Type: ToggleNetbootTrue, Status: metav1.ConditionFalse},
				{Type: ToggleNetbootFalse, Status: metav1.ConditionTrue},
			},
			Condition: PipelineCondition{Type: ToggleNetbootTrue, Status: metav1.ConditionFalse},
		},
		"append new condition": {
			ExistingConditions: []PipelineCondition{
				{Type: ToggleNetbootTrue, Status: metav1.ConditionTrue},
			},
			WantConditions: []PipelineCondition{
				{Type: ToggleNetbootTrue, Status: metav1.ConditionTrue},
				{Type: ToggleNetbootFalse, Status: metav1.ConditionFalse},
			},
			Condition: PipelineCondition{Type: ToggleNetbootFalse, Status: metav1.ConditionFalse},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w := &PipelineStatus{
				Conditions: tt.ExistingConditions,
			}
			w.SetCondition(tt.Condition)
			if !cmp.Equal(tt.WantConditions, w.Conditions) {
				t.Errorf("SetCondition() mismatch (-want +got):\n%s", cmp.Diff(tt.WantConditions, w.Conditions))
			}
		})
	}
}
