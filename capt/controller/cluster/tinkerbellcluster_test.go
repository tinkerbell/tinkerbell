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

package cluster_test

import (
	"context"
	"testing"

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

	tinkv1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"

	"github.com/tinkerbell/tinkerbell/capt/controller/cluster"
	infrastructurev1 "github.com/tinkerbell/tinkerbell/pkg/api/v1beta1/capt"
)

//nolint:unparam
func unreadyTinkerbellCluster(name, namespace string) *infrastructurev1.TinkerbellCluster {
	unreadyTinkerbellCluster := validTinkerbellCluster(name, namespace)
	unreadyTinkerbellCluster.Status.Ready = false
	unreadyTinkerbellCluster.ObjectMeta.Finalizers = nil
	unreadyTinkerbellCluster.Spec.ControlPlaneEndpoint.Host = ""
	unreadyTinkerbellCluster.Spec.ControlPlaneEndpoint.Port = 0

	return unreadyTinkerbellCluster
}

func Test_Cluster_reconciliation_when_controlplane_endpoint_not_set(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	objects := []runtime.Object{
		validHardware(hardwareName, uuid.New().String(), hardwareIP),
		validCluster(clusterName, clusterNamespace),
		unreadyTinkerbellCluster(clusterName, clusterNamespace),
	}

	client := kubernetesClientWithObjects(t, objects)

	_, err := reconcileClusterWithClient(client, clusterName, clusterNamespace)
	g.Expect(err).To(MatchError(cluster.ErrControlPlaneEndpointNotSet))
}

func Test_Cluster_reconciliation_when_controlplane_endpoint_set_on_cluster(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	cluster := validCluster(clusterName, clusterNamespace)
	cluster.Spec.ControlPlaneEndpoint.Host = "192.168.1.10"
	cluster.Spec.ControlPlaneEndpoint.Port = 443

	objects := []runtime.Object{
		validHardware(hardwareName, uuid.New().String(), hardwareIP),
		cluster.DeepCopy(),
		unreadyTinkerbellCluster(clusterName, clusterNamespace),
	}

	client := kubernetesClientWithObjects(t, objects)

	_, err := reconcileClusterWithClient(client, clusterName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	namespacedName := types.NamespacedName{
		Name:      clusterName,
		Namespace: clusterNamespace,
	}

	updatedTinkerbellCluster := &infrastructurev1.TinkerbellCluster{}

	g.Expect(client.Get(context.Background(), namespacedName, updatedTinkerbellCluster)).To(Succeed())

	g.Expect(updatedTinkerbellCluster.Spec.ControlPlaneEndpoint.Host).
		To(BeEquivalentTo(cluster.Spec.ControlPlaneEndpoint.Host), "Expected controlplane endpoint host to be set")

	g.Expect(updatedTinkerbellCluster.Spec.ControlPlaneEndpoint.Port).
		To(BeEquivalentTo(cluster.Spec.ControlPlaneEndpoint.Port), "Expected controlplane endpoint port to be set")

	g.Expect(updatedTinkerbellCluster.Status.Ready).To(BeTrue(), "Expected infrastructure to be ready")
}

type testOptions struct {
	// Labels allow providing labels for the machine
	Labels           map[string]string
	HardwareAffinity *infrastructurev1.HardwareAffinity
}

func validHardware(name, uuid, ip string, options ...testOptions) *tinkv1.Hardware {
	hw := &tinkv1.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: clusterNamespace,
			UID:       types.UID(uuid),
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

func Test_Cluster_reconciliation_when_controlplane_endpoint_set_on_tinkerbellCluster(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	tinkCluster := unreadyTinkerbellCluster(clusterName, clusterNamespace)
	tinkCluster.Spec.ControlPlaneEndpoint.Host = "192.168.1.10"
	tinkCluster.Spec.ControlPlaneEndpoint.Port = 443

	objects := []runtime.Object{
		validHardware(hardwareName, uuid.New().String(), hardwareIP),
		validCluster(clusterName, clusterNamespace),
		tinkCluster.DeepCopy(),
	}

	client := kubernetesClientWithObjects(t, objects)

	_, err := reconcileClusterWithClient(client, clusterName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred())

	namespacedName := types.NamespacedName{
		Name:      clusterName,
		Namespace: clusterNamespace,
	}

	updatedTinkerbellCluster := &infrastructurev1.TinkerbellCluster{}

	g.Expect(client.Get(context.Background(), namespacedName, updatedTinkerbellCluster)).To(Succeed())

	g.Expect(updatedTinkerbellCluster.Spec.ControlPlaneEndpoint.Host).
		To(BeEquivalentTo(tinkCluster.Spec.ControlPlaneEndpoint.Host), "Expected controlplane endpoint host to be set")

	g.Expect(updatedTinkerbellCluster.Spec.ControlPlaneEndpoint.Port).
		To(BeEquivalentTo(tinkCluster.Spec.ControlPlaneEndpoint.Port), "Expected controlplane endpoint port to be set")

	g.Expect(updatedTinkerbellCluster.Status.Ready).To(BeTrue(), "Expected infrastructure to be ready")
}

func Test_Cluster_reconciliation(t *testing.T) {
	t.Parallel()

	t.Run("is_not_requeued_when", func(t *testing.T) {
		t.Parallel()

		// This is introduced in v1alpha3 of CAPI even though behavior diagram does not reflect it.
		// This will be automatically requeued when the tinkerbellCluster is unpaused.
		t.Run("tinkerbellcluster_is_paused", clusterReconciliationIsNotRequeuedWhenTinkerbellClusterIsPaused) //nolint:paralleltest

		// This is introduced in v1alpha3 of CAPI even though behavior diagram does not reflect it.
		// Requeue happens through watch of Cluster.
		t.Run("cluster_is_paused", clusterReconciliationIsNotRequeuedWhenClusterIsPaused) //nolint:paralleltest

		// From https://cluster-api.sigs.k8s.io/developer/providers/cluster-infrastructure.html#behavior.
		// This will be automatically requeued when the ownerRef is set.
		t.Run("cluster_has_no_owner_set", clusterReconciliationIsNotRequeuedWhenClusterHasNoOwnerSet) //nolint:paralleltest

		// If reconciliation process started, but we cannot find cluster object anymore, it means object has been
		// removed in the meanwhile. This means there is nothing to do.
		t.Run("cluster_object_is_missing", clusterReconciliationIsNotRequeuedWhenClusterObjectIsMissing) //nolint:paralleltest
	})

	t.Run("fails_when", func(t *testing.T) {
		t.Parallel()

		t.Run("reconciler_has_no_client_set", clusterReconciliationFailsWhenReconcilerHasNoClientSet) //nolint:paralleltest
	})
}

func clusterReconciliationFailsWhenReconcilerHasNoClientSet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterController := &cluster.TinkerbellClusterReconciler{}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: clusterNamespace,
			Name:      clusterName,
		},
	}

	_, err := clusterController.Reconcile(context.TODO(), request)
	g.Expect(err).To(MatchError(cluster.ErrMissingClient))
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

