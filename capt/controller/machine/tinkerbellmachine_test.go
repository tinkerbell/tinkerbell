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

package machine_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	. "github.com/onsi/gomega" //nolint:revive // one day we will remove gomega
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/tinkerbell/tinkerbell/capt/controller/machine"
	tinkv1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"

	infrastructurev1 "github.com/tinkerbell/tinkerbell/pkg/api/v1beta1/capt"
)

const (
	machineName           = "myMachineName"
	tinkerbellMachineName = "myTinkerbellMachineName"
	clusterName           = "myClusterName"
	clusterNamespace      = "myClusterNamespace"
	hardwareIP            = "1.1.1.1"
	hardwareName          = "myHardwareName"
)

func notImplemented(t *testing.T) {
	t.Helper()
	t.Parallel()

	// t.Fatalf("not implemented")
	t.Skip("not implemented")
}

type testOptions struct {
	// Labels allow providing labels for the machine
	Labels           map[string]string
	HardwareAffinity *infrastructurev1.HardwareAffinity
}

func validTinkerbellMachine(name, namespace, machineName, hardwareUUID string, options ...testOptions) *infrastructurev1.TinkerbellMachine { //nolint:unparam // better to have flexible now, than to change it later.
	m := &infrastructurev1.TinkerbellMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(hardwareUUID),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Machine",
					Name:       machineName,
					UID:        types.UID(hardwareUUID),
				},
			},
		},
	}

	for _, o := range options {
		for k, v := range o.Labels {
			if m.Labels == nil {
				m.Labels = map[string]string{}
			}

			m.Labels[k] = v
		}

		if o.HardwareAffinity != nil {
			m.Spec.HardwareAffinity = o.HardwareAffinity
		}
	}

	return m
}

func validCluster(name, namespace string) *clusterv1.Cluster { //nolint:unparam // better to have flexible now, than to change it later.
	return &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{
				Name: name,
			},
		},
	}
}

func validTinkerbellCluster(name, namespace string) *infrastructurev1.TinkerbellCluster { //nolint:unparam // better to have flexible now, than to change it later.
	tinkCluster := &infrastructurev1.TinkerbellCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: []string{infrastructurev1.ClusterFinalizer},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "cluster.x-k8s.io/v1beta1",
					Kind:       "Cluster",
					Name:       name,
				},
			},
		},
		Spec: infrastructurev1.TinkerbellClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: hardwareIP,
				Port: 6443,
			},
		},
		Status: infrastructurev1.TinkerbellClusterStatus{
			Ready: true,
		},
	}

	tinkCluster.Default()

	return tinkCluster
}

func validMachine(name, namespace, clusterName string) *clusterv1.Machine { //nolint:unparam // better to have flexible now, than to change it later.
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: clusterName,
			},
		},
		Spec: clusterv1.MachineSpec{
			Version: ptr.To[string]("1.19.4"),
			Bootstrap: clusterv1.Bootstrap{
				DataSecretName: ptr.To[string](name),
			},
		},
	}
}

func validSecret(name, namespace string) *corev1.Secret { //nolint:unparam // better to have flexible now, than to change it later.
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"value": []byte("not nil bootstrap data"),
		},
	}
}

func validHardware(name, uid, ip string, options ...testOptions) *tinkv1.Hardware {
	hw := &tinkv1.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: clusterNamespace,
			UID:       types.UID(uid),
		},
		Spec: tinkv1.HardwareSpec{
			Disks: []tinkv1.Disk{
				{
					Device: "/dev/sda",
				},
			},
			Interfaces: []tinkv1.Interface{
				{
					DHCP: &tinkv1.DHCP{
						IP: &tinkv1.IP{
							Address: ip,
						},
					},
					Netboot: &tinkv1.Netboot{
						AllowPXE: ptr.To(true),
					},
				},
			},
			Metadata: &tinkv1.HardwareMetadata{
				Instance: &tinkv1.MetadataInstance{
					ID: ip,
				},
			},
		},
	}

	for _, o := range options {
		for k, v := range o.Labels {
			if hw.Labels == nil {
				hw.Labels = map[string]string{}
			}

			hw.Labels[k] = v
		}
	}

	return hw
}

func validTemplate(name, namespace string) *tinkv1.Template {
	tmpl := `version: "0.1"
name: ubuntu_provisioning
global_timeout: 6000
tasks:
  - name: "os-installation"
	worker: "{{.device_1}}"
	volumes:
	  - /dev:/dev
	  - /dev/console:/dev/console
	  - /lib/firmware:/lib/firmware:ro
	actions:
	  - name: "disk-wipe"
		image: disk-wipe
		timeout: 90`

	return &tinkv1.Template{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: tinkv1.TemplateSpec{
			Data: &tmpl,
		},
	}
}

