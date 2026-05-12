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

package bmc

import (
	"testing"
)

func TestHasConditionStatus(t *testing.T) {
	tests := map[string]struct {
		existingConditions []Condition
		conditionType      ConditionType
		conditionStatus    ConditionStatus
		expected           bool
	}{
		"condition exists with matching status": {
			existingConditions: []Condition{
				{
					Type:   ConditionTypeMachineContactable,
					Status: ConditionStatusTrue,
				},
			},
			conditionType:   ConditionTypeMachineContactable,
			conditionStatus: ConditionStatusTrue,
			expected:        true,
		},
		"condition exists with non-matching status": {
			existingConditions: []Condition{
				{
					Type:   ConditionTypeMachineContactable,
					Status: ConditionStatusFalse,
				},
			},
			conditionType:   ConditionTypeMachineContactable,
			conditionStatus: ConditionStatusTrue,
			expected:        false,
		},
		"condition does not exist": {
			existingConditions: []Condition{
				{
					Type:   ConditionTypeMachineContactable,
					Status: ConditionStatusTrue,
				},
			},
			conditionType:   ConditionTypeMachinePowerState,
			conditionStatus: ConditionStatusOn,
			expected:        false,
		},
		"empty conditions slice": {
			existingConditions: []Condition{},
			conditionType:      ConditionTypeMachineContactable,
			conditionStatus:    ConditionStatusTrue,
			expected:           false,
		},
		"nil conditions slice": {
			existingConditions: nil,
			conditionType:      ConditionTypeMachineContactable,
			conditionStatus:    ConditionStatusTrue,
			expected:           false,
		},
		"multiple conditions with match": {
			existingConditions: []Condition{
				{
					Type:   ConditionTypeMachineContactable,
					Status: ConditionStatusFalse,
				},
				{
					Type:   ConditionTypeMachinePowerState,
					Status: ConditionStatusOn,
				},
			},
			conditionType:   ConditionTypeMachinePowerState,
			conditionStatus: ConditionStatusOn,
			expected:        true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := HasConditionStatus(tc.existingConditions, tc.conditionType, tc.conditionStatus)
			if got != tc.expected {
				t.Errorf("HasConditionStatus() = %v, want %v", got, tc.expected)
			}
		})
	}
}
