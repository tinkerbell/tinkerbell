package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/api"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHandlerDiscover(t *testing.T) {
	tests := map[string]struct {
		id           string
		attrs        *data.AgentAttributes
		existingHw   *tinkerbell.Hardware
		wantHardware *tinkerbell.Hardware
		clientErrors map[string]error
		wantErr      bool
		wantCreated  bool
	}{
		"hardware doesn't exist, creates new one": {
			id: "test-id",
			attrs: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
				},
				Memory: &data.Memory{
					Total:  toPtr("8GB"),
					Usable: toPtr("7GB"),
				},
				NetworkInterfaces: []*data.Network{
					{
						Name: toPtr("eth0"),
						Mac:  toPtr("00:11:22:33:44:55"),
					},
				},
				BlockDevices: []*data.Block{
					{
						Name: toPtr("sda"),
					},
				},
				BIOS: &data.BIOS{
					Vendor:  toPtr("TestVendor"),
					Version: toPtr("1.0.0"),
				},
				Chassis: &data.Chassis{
					Vendor: toPtr("TestManufacturer"),
					Serial: toPtr("TestType"),
				},
			},
			wantErr:     false,
			wantCreated: true,
			wantHardware: &tinkerbell.Hardware{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "discovery-test-id",
					Namespace: "test-namespace",
					Labels:    map[string]string{"tinkerbell.org/auto-discovered": "true"},
					Annotations: map[string]string{
						"tinkerbell.org/agent-attributes": `{"cpu":{"totalCores":4,"totalThreads":8},"memory":{"total":"8GB","usable":"7GB"},"blockDevices":[{"name":"sda"}],"networkInterfaces":[{"name":"eth0","mac":"00:11:22:33:44:55"}],"chassis":{"serial":"TestType","vendor":"TestManufacturer"},"bios":{"vendor":"TestVendor","version":"1.0.0"}}`,
					},
					ResourceVersion: "1",
				},
				Spec: tinkerbell.HardwareSpec{
					AgentID:    "test-id",
					Interfaces: []tinkerbell.Interface{{DHCP: &tinkerbell.DHCP{MAC: "00:11:22:33:44:55"}}},
				},
			},
		},
		"hardware already exists, doesn't modify": {
			id: "existing-id",
			attrs: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
				},
			},
			existingHw: &tinkerbell.Hardware{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-id",
					Namespace: "test-namespace",
				},
			},
			wantErr:     false,
			wantCreated: false,
		},
		"get hardware error": {
			id: "error-id",
			attrs: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
				},
			},
			clientErrors: map[string]error{
				"get": errors.New("error getting hardware"),
			},
			wantErr:     true,
			wantCreated: false,
		},
		"create hardware error": {
			id: "create-error-id",
			attrs: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
				},
			},
			clientErrors: map[string]error{
				"create": errors.New("error creating hardware"),
			},
			wantErr:     true,
			wantCreated: false,
		},
		"nil attributes error": {
			id:          "no-attrs-id",
			attrs:       nil,
			wantErr:     false,
			wantCreated: false,
		},
		"hardware with an invalid mac address": {
			id: "test-id",
			attrs: &data.AgentAttributes{
				CPU: &data.CPU{
					TotalCores:   toPtr(uint32(4)),
					TotalThreads: toPtr(uint32(8)),
				},
				Memory: &data.Memory{
					Total:  toPtr("8GB"),
					Usable: toPtr("7GB"),
				},
				NetworkInterfaces: []*data.Network{
					{
						Name: toPtr("eth0"),
						Mac:  toPtr("00:11:22:33:44:55"),
					},
					{
						Name: toPtr("tunl0"),
						Mac:  toPtr("00:00:00:00"), // This is a real example from a Raspberry Pi 4b
					},
				},
				BlockDevices: []*data.Block{
					{
						Name: toPtr("sda"),
					},
				},
				BIOS: &data.BIOS{
					Vendor:  toPtr("TestVendor"),
					Version: toPtr("1.0.0"),
				},
				Chassis: &data.Chassis{
					Vendor: toPtr("TestManufacturer"),
					Serial: toPtr("TestType"),
				},
			},
			wantErr:     false,
			wantCreated: true,
			wantHardware: &tinkerbell.Hardware{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "discovery-test-id",
					Namespace: "test-namespace",
					Labels:    map[string]string{"tinkerbell.org/auto-discovered": "true"},
					Annotations: map[string]string{
						"tinkerbell.org/agent-attributes": `{"cpu":{"totalCores":4,"totalThreads":8},"memory":{"total":"8GB","usable":"7GB"},"blockDevices":[{"name":"sda"}],"networkInterfaces":[{"name":"eth0","mac":"00:11:22:33:44:55"},{"name":"tunl0","mac":"00:00:00:00"}],"chassis":{"serial":"TestType","vendor":"TestManufacturer"},"bios":{"vendor":"TestVendor","version":"1.0.0"}}`,
					},
					ResourceVersion: "1",
				},
				Spec: tinkerbell.HardwareSpec{
					AgentID:    "test-id",
					Interfaces: []tinkerbell.Interface{{DHCP: &tinkerbell.DHCP{MAC: "00:11:22:33:44:55"}}},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create a scheme with the necessary types
			scheme := runtime.NewScheme()
			_ = api.AddToSchemeTinkerbell(scheme)

			// Create fake client builder
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)

			// Add existing hardware if provided
			if tc.existingHw != nil {
				clientBuilder = clientBuilder.WithObjects(tc.existingHw)
			}

			// Create the fake client
			fakeClient := &mockClient{
				Client:      clientBuilder.Build(),
				errorMap:    tc.clientErrors,
				createdObjs: make(map[string]runtime.Object),
			}

			// Create mock discovery client
			mockDiscoveryClient := &mockAutoDiscoveryClient{
				client: fakeClient,
			}

			// Create the handler with the fake client
			handler := &Handler{
				Logger: logr.Discard(),
				AutoCapabilities: AutoCapabilities{
					Discovery: AutoDiscovery{
						Enabled:                  true,
						Namespace:                "test-namespace",
						AutoDiscoveryReadCreator: mockDiscoveryClient,
					},
				},
			}

			// Call Discover method
			hw, err := handler.Discover(context.Background(), tc.id, tc.attrs)

			// Check for expected error
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// If an existing hardware was provided, verify it wasn't modified
			if tc.existingHw != nil {
				hw := &tinkerbell.Hardware{}
				err = fakeClient.Get(context.Background(), types.NamespacedName{
					Name:      tc.id,
					Namespace: "test-namespace",
				}, hw)
				if err != nil {
					t.Fatalf("error getting hardware: %v", err)
				}

				// Verify it's the same as the original
				if hw.ResourceVersion != tc.existingHw.ResourceVersion {
					t.Errorf("hardware was modified when it shouldn't have been")
				}
				return
			}

			// Check if hardware was created when expected
			if tc.wantCreated {
				if diff := cmp.Diff(tc.wantHardware, hw); diff != "" {
					t.Log(hw.Annotations)
					t.Errorf("unexpected hardware created (-want +got):\n%s", diff)
				}
			}
		})
	}
}

