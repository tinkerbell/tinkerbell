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
	bondConfig := map[string]interface{}{
		"version": 1,
		"config": []interface{}{
			map[string]interface{}{
				"type":        "physical",
				"name":        "eno1",
				"mac_address": "1c:34:da:12:34:56",
				"mtu":         1500,
			},
			map[string]interface{}{
				"type":        "physical",
				"name":        "eno2",
				"mac_address": "1c:34:da:12:34:57",
				"mtu":         1500,
			},
			map[string]interface{}{
				"type":            "bond",
				"name":            "bond0",
				"bond_interfaces": []string{"eno1", "eno2"},
				"params": map[string]interface{}{
					"bond-mode":   "802.3ad",
					"bond-miimon": 100,
				},
				"subnets": []interface{}{
					map[string]interface{}{
						"type":    "static",
						"address": "192.168.1.10/24",
						"gateway": "192.168.1.1",
					},
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
				var config map[string]interface{}
				err := yaml.Unmarshal([]byte(body), &config)
				require.NoError(t, err)

				assert.Equal(t, 1, config["version"])
				assert.Contains(t, config, "config")

				configItems := config["config"].([]interface{})
				assert.Len(t, configItems, 3) // 2 physical + 1 bond
			},
		},
		{
			name: "fallback config when no network config",
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
				var config map[string]interface{}
				err := yaml.Unmarshal([]byte(body), &config)
				require.NoError(t, err)

				assert.Equal(t, 1, config["version"])
				configItems := config["config"].([]interface{})
				assert.Len(t, configItems, 1) // fallback DHCP config

				item := configItems[0].(map[string]interface{})
				assert.Equal(t, "physical", item["type"])
				assert.Equal(t, "eno1", item["name"])
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