func validWorkflow(name, namespace string) *tinkv1.Workflow {
	return &tinkv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: tinkv1.WorkflowSpec{
			TemplateRef: name,
		},
		Status: tinkv1.WorkflowStatus{
			State: tinkv1.WorkflowStateSuccess,
			Tasks: []tinkv1.Task{
				{
					Name: name,
					Actions: []tinkv1.Action{
						{
							Name:  name,
							State: tinkv1.WorkflowStateSuccess,
						},
					},
				},
			},
		},
	}
}

func kubernetesClientWithObjects(t *testing.T, objects []runtime.Object) client.Client {
	t.Helper()
	g := NewWithT(t)

	scheme := runtime.NewScheme()

	g.Expect(tinkv1.AddToScheme(scheme)).To(Succeed(), "Adding Tinkerbell objects to scheme should succeed")
	g.Expect(infrastructurev1.AddToScheme(scheme)).To(Succeed(), "Adding Tinkerbell CAPI objects to scheme should succeed")
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed(), "Adding CAPI objects to scheme should succeed")
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed(), "Adding Core V1 objects to scheme should succeed")

	objs := []client.Object{
		&infrastructurev1.TinkerbellMachine{},
		&infrastructurev1.TinkerbellCluster{},
	}

	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).WithStatusSubresource(objs...).Build()
}

func Test_Machine_reconciliation_with_available_hardware(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred(), "Unexpected reconciliation error")

	ctx := context.Background()

	globalResourceName := types.NamespacedName{
		Name:      tinkerbellMachineName,
		Namespace: clusterNamespace,
	}

	t.Run("creates_template", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		template := &tinkv1.Template{}

		g.Expect(cl.Get(ctx, globalResourceName, template)).To(Succeed(), "Expected template to be created")

		// Owner reference is required to make use of Kubernetes GC for removing dependent objects, so if
		// machine gets force-removed, template will be cleaned up.
		t.Run("with_owner_reference_set", func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(template.ObjectMeta.OwnerReferences).NotTo(BeEmpty(), "Expected at least one owner reference to be set")

			g.Expect(template.ObjectMeta.OwnerReferences[0].UID).To(BeEquivalentTo(types.UID(hardwareUUID)),
				"Expected owner reference UID to match hardwareUUID")
		})
	})

	t.Run("creates_workflow", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		workflow := &tinkv1.Workflow{}

		g.Expect(cl.Get(ctx, globalResourceName, workflow)).To(Succeed(), "Expected workflow to be created")

		// Owner reference is required to make use of Kubernetes GC for removing dependent objects, so if
		// machine gets force-removed, workflow will be cleaned up.
		t.Run("with_owner_reference_set", func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(workflow.ObjectMeta.OwnerReferences).NotTo(BeEmpty(), "Expected at least one owner reference to be set")

			g.Expect(workflow.ObjectMeta.OwnerReferences[0].Name).To(BeEquivalentTo(tinkerbellMachineName),
				"Expected owner reference name to match tinkerbellMachine name")
		})
	})

	namespacedName := types.NamespacedName{
		Name:      tinkerbellMachineName,
		Namespace: clusterNamespace,
	}

	updatedMachine := &infrastructurev1.TinkerbellMachine{}
	g.Expect(cl.Get(ctx, namespacedName, updatedMachine)).To(Succeed())

	// From https://cluster-api.sigs.k8s.io/developer/providers/machine-infrastructure.html#normal-resource.
	t.Run("sets_provider_id_with_selected_hardware_id", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		g.Expect(updatedMachine.Spec.ProviderID).To(Equal(fmt.Sprintf("tinkerbell://%s/%s", clusterNamespace, hardwareName)),
			"Expected ProviderID field to include hardwareUUID")
	})

	// From https://cluster-api.sigs.k8s.io/developer/providers/machine-infrastructure.html#normal-resource.
	t.Run("sets_tinkerbell_finalizer", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		g.Expect(updatedMachine.ObjectMeta.Finalizers).NotTo(BeEmpty(), "Expected at least one finalizer to be set")
	})

	// From https://cluster-api.sigs.k8s.io/developer/providers/machine-infrastructure.html#normal-resource.
	t.Run("sets_tinkerbell_machine_IP_address", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		g.Expect(updatedMachine.Status.Addresses).NotTo(BeEmpty(), "Expected at least one IP address to be populated")

		g.Expect(updatedMachine.Status.Addresses[0].Address).To(BeEquivalentTo(hardwareIP),
			"Expected first IP address to be %q", hardwareIP)
	})

	// So it becomes unavailable for other clusters.
	t.Run("sets_ownership_label_on_selected_hardware", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		hardwareNamespacedName := types.NamespacedName{
			Name:      hardwareName,
			Namespace: clusterNamespace,
		}

		updatedHardware := &tinkv1.Hardware{}
		g.Expect(cl.Get(ctx, hardwareNamespacedName, updatedHardware)).To(Succeed())

		g.Expect(updatedHardware.ObjectMeta.Labels).To(
			HaveKeyWithValue(machine.HardwareOwnerNameLabel, tinkerbellMachineName),
			"Expected owner name label to be set on Hardware")

		g.Expect(updatedHardware.ObjectMeta.Labels).To(
			HaveKeyWithValue(machine.HardwareOwnerNamespaceLabel, clusterNamespace),
			"Expected owner namespace label to be set on Hardware")
	})

	// Ensure idempotency of reconcile operation. E.g. we shouldn't try to create the template with the same name
	// on every iteration.
	t.Run("succeeds_when_executed_twice", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
		g.Expect(err).NotTo(HaveOccurred(), "Unexpected reconciliation error")
	})

	// Status should be updated on every run.
	//
	// Don't execute this test in parallel, as we reset status here.
	t.Run("refreshes_status_when_machine_is_already_provisioned", func(t *testing.T) {
		updatedMachine.Status.Addresses = nil
		g := NewWithT(t)

		g.Expect(cl.Update(context.Background(), updatedMachine)).To(Succeed())
		_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
		g.Expect(err).NotTo(HaveOccurred())

		updatedMachine = &infrastructurev1.TinkerbellMachine{}
		g.Expect(cl.Get(ctx, namespacedName, updatedMachine)).To(Succeed())
		g.Expect(updatedMachine.Status.Addresses).NotTo(BeEmpty(), "Machine status should be updated on every reconciliation")
	})

	t.Run("allowPXE_is_not_updated", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		hardwareNamespacedName := types.NamespacedName{
			Name:      hardwareName,
			Namespace: clusterNamespace,
		}

		updatedHardware := &tinkv1.Hardware{}
		g.Expect(cl.Get(ctx, hardwareNamespacedName, updatedHardware)).To(Succeed())

		if diff := cmp.Diff(updatedHardware.Spec.Interfaces[0].Netboot.AllowPXE, ptr.To(true)); diff != "" {
			t.Error(diff)
		}
	})
}

