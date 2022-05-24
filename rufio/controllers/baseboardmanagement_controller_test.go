package controllers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bmcv1alpha1 "github.com/tinkerbell/rufio/api/v1alpha1"
	"github.com/tinkerbell/rufio/controllers"
	"github.com/tinkerbell/rufio/controllers/mocks"
)

func TestReconcileSetPowerSuccess(t *testing.T) {
	g := gomega.NewWithT(t)

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockBMCClient := mocks.NewMockBMCClient(ctrl)

	bm := getBaseboardManagement()
	authSecret := getSecret()

	objs := []runtime.Object{bm, authSecret}
	scheme := runtime.NewScheme()
	_ = bmcv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	clientBuilder := fake.NewClientBuilder()
	client := clientBuilder.WithScheme(scheme).WithRuntimeObjects(objs...).Build()
	fakeRecorder := record.NewFakeRecorder(2)

	mockBMCClient.EXPECT().GetPowerState(ctx).Return(string(bmcv1alpha1.Off), nil)
	mockBMCClient.EXPECT().SetPowerState(ctx, string(bmcv1alpha1.On)).Return(true, nil)
	mockBMCClient.EXPECT().Close(ctx).Return(nil)

	reconciler := controllers.NewBaseboardManagementReconciler(
		client,
		fakeRecorder,
		newMockBMCClientFactoryFunc(mockBMCClient),
		testr.New(t),
	)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "test-namespace",
			Name:      "test-bm",
		},
	}

	_, err := reconciler.Reconcile(ctx, req)
	g.Expect(err).ToNot(gomega.HaveOccurred())
}

func TestReconcileDesiredPowerStateSuccess(t *testing.T) {
	g := gomega.NewWithT(t)

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockBMCClient := mocks.NewMockBMCClient(ctrl)

	bm := getBaseboardManagement()
	authSecret := getSecret()

	objs := []runtime.Object{bm, authSecret}
	scheme := runtime.NewScheme()
	_ = bmcv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	clientBuilder := fake.NewClientBuilder()
	client := clientBuilder.WithScheme(scheme).WithRuntimeObjects(objs...).Build()
	fakeRecorder := record.NewFakeRecorder(2)

	mockBMCClient.EXPECT().GetPowerState(ctx).Return(string(bmcv1alpha1.On), nil)
	mockBMCClient.EXPECT().Close(ctx).Return(nil)

	reconciler := controllers.NewBaseboardManagementReconciler(
		client,
		fakeRecorder,
		newMockBMCClientFactoryFunc(mockBMCClient),
		testr.New(t),
	)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "test-namespace",
			Name:      "test-bm",
		},
	}

	_, err := reconciler.Reconcile(ctx, req)
	g.Expect(err).ToNot(gomega.HaveOccurred())
}

func TestReconcileSecretReferenceError(t *testing.T) {
	g := gomega.NewWithT(t)

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockBMCClient := mocks.NewMockBMCClient(ctrl)

	bm := getBaseboardManagement()

	scheme := runtime.NewScheme()
	_ = bmcv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	fakeRecorder := record.NewFakeRecorder(2)

	tt := map[string]struct {
		Secret *corev1.Secret
	}{
		"secret not found": {
			Secret: &corev1.Secret{},
		},
		"username not found": {
			Secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-bm-auth",
				},
				Data: map[string][]byte{
					"password": []byte("test"),
				},
			},
		},
		"password not found": {
			Secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
					Name:      "test-bm-auth",
				},
				Data: map[string][]byte{
					"username": []byte("test"),
				},
			},
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			objs := []runtime.Object{bm, test.Secret}
			clientBuilder := fake.NewClientBuilder()
			client := clientBuilder.WithScheme(scheme).WithRuntimeObjects(objs...).Build()
			reconciler := controllers.NewBaseboardManagementReconciler(
				client,
				fakeRecorder,
				newMockBMCClientFactoryFunc(mockBMCClient),
				testr.New(t),
			)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "test-namespace",
					Name:      "test-bm",
				},
			}

			_, err := reconciler.Reconcile(ctx, req)
			g.Expect(err).To(gomega.HaveOccurred())
		})

	}

}