// mockClient implements client.Client with the ability to inject errors.
type mockClient struct {
	client.Client
	errorMap    map[string]error
	createdObjs map[string]runtime.Object
}

func (m *mockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	if err, ok := m.errorMap["get"]; ok {
		return err
	}

	err := m.Client.Get(ctx, key, obj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// This is expected for the "create new" test case
			return err
		}
		return err
	}
	return nil
}

func (m *mockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err, ok := m.errorMap["create"]; ok {
		return err
	}

	// Store the object that was created for later verification
	hw, ok := obj.(*tinkerbell.Hardware)
	if ok {
		m.createdObjs[hw.Name] = obj
	}

	return m.Client.Create(ctx, obj, opts...)
}

// mockAutoDiscoveryClient implements AutoDiscoveryReaderCreator.
type mockAutoDiscoveryClient struct {
	client client.Client
}

func (m *mockAutoDiscoveryClient) ReadHardware(ctx context.Context, id, namespace string) (*tinkerbell.Hardware, error) {
	hw := &tinkerbell.Hardware{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      id,
		Namespace: namespace,
	}, hw)
	return hw, err
}

func (m *mockAutoDiscoveryClient) CreateHardware(ctx context.Context, hw *tinkerbell.Hardware) error {
	return m.client.Create(ctx, hw)
}