func Test_Machine_reconciliation_workflow_complete(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
		validTemplate(tinkerbellMachineName, clusterNamespace),
		validWorkflow(tinkerbellMachineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred(), "Unexpected reconciliation error")

	ctx := context.Background()

	namespacedName := types.NamespacedName{
		Name:      tinkerbellMachineName,
		Namespace: clusterNamespace,
	}

	updatedMachine := &infrastructurev1.TinkerbellMachine{}
	g.Expect(cl.Get(ctx, namespacedName, updatedMachine)).To(Succeed())

	// From https://cluster-api.sigs.k8s.io/developer/providers/machine-infrastructure.html#normal-resource.
	t.Run("sets_provider_id_with_selected_hardware_id", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		g.Expect(updatedMachine.Spec.ProviderID).To(Equal(fmt.Sprintf("tinkerbell://%s/%s", clusterNamespace, hardwareName)),
			"Expected ProviderID field to include hardwareUUID")
	})

	// From https://cluster-api.sigs.k8s.io/developer/providers/machine-infrastructure.html#normal-resource.
	t.Run("sets_tinkerbell_machine_status_to_ready", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		g.Expect(updatedMachine.Status.Ready).To(BeTrue(), "Machine is not ready")
	})

	// From https://cluster-api.sigs.k8s.io/developer/providers/machine-infrastructure.html#normal-resource.
	t.Run("sets_tinkerbell_finalizer", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		g.Expect(updatedMachine.ObjectMeta.Finalizers).NotTo(BeEmpty(), "Expected at least one finalizer to be set")
	})

	// From https://cluster-api.sigs.k8s.io/developer/providers/machine-infrastructure.html#normal-resource.
	t.Run("sets_tinkerbell_machine_IP_address", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		g.Expect(updatedMachine.Status.Addresses).NotTo(BeEmpty(), "Expected at least one IP address to be populated")

		g.Expect(updatedMachine.Status.Addresses[0].Address).To(BeEquivalentTo(hardwareIP),
			"Expected first IP address to be %q", hardwareIP)
	})

	// So it becomes unavailable for other clusters.
	t.Run("sets_ownership_label_on_selected_hardware", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		hardwareNamespacedName := types.NamespacedName{
			Name:      hardwareName,
			Namespace: clusterNamespace,
		}

		updatedHardware := &tinkv1.Hardware{}
		g.Expect(cl.Get(ctx, hardwareNamespacedName, updatedHardware)).To(Succeed())

		g.Expect(updatedHardware.ObjectMeta.Labels).To(
			HaveKeyWithValue(machine.HardwareOwnerNameLabel, tinkerbellMachineName),
			"Expected owner name label to be set on Hardware")

		g.Expect(updatedHardware.ObjectMeta.Labels).To(
			HaveKeyWithValue(machine.HardwareOwnerNamespaceLabel, clusterNamespace),
			"Expected owner namespace label to be set on Hardware")
	})

	// Ensure idempotency of reconcile operation. E.g. we shouldn't try to create the template with the same name
	// on every iteration.
	t.Run("succeeds_when_executed_twice", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
		g.Expect(err).NotTo(HaveOccurred(), "Unexpected reconciliation error")
	})

	// Status should be updated on every run.
	//
	// Don't execute this test in parallel, as we reset status here.
	t.Run("refreshes_status_when_machine_is_already_provisioned", func(t *testing.T) {
		updatedMachine.Status.Addresses = nil
		g := NewWithT(t)

		g.Expect(cl.Update(context.Background(), updatedMachine)).To(Succeed())
		_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
		g.Expect(err).NotTo(HaveOccurred())

		updatedMachine = &infrastructurev1.TinkerbellMachine{}
		g.Expect(cl.Get(ctx, namespacedName, updatedMachine)).To(Succeed())
		g.Expect(updatedMachine.Status.Addresses).NotTo(BeEmpty(), "Machine status should be updated on every reconciliation")
	})
}

