package controllers_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/rufio/controllers"
	"github.com/tinkerbell/rufio/controllers/mocks"
)

func TestTaskReconciler_ReconcilePower(t *testing.T) {
	for name, tt := range map[string]struct {
		configureClientCalls func(expect *mocks.MockBMCClientMockRecorder)
		action               v1alpha1.Action
	}{
		"PowerOn": {
			configureClientCalls: func(expect *mocks.MockBMCClientMockRecorder) {
				gomock.InOrder(
					expect.SetPowerState(gomock.Any(), string(v1alpha1.PowerOn)).Return(true, nil),
					expect.Close(gomock.Any()).Return(nil),
					expect.GetPowerState(gomock.Any()).Return("on", nil),
					expect.Close(gomock.Any()).Return(nil),
				)
			},
			action: v1alpha1.Action{PowerAction: v1alpha1.PowerOn.Pointer()},
		},
		"HardOff": {
			configureClientCalls: func(expect *mocks.MockBMCClientMockRecorder) {
				gomock.InOrder(
					expect.SetPowerState(gomock.Any(), string(v1alpha1.HardPowerOff)).Return(true, nil),
					expect.Close(gomock.Any()).Return(nil),
					expect.GetPowerState(gomock.Any()).Return("off", nil),
					expect.Close(gomock.Any()).Return(nil),
				)
			},
			action: v1alpha1.Action{PowerAction: v1alpha1.HardPowerOff.Pointer()},
		},
		"SoftOff": {
			configureClientCalls: func(expect *mocks.MockBMCClientMockRecorder) {
				gomock.InOrder(
					expect.SetPowerState(gomock.Any(), string(v1alpha1.SoftPowerOff)).Return(true, nil),
					expect.Close(gomock.Any()).Return(nil),
					expect.GetPowerState(gomock.Any()).Return("off", nil),
					expect.Close(gomock.Any()).Return(nil),
				)
			},
			action: v1alpha1.Action{PowerAction: v1alpha1.SoftPowerOff.Pointer()},
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			secret := createSecret()
			task := createTask(name, tt.action, secret)
			cluster := createKubeClientWithObjects(task, secret)

			bmc := mocks.NewMockBMCClient(gomock.NewController(t))
			tt.configureClientCalls(bmc.EXPECT())

			reconciler := controllers.NewTaskReconciler(cluster, mockBMCClientFactoryFunc(bmc))

			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: task.Namespace,
					Name:      task.Name,
				},
			}

			result, err := reconciler.Reconcile(context.Background(), request)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(result).To(gomega.Equal(ctrl.Result{}))

			var retrieved v1alpha1.Task
			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(retrieved.Status.StartTime.Unix()).To(gomega.BeNumerically("~", time.Now().Unix(), 2))
			g.Expect(retrieved.Status.CompletionTime.IsZero()).To(gomega.BeTrue())
			g.Expect(retrieved.Status.Conditions).To(gomega.HaveLen(0))

			// Ensure re-reconciling a task does sends it into a success state.
			result, err = reconciler.Reconcile(context.Background(), request)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(result).To(gomega.Equal(reconcile.Result{}))

			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(retrieved.Status.CompletionTime.Unix()).To(gomega.BeNumerically("~", time.Now().Unix(), 2))
			g.Expect(retrieved.Status.Conditions).To(gomega.HaveLen(1))
			g.Expect(retrieved.Status.Conditions[0].Type).To(gomega.Equal(v1alpha1.TaskCompleted))
			g.Expect(retrieved.Status.Conditions[0].Status).To(gomega.Equal(v1alpha1.ConditionTrue))

			var retrieved2 v1alpha1.Task
			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved2)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(retrieved2).To(gomega.BeEquivalentTo(retrieved))
		})
	}
}

