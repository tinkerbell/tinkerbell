package controller_test

import (
	"context"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/bmclib/v2/providers"
	"github.com/go-logr/logr"
	"github.com/jacobweinstock/registrar"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// This source file is currently a bucket of stuff. If it grows too big, consider breaking it
// into more granular helper sources.

// newClientBuilder creates a fake kube client builder loaded with Rufio's and Kubernetes'
// corev1 schemes. It uses a basic ObjectTracker to avoid managed fields issues with
// controller-runtime v0.22+.
func newClientBuilder() *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	if err := api.AddToSchemeBMC(scheme); err != nil {
		panic(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	ensureTypeMeta := func(obj client.Object) {
		if obj != nil {
			gvks, _, _ := scheme.ObjectKinds(obj)
			if len(gvks) > 0 {
				obj.GetObjectKind().SetGroupVersionKind(gvks[0])
			}
		}
	}

	// Use a basic ObjectTracker that does NOT do managed fields tracking.
	// controller-runtime v0.22+ uses FieldManagedObjectTracker by default which causes issues
	// with MergeFrom patches when TypeMeta is not properly set.
	// Note: WithObjectTracker is incompatible with WithStatusSubresource, so tests using
	// newClientBuilder() must NOT chain WithStatusSubresource. Tests that need WithStatusSubresource
	// (like those in tink/controller/internal/workflow) should create their own fake client builder.
	codecs := serializer.NewCodecFactory(scheme)
	tracker := k8stesting.NewObjectTracker(scheme, codecs.UniversalDecoder())

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjectTracker(tracker).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				err := c.Get(ctx, key, obj, opts...)
				ensureTypeMeta(obj)
				return err
			},
			SubResourceGet: func(ctx context.Context, c client.Client, subResource string, obj client.Object, subResourceObj client.Object, opts ...client.SubResourceGetOption) error {
				err := c.SubResource(subResource).Get(ctx, obj, subResourceObj, opts...)
				ensureTypeMeta(obj)
				ensureTypeMeta(subResourceObj)
				return err
			},
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				ensureTypeMeta(obj)
				return c.Update(ctx, obj, opts...)
			},
			// SubResourcePatch and SubResourceUpdate use direct Update since we're using a basic
			// ObjectTracker without status subresource support. As a result, the patch parameter
			// provided to SubResourcePatch is intentionally ignored and a full update is performed
			// instead; tests relying on real patch semantics should not use this helper.
			SubResourcePatch: func(ctx context.Context, c client.Client, _ string, obj client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
				ensureTypeMeta(obj)
				return c.Update(ctx, obj)
			},
			SubResourceUpdate: func(ctx context.Context, c client.Client, _ string, obj client.Object, _ ...client.SubResourceUpdateOption) error {
				ensureTypeMeta(obj)
				return c.Update(ctx, obj)
			},
		})
}

type testProvider struct {
	PName                 string
	Proto                 string
	Powerstate            string
	PowerSetOK            bool
	BootdeviceOK          bool
	VirtualMediaOK        bool
	ErrOpen               error
	ErrClose              error
	ErrPowerStateGet      error
	ErrPowerStateSet      error
	ErrBootDeviceSet      error
	ErrVirtualMediaInsert error
}

func (t *testProvider) Name() string {
	if t.PName != "" {
		return t.PName
	}
	return "tester"
}

func (t *testProvider) Protocol() string {
	if t.Proto != "" {
		return t.Proto
	}
	return "redfish"
}

func (t *testProvider) Features() registrar.Features {
	return registrar.Features{
		providers.FeaturePowerState,
		providers.FeaturePowerSet,
		providers.FeatureBootDeviceSet,
		providers.FeatureVirtualMedia,
	}
}

func (t *testProvider) Open(_ context.Context) error {
	return t.ErrOpen
}

func (t *testProvider) Close(_ context.Context) error {
	return t.ErrClose
}

func (t *testProvider) PowerStateGet(_ context.Context) (string, error) {
	return t.Powerstate, t.ErrPowerStateGet
}

func (t *testProvider) PowerSet(_ context.Context, _ string) (ok bool, err error) {
	return t.PowerSetOK, t.ErrPowerStateSet
}

func (t *testProvider) BootDeviceSet(_ context.Context, _ string, _, _ bool) (ok bool, err error) {
	return t.BootdeviceOK, t.ErrBootDeviceSet
}

func (t *testProvider) SetVirtualMedia(_ context.Context, _ string, _ string) (ok bool, err error) {
	return t.VirtualMediaOK, t.ErrVirtualMediaInsert
}

// newMockBMCClientFactoryFunc returns a new BMCClientFactoryFunc.
func newTestClient(provider *testProvider) controller.ClientFunc {
	return func(ctx context.Context, log logr.Logger, hostIP, username, password string, opts *controller.BMCOptions) (*bmclib.Client, error) {
		o := opts.Translate(hostIP)
		reg := registrar.NewRegistry(registrar.WithLogger(log))
		reg.Register(provider.Name(), provider.Protocol(), provider.Features(), nil, provider)
		o = append(o, bmclib.WithLogger(log), bmclib.WithRegistry(reg))
		cl := bmclib.NewClient(hostIP, username, password, o...)
		return cl, cl.Open(ctx)
	}
}