func Test_Machine_reconciliation(t *testing.T) {
	t.Parallel()

	t.Run("is_not_requeued_when", func(t *testing.T) {
		t.Parallel()

		// Requeue will be handled when resource is created.
		t.Run("is_requeued_when_machine_object_is_missing",
			machineReconciliationIsRequeuedWhenTinkerbellMachineObjectIsMissing)

		// From https://cluster-api.sigs.k8s.io/developer/providers/cluster-infrastructure.html#behavior
		// Requeue will be handled when ownerRef is set
		t.Run("machine_has_no_owner_set", machineReconciliationIsRequeuedWhenTinkerbellMachineHasNoOwnerSet)

		// From https://cluster-api.sigs.k8s.io/developer/providers/cluster-infrastructure.html#behavior
		// Requeue will be handled when bootstrap secret is set through the Watch on Machines
		t.Run("bootstrap_secret_is_not_ready", machineReconciliationIsRequeuedWhenBootstrapSecretIsNotReady)

		// From https://cluster-api.sigs.k8s.io/developer/providers/cluster-infrastructure.html#behavior
		// Requeue will be handled when bootstrap secret is set through the Watch on Clusters
		t.Run("cluster_infrastructure_is_not_ready", machineReconciliationIsRequeuedWhenClusterInfrastructureIsNotReady)
	})

	t.Run("fails_when", func(t *testing.T) {
		t.Parallel()

		t.Run("reconciler_is_nil", machineReconciliationPanicsWhenReconcilerIsNil)
		t.Run("reconciler_has_no_client_set", machineReconciliationPanicsWhenReconcilerHasNoClientSet)

		// CAPI spec says this is optional, but @detiber says it's effectively required, so treat it as so.
		t.Run("machine_has_no_version_set", machineReconciliationFailsWhenMachineHasNoVersionSet)

		t.Run("associated_cluster_object_does_not_exist",
			machineReconciliationFailsWhenAssociatedClusterObjectDoesNotExist)

		t.Run("associated_tinkerbell_cluster_object_does_not_exist",
			machineReconciliationFailsWhenAssociatedTinkerbellClusterObjectDoesNotExist)

		// If for example CAPI changes key used to store bootstrap date, we shouldn't try to create machines
		// with empty bootstrap config, we should fail early instead.
		t.Run("bootstrap_config_is_empty", machineReconciliationFailsWhenBootstrapConfigIsEmpty)
		t.Run("bootstrap_config_has_no_value_key", machineReconciliationFailsWhenBootstrapConfigHasNoValueKey)

		t.Run("there_is_no_hardware_available", machineReconciliationFailsWhenThereIsNoHardwareAvailable)

		t.Run("selected_hardware_has_no_ip_address_set", machineReconciliationFailsWhenSelectedHardwareHasNoIPAddressSet)
	})

	// Single hardware should only ever be used for a single machine.
	t.Run("selects_unique_and_available_hardware_for_each_machine",
		machineReconciliationSelectsUniqueAndAvailablehardwareForEachMachine)

	t.Run("selects_unique_and_available_hardware_for_each_machine_filtering_by_required_hardware_affinity",
		machineReconciliationSelectsUniqueAndAvailablehardwareForEachMachineFilteringByRequiredHardwareAffinity)

	t.Run("selects_unique_and_available_hardware_for_each_machine_filtering_by_preferred_hardware_affinity",
		machineReconciliationSelectsUniqueAndAvailablehardwareForEachMachineFilteringByPreferredHardwareAffinity)

	t.Run("selects_unique_and_available_hardware_for_each_machine_filtering_by_required_and_preferred_hardware_affinity",
		machineReconciliationSelectsUniqueAndAvailablehardwareForEachMachineFilteringByRequiredAndPreferredHardwareAffinity)

	// Patching Hardware and TinkerbellMachine are not atomic operations, so we should handle situation, when
	// misspelling process is aborted in the middle.
	//
	// Without that, new Hardware will be selected each time.
	t.Run("uses_already_selected_hardware_if_patching_tinkerbell_machine_failed",
		machineReconciliationUsesAlreadySelectedHardwareIfPatchingTinkerbellMachineFailed)

	t.Run("when_machine_is_scheduled_for_removal_it", func(t *testing.T) {
		t.Parallel()

		// From https://cluster-api.sigs.k8s.io/developer/providers/machine-infrastructure.html#behavior
		t.Run("removes_tinkerbell_finalizer", notImplemented)

		// Removing machine should release used hardware.
		t.Run("marks_hardware_as_available_for_other_machines", notImplemented)
	})
}