func TestTaskReconciler_ReconcileBootDevice(t *testing.T) {
	for name, tt := range map[string]struct {
		configureClientCalls func(expect *mocks.MockBMCClientMockRecorder)
		action               v1alpha1.Action
	}{
		"Set": {
			configureClientCalls: func(expect *mocks.MockBMCClientMockRecorder) {
				gomock.InOrder(
					expect.
						SetBootDevice(gomock.Any(), string(v1alpha1.PXE), false, gomock.Any()).
						Return(true, nil),
					expect.Close(gomock.Any()).Return(nil),
					expect.Close(gomock.Any()).Return(nil),
				)
			},
			action: v1alpha1.Action{
				OneTimeBootDeviceAction: &v1alpha1.OneTimeBootDeviceAction{
					Devices: []v1alpha1.BootDevice{v1alpha1.PXE},
				},
			},
		},
		"MultipleDevices": {
			configureClientCalls: func(expect *mocks.MockBMCClientMockRecorder) {
				gomock.InOrder(
					expect.
						SetBootDevice(gomock.Any(), string(v1alpha1.BIOS), false, gomock.Any()).
						Return(true, nil),
					expect.Close(gomock.Any()).Return(nil),
					expect.Close(gomock.Any()).Return(nil),
				)
			},
			action: v1alpha1.Action{
				OneTimeBootDeviceAction: &v1alpha1.OneTimeBootDeviceAction{
					Devices: []v1alpha1.BootDevice{v1alpha1.BIOS, v1alpha1.PXE},
				},
			},
		},
		"EFIBoot": {
			configureClientCalls: func(expect *mocks.MockBMCClientMockRecorder) {
				gomock.InOrder(
					expect.SetBootDevice(gomock.Any(), string(v1alpha1.PXE), false, true).Return(true, nil),
					expect.Close(gomock.Any()).Return(nil),
					expect.Close(gomock.Any()).Return(nil),
				)
			},
			action: v1alpha1.Action{
				OneTimeBootDeviceAction: &v1alpha1.OneTimeBootDeviceAction{
					Devices: []v1alpha1.BootDevice{v1alpha1.PXE},
					EFIBoot: true,
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			secret := createSecret()
			task := createTask(name, tt.action, secret)
			cluster := createKubeClientWithObjects(task, secret)

			bmc := mocks.NewMockBMCClient(gomock.NewController(t))
			tt.configureClientCalls(bmc.EXPECT())

			reconciler := controllers.NewTaskReconciler(cluster, mockBMCClientFactoryFunc(bmc))

			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: task.Namespace,
					Name:      task.Name,
				},
			}

			result, err := reconciler.Reconcile(context.Background(), request)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(result).To(gomega.Equal(ctrl.Result{}))

			var retrieved v1alpha1.Task
			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(retrieved.Status.StartTime.Unix()).To(gomega.BeNumerically("~", time.Now().Unix(), 2))
			g.Expect(retrieved.Status.CompletionTime.IsZero()).To(gomega.BeTrue())
			g.Expect(retrieved.Status.Conditions).To(gomega.HaveLen(0))

			// Ensure re-reconciling a task does sends it into a success state.
			result, err = reconciler.Reconcile(context.Background(), request)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(result).To(gomega.Equal(reconcile.Result{}))

			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(retrieved.Status.CompletionTime.Unix()).To(gomega.BeNumerically("~", time.Now().Unix(), 2))
			g.Expect(retrieved.Status.Conditions).To(gomega.HaveLen(1))
			g.Expect(retrieved.Status.Conditions[0].Type).To(gomega.Equal(v1alpha1.TaskCompleted))
			g.Expect(retrieved.Status.Conditions[0].Status).To(gomega.Equal(v1alpha1.ConditionTrue))

			var retrieved2 v1alpha1.Task
			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved2)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(retrieved2).To(gomega.BeEquivalentTo(retrieved))
		})
	}
}

func TestTaskReconciler_ReconcileWithBMCClientError(t *testing.T) {
	g := gomega.NewWithT(t)
	secret := createSecret()
	task := createTask("task", v1alpha1.Action{}, secret)
	client := createKubeClientWithObjects(task, secret)

	reconciler := controllers.NewTaskReconciler(client, erroringBMCClientFactoryFunc())

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: task.Namespace,
			Name:      task.Name,
		},
	}

	_, err := reconciler.Reconcile(context.Background(), request)
	g.Expect(err).To(gomega.HaveOccurred())
}

