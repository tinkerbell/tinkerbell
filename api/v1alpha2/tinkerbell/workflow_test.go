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
				{Type: ToggleNetbootTrue, Status: metav1.ConditionTrue},
				{Type: ToggleNetbootFalse, Status: metav1.ConditionTrue},
			},
			WantConditions: []WorkflowCondition{
				{Type: ToggleNetbootTrue, Status: metav1.ConditionFalse},
				{Type: ToggleNetbootFalse, Status: metav1.ConditionTrue},
			},
			Condition: WorkflowCondition{Type: ToggleNetbootTrue, Status: metav1.ConditionFalse},
		},
		"append new condition": {
			ExistingConditions: []WorkflowCondition{
				{Type: ToggleNetbootTrue, Status: metav1.ConditionTrue},
			},
			WantConditions: []WorkflowCondition{
				{Type: ToggleNetbootTrue, Status: metav1.ConditionTrue},
				{Type: ToggleNetbootFalse, Status: metav1.ConditionFalse},
			},
			Condition: WorkflowCondition{Type: ToggleNetbootFalse, Status: metav1.ConditionFalse},
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
