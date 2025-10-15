package nocloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"gopkg.in/yaml.v3"
)

// MockClient is a mock implementation of the NoCloud Client interface for testing.
type MockClient struct {
	instance data.NoCloudInstance
	err      error
}

func (m *MockClient) GetNoCloudInstance(_ context.Context, _ string) (data.NoCloudInstance, error) {
	if m.err != nil {
		return data.NoCloudInstance{}, m.err
	}
	return m.instance, nil
}

func TestFrontend_metaDataHandler(t *testing.T) {
	tests := []struct {
		name           string
		instance       data.NoCloudInstance
		clientErr      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "successful metadata response",
			instance: data.NoCloudInstance{
				Metadata: data.Metadata{
					InstanceID:    "server-001",
					LocalHostname: "web01.example.com",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "instance-id: server-001\nlocal-hostname: web01.example.com",
		},
		{
			name: "minimal metadata response",
			instance: data.NoCloudInstance{
				Metadata: data.Metadata{
					InstanceID:    "server-002",
					LocalHostname: "db01.example.com",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "instance-id: server-002\nlocal-hostname: db01.example.com",
		},
		{
			name:           "instance not found",
			clientErr:      ErrInstanceNotFound,
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{
				instance: tt.instance,
				err:      tt.clientErr,
			}

			frontend := New(mockClient)
			router := gin.New()
			frontend.Configure(router)

			req := httptest.NewRequest(http.MethodGet, "/nocloud/meta-data", nil)
			req.RemoteAddr = "192.168.1.10:12345"
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, strings.TrimSpace(w.Body.String()))
				assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestFrontend_userDataHandler(t *testing.T) {
	tests := []struct {
		name           string
		instance       data.NoCloudInstance
		clientErr      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "successful user-data response",
			instance: data.NoCloudInstance{
				Userdata: "#cloud-config\npackage_update: true\n",
				Metadata: data.Metadata{
					InstanceID:    "server-001",
					LocalHostname: "web01.example.com",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "#cloud-config\npackage_update: true\n",
		},
		{
			name: "empty user-data",
			instance: data.NoCloudInstance{
				Userdata: "",
				Metadata: data.Metadata{
					InstanceID:    "server-002",
					LocalHostname: "db01.example.com",
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "instance not found",
			clientErr:      ErrInstanceNotFound,
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{
				instance: tt.instance,
				err:      tt.clientErr,
			}

			frontend := New(mockClient)
			router := gin.New()
			frontend.Configure(router)

			req := httptest.NewRequest(http.MethodGet, "/nocloud/user-data", nil)
			req.RemoteAddr = "192.168.1.10:12345"
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.Equal(t, tt.expectedBody, w.Body.String())
				assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestFrontend_vendorDataHandler(t *testing.T) {
	tests := []struct {
		name           string
		instance       data.NoCloudInstance
		clientErr      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "successful vendor-data response (empty)",
			instance: data.NoCloudInstance{
				Metadata: data.Metadata{
					InstanceID:    "server-001",
					LocalHostname: "web01.example.com",
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "",
		},
		{
			name:           "instance not found",
			clientErr:      ErrInstanceNotFound,
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{
				instance: tt.instance,
				err:      tt.clientErr,
			}

			frontend := New(mockClient)
			router := gin.New()
			frontend.Configure(router)

			req := httptest.NewRequest(http.MethodGet, "/nocloud/vendor-data", nil)
			req.RemoteAddr = "192.168.1.10:12345"
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, tt.expectedBody, w.Body.String())
				assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestFrontend_networkConfigHandler(t *testing.T) {
	bondConfig := &data.NetworkConfig{
		Network: data.NetworkSpecV2{
			Version: 2,
			Ethernets: map[string]data.EthernetConfig{
				"bond0phy0": {
					Match: &data.MatchConfig{
						MACAddress: "1c:34:da:12:34:56",
					},
					SetName: "bond0phy0",
					Dhcp4:   false,
				},
				"bond0phy1": {
					Match: &data.MatchConfig{
						MACAddress: "1c:34:da:12:34:57",
					},
					SetName: "bond0phy1",
					Dhcp4:   false,
				},
			},
			Bonds: map[string]data.BondConfig{
				"bond0": {
					Interfaces: []string{"bond0phy0", "bond0phy1"},
					Parameters: data.BondParameters{
						Mode:               "802.3ad",
						MIIMonitorInterval: 100,
						LACPRate:           "fast",
						TransmitHashPolicy: "layer3+4",
						ADSelect:           "stable",
					},
					Addresses: []string{"192.168.1.10/24"},
					Gateway4:  "192.168.1.1",
				},
			},
		},
	}

	tests := []struct {
		name           string
		instance       data.NoCloudInstance
		clientErr      error
		expectedStatus int
		validateBody   func(t *testing.T, body string)
	}{
		{
			name: "successful network config response",
			instance: data.NoCloudInstance{
				NetworkConfig: bondConfig,
				Metadata: data.Metadata{
					InstanceID:    "server-001",
					LocalHostname: "web01.example.com",
				},
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body string) {
				t.Helper()
				var config data.NetworkConfig
				err := yaml.Unmarshal([]byte(body), &config)
				require.NoError(t, err)

				assert.Equal(t, 2, config.Network.Version)
				assert.Contains(t, config.Network.Bonds, "bond0")
				assert.Len(t, config.Network.Ethernets, 2)
			},
		},
		{
			name: "empty config when no network config",
			instance: data.NoCloudInstance{
				NetworkConfig: nil,
				Metadata: data.Metadata{
					InstanceID:    "server-002",
					LocalHostname: "db01.example.com",
				},
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body string) {
				t.Helper()
				assert.Equal(t, "", body)
			},
		},
		{
			name:           "instance not found",
			clientErr:      ErrInstanceNotFound,
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{
				instance: tt.instance,
				err:      tt.clientErr,
			}

			frontend := New(mockClient)
			router := gin.New()
			frontend.Configure(router)

			req := httptest.NewRequest(http.MethodGet, "/nocloud/network-config", nil)
			req.RemoteAddr = "192.168.1.10:12345"
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.validateBody != nil {
				assert.Equal(t, "text/plain", w.Header().Get("Content-Type"))
				tt.validateBody(t, w.Body.String())
			}
		})
	}
}
