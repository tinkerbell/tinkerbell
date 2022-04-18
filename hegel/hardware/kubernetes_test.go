package hardware_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tinkerbell/hegel/hardware"
	tinkv1alpha1 "github.com/tinkerbell/tink/pkg/apis/core/v1alpha1"
	tink "github.com/tinkerbell/tink/pkg/controllers"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestKubernetesClientLists(t *testing.T) {
	const ip = "10.0.10.0"
	const name = "hello-world"

	listerClient := &ListerClientMock{}
	listerClient.
		On("List", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			require.IsType(t, &tinkv1alpha1.HardwareList{}, args.Get(1))
			hw := args.Get(1).(*tinkv1alpha1.HardwareList)

			hw.Items = append(hw.Items, tinkv1alpha1.Hardware{
				ObjectMeta: v1.ObjectMeta{Name: name},
				Spec: tinkv1alpha1.HardwareSpec{
					Metadata: &tinkv1alpha1.HardwareMetadata{
						Facility: &tinkv1alpha1.MetadataFacility{},
						Instance: &tinkv1alpha1.MetadataInstance{
							OperatingSystem: &tinkv1alpha1.MetadataInstanceOperatingSystem{},
							Ips:             []*tinkv1alpha1.MetadataInstanceIP{},
						},
					},
				},
			})
		}).
		Return((error)(nil))

	client := hardware.NewKubernetesClientWithClient(listerClient)

	_, err := client.ByIP(context.Background(), ip)
	require.NoError(t, err)

	require.Equal(t, len(listerClient.Calls), 1)
	args := listerClient.Calls[0].Arguments

	_, ok := args.Get(1).(*tinkv1alpha1.HardwareList)
	assert.True(t, ok)

	opts := args.Get(2).([]crclient.ListOption)
	require.Len(t, opts, 1)

	matchingFields, ok := opts[0].(crclient.MatchingFields)
	require.True(t, ok)

	require.Contains(t, matchingFields, tink.HardwareIPAddrIndex)
	assert.Equal(t, ip, matchingFields[tink.HardwareIPAddrIndex])

	// todo(chrisdoherty4) Validate the returned hardware Export() has correctly serialized data.
}

func TestKubernetesClientListsWithError(t *testing.T) {
	expect := errors.New("foo-bar")
	listerClient := &ListerClientMock{}
	listerClient.
		On("List", mock.Anything, mock.Anything, mock.Anything).
		Return(expect)

	client := hardware.NewKubernetesClientWithClient(listerClient)

	_, err := client.ByIP(context.Background(), "10.10.10.10")
	require.Error(t, err)
	assert.Contains(t, err.Error(), expect.Error())
}

func TestKubernetesClientListWithGt1Result(t *testing.T) {
	listerClient := &ListerClientMock{}
	listerClient.
		On("List", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			require.IsType(t, &tinkv1alpha1.HardwareList{}, args.Get(1))
			hw := args.Get(1).(*tinkv1alpha1.HardwareList)
			hw.Items = make([]tinkv1alpha1.Hardware, 2)
		}).
		Return((error)(nil))

	client := hardware.NewKubernetesClientWithClient(listerClient)

	_, err := client.ByIP(context.Background(), "10.10.10.10")
	assert.Error(t, err)
}

func TestKubernetesClientListWithNoResults(t *testing.T) {
	listerClient := &ListerClientMock{}
	listerClient.
		On("List", mock.Anything, mock.Anything, mock.Anything).
		Return((error)(nil))

	client := hardware.NewKubernetesClientWithClient(listerClient)

	_, err := client.ByIP(context.Background(), "10.10.10.10")
	assert.Error(t, err)
}

func TestKubernetesClientHealthyAndClose(t *testing.T) {
	listerClient := &ListerClientMock{}
	client := hardware.NewKubernetesClientWithClient(listerClient)

	assert.True(t, client.IsHealthy(context.Background()))

	client.Close()

	assert.False(t, client.IsHealthy(context.Background()))
}

func TestK8sHardwareExport(t *testing.T) {
	userdata := "hello-world"
	hw := hardware.K8sHardware{
		Metadata: hardware.K8sHardwareMetadata{
			Userdata: &userdata,
			Instance: hardware.K8sHardwareMetadataInstance{
				ID:        "id",
				Hostname:  "hostname",
				Plan:      "plan",
				Factility: "facility",
				Tags:      []string{"foo", "bar"},
				SSHKeys:   []string{"baz", "qux"},
				OperatingSystem: hardware.K8sHardwareMetadataInstanceOperatingSystem{
					Slug:     "slug",
					Distro:   "distro",
					Version:  "version",
					ImageTag: "imagetag",
				},
				Network: hardware.K8sHardwareMetadataInstanceNetwork{
					Addresses: []hardware.K8sHardwareMetadataInstanceNetworkAddress{
						{
							AddressFamily: 4,
							Address:       "1.1.1.1",
							Public:        true,
						},
					},
				},
			},
		},
	}

	expect, err := json.Marshal(hw)
	require.NoError(t, err)

	actual, err := hw.Export()
	require.NoError(t, err)

	assert.Equal(t, expect, actual)
}

type ListerClientMock struct {
	mock.Mock
}

func (c *ListerClientMock) List(ctx context.Context, list crclient.ObjectList, opts ...crclient.ListOption) error {
	return c.Called(ctx, list, opts).Error(0)
}