//nolint:unparam
func reconcileClusterWithClient(client client.Client, name, namespace string) (ctrl.Result, error) {
	clusterController := &cluster.TinkerbellClusterReconciler{
		Client: client,
	}

	request := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	return clusterController.Reconcile(context.TODO(), request) //nolint:wrapcheck
}

func clusterReconciliationIsNotRequeuedWhenClusterObjectIsMissing(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	result, err := reconcileClusterWithClient(kubernetesClientWithObjects(t, nil), clusterName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred(), "Reconciling when cluster object does not exist should not return error")
	g.Expect(result.IsZero()).To(BeTrue(), "Expected result to not request requeue")
}

const (
	clusterName      = "myClusterName"
	clusterNamespace = "myClusterNamespace"
	hardwareIP       = "1.1.1.1"
	hardwareName     = "myHardwareName"
)

//nolint:unparam
func validCluster(name, namespace string) *clusterv1.Cluster {
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

func validTinkerbellCluster(name, namespace string) *infrastructurev1.TinkerbellCluster {
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

func clusterReconciliationIsNotRequeuedWhenClusterHasNoOwnerSet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	unreadyTinkerbellClusterWithoutOwner := unreadyTinkerbellCluster(clusterName, clusterNamespace)
	unreadyTinkerbellClusterWithoutOwner.ObjectMeta.OwnerReferences = nil

	objects := []runtime.Object{
		validCluster(clusterName, clusterNamespace),
		unreadyTinkerbellClusterWithoutOwner,
	}

	result, err := reconcileClusterWithClient(kubernetesClientWithObjects(t, objects), clusterName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred(), "Reconciling new cluster object should not fail when cluster has no owner set yet")

	g.Expect(result.IsZero()).To(BeTrue(), "Expected result to not request requeue")
}

func clusterReconciliationIsNotRequeuedWhenTinkerbellClusterIsPaused(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	pausedTinkerbellCluster := validTinkerbellCluster(clusterName, clusterNamespace)
	pausedTinkerbellCluster.ObjectMeta.Annotations = map[string]string{
		clusterv1.PausedAnnotation: "true",
	}

	objects := []runtime.Object{
		validCluster(clusterName, clusterNamespace),
		pausedTinkerbellCluster,
	}

	result, err := reconcileClusterWithClient(kubernetesClientWithObjects(t, objects), clusterName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred(), "Reconciling new cluster object should not fail when tinkerbellCluster is paused")

	g.Expect(result.IsZero()).To(BeTrue(), "Expected result to not request requeue")
}

func clusterReconciliationIsNotRequeuedWhenClusterIsPaused(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	pausedCluster := validCluster(clusterName, clusterNamespace)
	pausedCluster.Spec.Paused = true

	objects := []runtime.Object{
		pausedCluster,
		validTinkerbellCluster(clusterName, clusterNamespace),
	}

	result, err := reconcileClusterWithClient(kubernetesClientWithObjects(t, objects), clusterName, clusterNamespace)
	g.Expect(err).NotTo(HaveOccurred(), "Reconciling new cluster object should not fail when tinkerbellCluster is paused")

	g.Expect(result.IsZero()).To(BeTrue(), "Expected result to not request requeue")
}