func Test_Machine_reconciliation_when_machine_is_scheduled_for_removal_it(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, ""),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, uuid.New().String(), hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	ctx := context.Background()

	tinkerbellMachineNamespacedName := types.NamespacedName{
		Name:      tinkerbellMachineName,
		Namespace: clusterNamespace,
	}

	updatedMachine := &infrastructurev1.TinkerbellMachine{}
	g.Expect(cl.Get(ctx, tinkerbellMachineNamespacedName, updatedMachine)).To(Succeed())

	g.Expect(cl.Delete(ctx, updatedMachine)).To(Succeed())
	_, err = reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	hardwareNamespacedName := types.NamespacedName{
		Name:      hardwareName,
		Namespace: clusterNamespace,
	}

	updatedHardware := &tinkv1.Hardware{}
	g.Expect(cl.Get(ctx, hardwareNamespacedName, updatedHardware)).To(Succeed())

	t.Run("removes_tinkerbell_machine_finalizer_from_hardware", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		g.Expect(updatedHardware.ObjectMeta.GetFinalizers()).To(BeEmpty())
	})

	t.Run("makes_hardware_available_for_other_machines", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		g.Expect(updatedHardware.ObjectMeta.Labels).NotTo(HaveKey(machine.HardwareOwnerNameLabel),
			"Found hardware owner name label")
		g.Expect(updatedHardware.ObjectMeta.Labels).NotTo(HaveKey(machine.HardwareOwnerNamespaceLabel),
			"Found hardware owner namespace label")
	})
}

func machineReconciliationPanicsWhenReconcilerIsNil(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	var machineController *machine.TinkerbellMachineReconciler

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: clusterNamespace,
			Name:      tinkerbellMachineName,
		},
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic but received none")
		}
	}()

	_, err := machineController.Reconcile(context.TODO(), request)
	g.Expect(err).To(MatchError(machine.ErrConfigurationNil))
}

func machineReconciliationPanicsWhenReconcilerHasNoClientSet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	machineController := &machine.TinkerbellMachineReconciler{}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: clusterNamespace,
			Name:      tinkerbellMachineName,
		},
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic but received none")
		}
	}()

	_, err := machineController.Reconcile(context.TODO(), request)
	g.Expect(err).To(MatchError(machine.ErrMissingClient))
}

func reconcileMachineWithClient(cc client.Client, name, namespace string) (ctrl.Result, error) { //nolint:unparam // better to have flexible now, than to change it later.
	machineController := &machine.TinkerbellMachineReconciler{
		Client: cc,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	r, err := machineController.Reconcile(context.TODO(), request)
	if err != nil {
		return r, fmt.Errorf("error with Reconcile: %w", err)
	}

	return r, nil
}

func machineReconciliationIsRequeuedWhenTinkerbellMachineObjectIsMissing(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	result, err := reconcileMachineWithClient(kubernetesClientWithObjects(t, nil), tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred(), "Reconciling when machine object does not exist should not return error")
	g.Expect(result.IsZero()).To(BeTrue(), "Expected no requeue to be requested")
}

func machineReconciliationIsRequeuedWhenTinkerbellMachineHasNoOwnerSet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	hardwareUUID := uuid.New().String()
	tinkerbellMachine := validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID)
	tinkerbellMachine.OwnerReferences = nil

	objects := []runtime.Object{
		tinkerbellMachine,
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	result, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred(), "Reconciling when machine object does not exist should not return error")
	g.Expect(result.IsZero()).To(BeTrue(), "Expected no requeue to be requested")
}

func machineReconciliationIsRequeuedWhenBootstrapSecretIsNotReady(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()
	machineWithoutSecretReference := validMachine(machineName, clusterNamespace, clusterName)
	machineWithoutSecretReference.Spec.Bootstrap.DataSecretName = nil

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		machineWithoutSecretReference,
		validSecret(machineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	result, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue(), "Expected no requeue to be requested")
}

func machineReconciliationIsRequeuedWhenClusterInfrastructureIsNotReady(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()
	notReadyTinkerbellCluster := validTinkerbellCluster(clusterName, clusterNamespace)
	notReadyTinkerbellCluster.Status.Ready = false

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		notReadyTinkerbellCluster,
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	result, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.IsZero()).To(BeTrue(), "Expected no requeue to be requested")
}

func machineReconciliationFailsWhenMachineHasNoVersionSet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()
	machineWithoutVersion := validMachine(machineName, clusterNamespace, clusterName)
	machineWithoutVersion.Spec.Version = nil

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		machineWithoutVersion,
		validSecret(machineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).To(MatchError(machine.ErrMachineVersionEmpty))
}