func TestTaskReconciler_BMCClientErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		configureClientCalls func(expect *mocks.MockBMCClientMockRecorder)
		action               v1alpha1.Action
	}{
		"PowerOn": {
			configureClientCalls: func(expect *mocks.MockBMCClientMockRecorder) {
				gomock.InOrder(
					expect.
						SetPowerState(gomock.Any(), gomock.Any()).
						Return(false, errors.New("power on error")),
					expect.Close(gomock.Any()).Return(nil),
				)
			},
			action: v1alpha1.Action{PowerAction: v1alpha1.PowerOn.Pointer()},
		},
		"BootDevice": {
			configureClientCalls: func(expect *mocks.MockBMCClientMockRecorder) {
				gomock.InOrder(
					expect.
						SetBootDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
						Return(false, errors.New("boot device error")),
					expect.Close(gomock.Any()).Return(nil),
				)
			},
			action: v1alpha1.Action{OneTimeBootDeviceAction: &v1alpha1.OneTimeBootDeviceAction{
				Devices: []v1alpha1.BootDevice{v1alpha1.PXE},
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			secret := createSecret()
			task := createTask(name, tt.action, secret)
			cluster := createKubeClientWithObjects(task, secret)

			bmc := mocks.NewMockBMCClient(gomock.NewController(t))
			tt.configureClientCalls(bmc.EXPECT())

			reconciler := controllers.NewTaskReconciler(cluster, mockBMCClientFactoryFunc(bmc))

			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: task.Namespace,
					Name:      task.Name,
				},
			}

			result, err := reconciler.Reconcile(context.Background(), request)
			g.Expect(err).To(gomega.HaveOccurred())
			g.Expect(result).To(gomega.Equal(ctrl.Result{}))

			var retrieved v1alpha1.Task
			err = cluster.Get(context.Background(), request.NamespacedName, &retrieved)
			g.Expect(err).To(gomega.Succeed())
			g.Expect(retrieved.Status.StartTime.Unix()).To(gomega.BeNumerically("~", time.Now().Unix(), 2))
			g.Expect(retrieved.Status.CompletionTime.IsZero()).To(gomega.BeTrue())
			g.Expect(retrieved.Status.Conditions).To(gomega.HaveLen(1))
			g.Expect(retrieved.Status.Conditions[0].Type).To(gomega.Equal(v1alpha1.TaskFailed))
			g.Expect(retrieved.Status.Conditions[0].Status).To(gomega.Equal(v1alpha1.ConditionTrue))
		})
	}
}

func TestTaskReconciler_Timeout(t *testing.T) {
	g := gomega.NewWithT(t)
	logger := mustCreateLogr(t.Name())
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, logger)

	secret := createSecret()
	task := createTask("task", v1alpha1.Action{PowerAction: v1alpha1.PowerOn.Pointer()}, secret)
	cluster := createKubeClientWithObjects(task, secret)

	bmc := mocks.NewMockBMCClient(gomock.NewController(t))
	gomock.InOrder(
		bmc.EXPECT().SetPowerState(gomock.Any(), string(v1alpha1.PowerOn)).Return(true, nil),
		bmc.EXPECT().Close(gomock.Any()).Return(nil),
		bmc.EXPECT().Close(gomock.Any()).Return(nil),
	)

	reconciler := controllers.NewTaskReconciler(cluster, mockBMCClientFactoryFunc(bmc))

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: task.Namespace,
			Name:      task.Name,
		},
	}

	result, err := reconciler.Reconcile(ctx, request)
	g.Expect(err).To(gomega.Succeed())
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))

	// Retrieve the task and update the start time to be within the timeout period
	var retrieved v1alpha1.Task
	err = cluster.Get(ctx, request.NamespacedName, &retrieved)
	g.Expect(err).To(gomega.Succeed())

	expired := metav1.NewTime(retrieved.Status.StartTime.Add(-time.Hour))
	retrieved.Status.StartTime = &expired
	err = cluster.Update(ctx, &retrieved)
	g.Expect(err).To(gomega.Succeed())

	result, err = reconciler.Reconcile(ctx, request)
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(result).To(gomega.Equal(ctrl.Result{}))
}

func createTask(name string, action v1alpha1.Action, secret *corev1.Secret) *v1alpha1.Task {
	return &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1alpha1.TaskSpec{
			Task: action,
			Connection: v1alpha1.Connection{
				Host: "host",
				Port: 22,
				AuthSecretRef: corev1.SecretReference{
					Name:      secret.Name,
					Namespace: secret.Namespace,
				},
			},
		},
	}
}

func mockBMCClientFactoryFunc(m *mocks.MockBMCClient) controllers.BMCClientFactoryFunc {
	return func(_ context.Context, hostIP, port, username, password string) (controllers.BMCClient, error) {
		return m, nil
	}
}

func erroringBMCClientFactoryFunc() controllers.BMCClientFactoryFunc {
	return func(_ context.Context, hostIP, port, username, password string) (controllers.BMCClient, error) {
		return nil, errors.New("error")
	}
}
