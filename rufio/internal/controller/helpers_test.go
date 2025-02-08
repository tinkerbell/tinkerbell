package controller_test

import (
	"context"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/bmclib/v2/providers"
	"github.com/go-logr/logr"
	"github.com/jacobweinstock/registrar"
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/bmc/v1alpha1"
	"github.com/tinkerbell/tinkerbell/rufio/internal/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// This source file is currently a bucket of stuff. If it grows too big, consider breaking it
// into more granular helper sources.

// newClientBuilder creates a fake kube client builder loaded with Rufio's and Kubernetes'
// corev1 schemes.
func newClientBuilder() *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	return fake.NewClientBuilder().
		WithScheme(scheme)
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