func machineReconciliationFailsWhenBootstrapConfigIsEmpty(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()
	emptySecret := validSecret(machineName, clusterNamespace)
	emptySecret.Data["value"] = nil

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		emptySecret,
	}

	_, err := reconcileMachineWithClient(kubernetesClientWithObjects(t, objects), tinkerbellMachineName, clusterNamespace)
	g.Expect(err).To(MatchError(machine.ErrBootstrapUserDataEmpty))
}

func machineReconciliationFailsWhenBootstrapConfigHasNoValueKey(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()
	emptySecret := validSecret(machineName, clusterNamespace)
	emptySecret.Data = nil

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		emptySecret,
	}

	_, err := reconcileMachineWithClient(kubernetesClientWithObjects(t, objects), tinkerbellMachineName, clusterNamespace)
	g.Expect(err).To(MatchError(machine.ErrMissingBootstrapDataSecretValueKey))
}

func machineReconciliationFailsWhenAssociatedClusterObjectDoesNotExist(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()
	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		// validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
	}

	_, err := reconcileMachineWithClient(kubernetesClientWithObjects(t, objects), tinkerbellMachineName, clusterNamespace)
	g.Expect(err).To(SatisfyAll(
		MatchError(ContainSubstring("not found")),
		MatchError(ContainSubstring("getting cluster from metadata")),
	))
}

func machineReconciliationFailsWhenAssociatedTinkerbellClusterObjectDoesNotExist(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()
	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		// validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
	}

	_, err := reconcileMachineWithClient(kubernetesClientWithObjects(t, objects), tinkerbellMachineName, clusterNamespace)
	g.Expect(err).To(MatchError(ContainSubstring("getting TinkerbellCluster object")))
}

func machineReconciliationFailsWhenThereIsNoHardwareAvailable(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()
	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		// validHardware(hardwareName, hardwareUUID, hardwareIP),
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
	}

	_, err := reconcileMachineWithClient(kubernetesClientWithObjects(t, objects), tinkerbellMachineName, clusterNamespace)
	g.Expect(err).To(MatchError(machine.ErrNoHardwareAvailable))
}

func machineReconciliationFailsWhenSelectedHardwareHasNoIPAddressSet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	hardwareUUID := uuid.New().String()
	malformedHardware := validHardware(hardwareName, hardwareUUID, "")
	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		malformedHardware,
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
	}

	_, err := reconcileMachineWithClient(kubernetesClientWithObjects(t, objects), tinkerbellMachineName, clusterNamespace)
	g.Expect(err).To(MatchError(machine.ErrHardwareFirstInterfaceDHCPMissingIP))
}