func TestReconcileConnectionError(t *testing.T) {
	g := gomega.NewWithT(t)

	ctx := context.Background()

	bm := getBaseboardManagement()
	authSecret := getSecret()

	objs := []runtime.Object{bm, authSecret}
	scheme := runtime.NewScheme()
	_ = bmcv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	clientBuilder := fake.NewClientBuilder()
	client := clientBuilder.WithScheme(scheme).WithRuntimeObjects(objs...).Build()
	fakeRecorder := record.NewFakeRecorder(2)

	reconciler := controllers.NewBaseboardManagementReconciler(
		client,
		fakeRecorder,
		newMockBMCClientFactoryFuncError(),
		testr.New(t),
	)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "test-namespace",
			Name:      "test-bm",
		},
	}

	_, err := reconciler.Reconcile(ctx, req)
	g.Expect(err).To(gomega.HaveOccurred())
}

func TestReconcileSetPowerError(t *testing.T) {
	g := gomega.NewWithT(t)

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockBMCClient := mocks.NewMockBMCClient(ctrl)

	bm := getBaseboardManagement()
	authSecret := getSecret()

	objs := []runtime.Object{bm, authSecret}
	scheme := runtime.NewScheme()
	_ = bmcv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	clientBuilder := fake.NewClientBuilder()
	client := clientBuilder.WithScheme(scheme).WithRuntimeObjects(objs...).Build()
	fakeRecorder := record.NewFakeRecorder(2)

	mockBMCClient.EXPECT().GetPowerState(ctx).Return(string(bmcv1alpha1.Off), nil)
	mockBMCClient.EXPECT().SetPowerState(ctx, string(bmcv1alpha1.On)).Return(false, errors.New("this is not allowed"))
	mockBMCClient.EXPECT().Close(ctx).Return(nil)

	reconciler := controllers.NewBaseboardManagementReconciler(
		client,
		fakeRecorder,
		newMockBMCClientFactoryFunc(mockBMCClient),
		testr.New(t),
	)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "test-namespace",
			Name:      "test-bm",
		},
	}

	_, err := reconciler.Reconcile(ctx, req)
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(fakeRecorder.Events).NotTo(gomega.BeEmpty())
}

// newMockBMCClientFactoryFunc returns a new BMCClientFactoryFunc
func newMockBMCClientFactoryFunc(mockBMCClient *mocks.MockBMCClient) controllers.BMCClientFactoryFunc {
	return func(ctx context.Context, hostIP, port, username, password string) (controllers.BMCClient, error) {
		return mockBMCClient, nil
	}
}

// newMockBMCClientFactoryFunc returns a new BMCClientFactoryFunc
func newMockBMCClientFactoryFuncError() controllers.BMCClientFactoryFunc {
	return func(ctx context.Context, hostIP, port, username, password string) (controllers.BMCClient, error) {
		return nil, errors.New("connection failed")
	}
}

func getBaseboardManagement() *bmcv1alpha1.BaseboardManagement {
	return &bmcv1alpha1.BaseboardManagement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bm",
			Namespace: "test-namespace",
		},
		Spec: bmcv1alpha1.BaseboardManagementSpec{
			Connection: bmcv1alpha1.Connection{
				Host: "0.0.0.0",
				Port: 623,
				AuthSecretRef: corev1.SecretReference{
					Name:      "test-bm-auth",
					Namespace: "test-namespace",
				},
				InsecureTLS: false,
			},
			Power: bmcv1alpha1.On,
		},
	}
}

func getSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-bm-auth",
		},
		Data: map[string][]byte{
			"username": []byte("test"),
			"password": []byte("test"),
		},
	}
}
