/*
Copyright 2022 The Tinkerbell Authors.

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

package machine

import (
	"testing"

	. "github.com/onsi/gomega" //nolint:revive // one day we will remove gomega
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func readyMachine() *clusterv1.Machine {
	version := "foo"
	dataSecretName := "bar"

	return &clusterv1.Machine{
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: &dataSecretName,
			},
			Version: &version,
		},
	}
}

func Test_Machine(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		mutateF       func(m *clusterv1.Machine) *clusterv1.Machine
		expectError   bool
		expectedError error
		ready         bool
	}{
		"is_not_ready_when_it_is_nil": {
			mutateF: func(_ *clusterv1.Machine) *clusterv1.Machine {
				return nil
			},
		},
		"is_not_ready_when_bootstrap_reference_is_not_set": {
			mutateF: func(m *clusterv1.Machine) *clusterv1.Machine {
				m.Spec.Bootstrap.DataSecretName = nil

				return m
			},
		},
		"is_not_valid_when_version_is_not_set": {
			mutateF: func(m *clusterv1.Machine) *clusterv1.Machine {
				m.Spec.Version = nil

				return m
			},
			expectError:   true,
			expectedError: ErrMachineVersionEmpty,
		},
		"is_not_valid_when_version_is_empty": {
			mutateF: func(m *clusterv1.Machine) *clusterv1.Machine {
				v := ""
				m.Spec.Version = &v

				return m
			},
			expectError:   true,
			expectedError: ErrMachineVersionEmpty,
		},
		"is_ready_when_all_requirements_are_met": {
			ready: true,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			m := readyMachine()

			if c.mutateF != nil {
				m = c.mutateF(m)
			}

			reason, err := isMachineReady(m)
			if c.expectError {
				g.Expect(err).To(MatchError(c.expectedError))

				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			if c.ready {
				g.Expect(reason).To(BeEmpty(), "Expected ready machine")

				return
			}

			// TODO: should we match reason here?
			g.Expect(reason).NotTo(BeEmpty(), "Expect machine to not be ready")
		})
	}
}