func machineReconciliationSelectsUniqueAndAvailablehardwareForEachMachine(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	secondMachineName := "secondMachineName"
	secondHardwareName := "secondHardwareName"

	firstHardwareUUID := uuid.New().String()
	secondHardwareUUID := uuid.New().String()

	secondTinkerbellMachineName := "secondTinkerbellMachineName"

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, firstHardwareUUID),
		validTinkerbellMachine(secondTinkerbellMachineName, clusterNamespace, secondMachineName, secondHardwareUUID),

		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),

		validHardware(hardwareName, firstHardwareUUID, hardwareIP),
		validHardware(secondHardwareName, secondHardwareUUID, "2.2.2.2"),

		validMachine(machineName, clusterNamespace, clusterName),
		validMachine(secondMachineName, clusterNamespace, clusterName),

		validSecret(machineName, clusterNamespace),
		validSecret(secondMachineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	tinkerbellMachineNamespacedName := types.NamespacedName{
		Name:      tinkerbellMachineName,
		Namespace: clusterNamespace,
	}

	ctx := context.Background()

	firstMachine := &infrastructurev1.TinkerbellMachine{}
	g.Expect(cl.Get(ctx, tinkerbellMachineNamespacedName, firstMachine)).To(Succeed(), "Getting first updated machine")

	_, err = reconcileMachineWithClient(cl, secondTinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	tinkerbellMachineNamespacedName.Name = secondTinkerbellMachineName

	secondMachine := &infrastructurev1.TinkerbellMachine{}
	g.Expect(cl.Get(ctx, tinkerbellMachineNamespacedName, secondMachine)).To(Succeed())

	g.Expect(firstMachine.Spec.HardwareName).NotTo(BeEquivalentTo(secondMachine.Spec.HardwareName),
		"Two machines use the same hardware")
}

func machineReconciliationUsesAlreadySelectedHardwareIfPatchingTinkerbellMachineFailed(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	expectedHardwareName := "alreadyOwnedHardware"
	alreadyOwnedHardware := validHardware(expectedHardwareName, uuid.New().String(), "2.2.2.2")
	alreadyOwnedHardware.Labels = map[string]string{
		machine.HardwareOwnerNameLabel:      tinkerbellMachineName,
		machine.HardwareOwnerNamespaceLabel: clusterNamespace,
	}

	hardwareUUID := uuid.New().String()

	objects := []runtime.Object{
		validTinkerbellMachine(tinkerbellMachineName, clusterNamespace, machineName, hardwareUUID),
		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),
		validHardware(hardwareName, hardwareUUID, hardwareIP),
		alreadyOwnedHardware,
		validMachine(machineName, clusterNamespace, clusterName),
		validSecret(machineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	_, err := reconcileMachineWithClient(cl, tinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	ctx := context.Background()

	tinkerbellMachineNamespacedName := types.NamespacedName{
		Name:      tinkerbellMachineName,
		Namespace: clusterNamespace,
	}

	updatedMachine := &infrastructurev1.TinkerbellMachine{}
	g.Expect(cl.Get(ctx, tinkerbellMachineNamespacedName, updatedMachine)).To(Succeed())

	g.Expect(updatedMachine.Spec.HardwareName).To(BeEquivalentTo(expectedHardwareName),
		"Wrong hardware selected. Expected %q", expectedHardwareName)
}

func machineReconciliationSelectsUniqueAndAvailablehardwareForEachMachineFilteringByRequiredHardwareAffinity(t *testing.T) {
	machineReconciliationHardwareAffinityHelper(t, testOptions{
		HardwareAffinity: &infrastructurev1.HardwareAffinity{
			Required: []infrastructurev1.HardwareAffinityTerm{
				{
					LabelSelector: metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "rack",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"foo"},
							},
						},
					},
				},
			},
		},
	}, testOptions{
		HardwareAffinity: &infrastructurev1.HardwareAffinity{
			Required: []infrastructurev1.HardwareAffinityTerm{
				{
					LabelSelector: metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "rack",
								Operator: metav1.LabelSelectorOpNotIn,
								Values:   []string{"foo", "baz"},
							},
						},
					},
				},
			},
		},
	},
		testOptions{
			HardwareAffinity: &infrastructurev1.HardwareAffinity{
				Required: []infrastructurev1.HardwareAffinityTerm{
					// required terms are OR'd, so this one shouldn't affect the results
					{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "non-existent"},
						},
					},
					{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "baz"},
						},
					},
				},
			},
		})
}

func machineReconciliationSelectsUniqueAndAvailablehardwareForEachMachineFilteringByPreferredHardwareAffinity(t *testing.T) {
	machineReconciliationHardwareAffinityHelper(t, testOptions{
		HardwareAffinity: &infrastructurev1.HardwareAffinity{
			Preferred: []infrastructurev1.WeightedHardwareAffinityTerm{
				{
					Weight: 12,
					HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "foo"},
						},
					},
				},
				{
					Weight: 10,
					HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "bar"},
						},
					},
				},
				{
					Weight: 11,
					HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "baz"},
						},
					},
				},
			},
		},
	},
		testOptions{
			HardwareAffinity: &infrastructurev1.HardwareAffinity{
				Preferred: []infrastructurev1.WeightedHardwareAffinityTerm{
					{
						Weight: 50,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"rack": "bar"},
							},
						},
					},
					{
						Weight: 49,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"rack": "baz"},
							},
						},
					},
					{
						Weight: 49,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "rack",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"foo"},
									},
								},
							},
						},
					},
				},
			},
		}, testOptions{
			HardwareAffinity: &infrastructurev1.HardwareAffinity{
				Preferred: []infrastructurev1.WeightedHardwareAffinityTerm{
					{
						Weight: 91,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "rack",
										Operator: metav1.LabelSelectorOpNotIn,
										Values:   []string{"bar", "foo"},
									},
								},
							},
						},
					},
					{
						Weight: 90,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "rack",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"foo"},
									},
								},
							},
						},
					},
				},
			},
		})
}

func machineReconciliationSelectsUniqueAndAvailablehardwareForEachMachineFilteringByRequiredAndPreferredHardwareAffinity(t *testing.T) {
	machineReconciliationHardwareAffinityHelper(t,
		testOptions{
			HardwareAffinity: &infrastructurev1.HardwareAffinity{
				// least preferred per preferences, but required so we must pick it
				Required: []infrastructurev1.HardwareAffinityTerm{
					{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "foo"},
						},
					},
				},
				Preferred: []infrastructurev1.WeightedHardwareAffinityTerm{
					{
						Weight: 50,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"rack": "bar"},
							},
						},
					},
					{
						Weight: 50,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"rack": "baz"},
							},
						},
					},
					{
						Weight: 0,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "rack",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"foo"},
									},
								},
							},
						},
					},
				},
			},
		}, testOptions{
			HardwareAffinity: &infrastructurev1.HardwareAffinity{
				// these are OR'd so it should select everything, but we select for 'bar' via preferences
				Required: []infrastructurev1.HardwareAffinityTerm{
					{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "non-existent"},
						},
					},
					{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "something-else"},
						},
					},
					{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "foo"},
						},
					},
					{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "bar"},
						},
					},
					{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"rack": "baz"},
						},
					},
				},
				Preferred: []infrastructurev1.WeightedHardwareAffinityTerm{
					{
						Weight: 91,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "rack",
										Operator: metav1.LabelSelectorOpNotIn,
										Values:   []string{"foo", "baz"},
									},
								},
							},
						},
					},
					{
						Weight: 90,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "rack",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"foo"},
									},
								},
							},
						},
					},
				},
			},
		},
		testOptions{
			HardwareAffinity: &infrastructurev1.HardwareAffinity{
				Required: []infrastructurev1.HardwareAffinityTerm{
					{
						LabelSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "non-existent-key",
									Operator: metav1.LabelSelectorOpDoesNotExist,
								},
							},
						},
					},
				},
				Preferred: []infrastructurev1.WeightedHardwareAffinityTerm{
					{
						Weight: 50,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"rack": "baz"},
							},
						},
					},
					{
						Weight: 10,
						HardwareAffinityTerm: infrastructurev1.HardwareAffinityTerm{
							LabelSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{"rack": "foo"},
							},
						},
					},
				},
			},
		})
}

func machineReconciliationHardwareAffinityHelper(t *testing.T, fooOptions testOptions, barOptions testOptions, bazOptions testOptions) {
	t.Helper()
	t.Parallel()
	g := NewWithT(t)

	fooMachineName := "fooMachineName"
	barMachineName := "barMachineName"
	bazMachineName := "bazMachineName"

	firstHardwareUUID := uuid.New().String()
	secondHardwareUUID := uuid.New().String()
	thirdHardwareUUID := uuid.New().String()

	fooTinkerbellMachineName := "machineInRackFoo"
	barTinkerbellMachineName := "machineInRackBar"
	bazTinkerbellMachineName := "machineInRackBaz"
	fooHardwareName := "hwInRackFoo"
	barHardwareName := "hwInRackBar"
	bazHardwareName := "hwInRackBaz"
	objects := []runtime.Object{
		validTinkerbellMachine(bazTinkerbellMachineName, clusterNamespace, bazMachineName, thirdHardwareUUID, bazOptions),
		validTinkerbellMachine(fooTinkerbellMachineName, clusterNamespace, fooMachineName, firstHardwareUUID, fooOptions),
		validTinkerbellMachine(barTinkerbellMachineName, clusterNamespace, barMachineName, secondHardwareUUID, barOptions),

		validCluster(clusterName, clusterNamespace),
		validTinkerbellCluster(clusterName, clusterNamespace),

		validHardware(bazHardwareName, thirdHardwareUUID, "3.3.3.3", testOptions{Labels: map[string]string{"rack": "baz"}}),
		validHardware(barHardwareName, secondHardwareUUID, "2.2.2.2", testOptions{Labels: map[string]string{"rack": "bar"}}),
		validHardware(fooHardwareName, firstHardwareUUID, "1.1.1.1", testOptions{Labels: map[string]string{"rack": "foo"}}),

		validMachine(barMachineName, clusterNamespace, clusterName),
		validMachine(fooMachineName, clusterNamespace, clusterName),
		validMachine(bazMachineName, clusterNamespace, clusterName),

		validSecret(fooMachineName, clusterNamespace),
		validSecret(barMachineName, clusterNamespace),
		validSecret(bazMachineName, clusterNamespace),
	}

	cl := kubernetesClientWithObjects(t, objects)

	_, err := reconcileMachineWithClient(cl, fooTinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	tinkerbellMachineNamespacedName := types.NamespacedName{
		Name:      fooTinkerbellMachineName,
		Namespace: clusterNamespace,
	}

	ctx := context.Background()

	fooMachine := &infrastructurev1.TinkerbellMachine{}
	g.Expect(cl.Get(ctx, tinkerbellMachineNamespacedName, fooMachine)).To(Succeed(), "Getting first updated machine")

	_, err = reconcileMachineWithClient(cl, barTinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	tinkerbellMachineNamespacedName.Name = barTinkerbellMachineName
	barMachine := &infrastructurev1.TinkerbellMachine{}
	g.Expect(cl.Get(ctx, tinkerbellMachineNamespacedName, barMachine)).To(Succeed())

	_, err = reconcileMachineWithClient(cl, bazTinkerbellMachineName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	tinkerbellMachineNamespacedName.Name = bazTinkerbellMachineName
	bazMachine := &infrastructurev1.TinkerbellMachine{}
	g.Expect(cl.Get(ctx, tinkerbellMachineNamespacedName, bazMachine)).To(Succeed())

	g.Expect(fooMachine.Spec.HardwareName).To(Equal(fooHardwareName))
	g.Expect(barMachine.Spec.HardwareName).To(Equal(barHardwareName))
	g.Expect(bazMachine.Spec.HardwareName).To(Equal(bazHardwareName))
}
