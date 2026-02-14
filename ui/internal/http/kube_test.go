package webhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	bmcv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/ui/templates"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestGetHardwareInterfaces(t *testing.T) {
	tests := []struct {
		name     string
		hardware tinkv1alpha1.Hardware
		want     []templates.HardwareInterface
	}{
		{
			name: "no interfaces",
			hardware: tinkv1alpha1.Hardware{
				Spec: tinkv1alpha1.HardwareSpec{
					Interfaces: nil,
				},
			},
			want: nil,
		},
		{
			name: "single interface with MAC and IP",
			hardware: tinkv1alpha1.Hardware{
				Spec: tinkv1alpha1.HardwareSpec{
					Interfaces: []tinkv1alpha1.Interface{
						{
							DHCP: &tinkv1alpha1.DHCP{
								MAC: "aa:bb:cc:dd:ee:ff",
								IP: &tinkv1alpha1.IP{
									Address: "192.168.1.100",
								},
							},
						},
					},
				},
			},
			want: []templates.HardwareInterface{
				{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.100"},
			},
		},
		{
			name: "interface with MAC only",
			hardware: tinkv1alpha1.Hardware{
				Spec: tinkv1alpha1.HardwareSpec{
					Interfaces: []tinkv1alpha1.Interface{
						{
							DHCP: &tinkv1alpha1.DHCP{
								MAC: "aa:bb:cc:dd:ee:ff",
							},
						},
					},
				},
			},
			want: []templates.HardwareInterface{
				{MAC: "aa:bb:cc:dd:ee:ff", IP: ""},
			},
		},
		{
			name: "multiple interfaces",
			hardware: tinkv1alpha1.Hardware{
				Spec: tinkv1alpha1.HardwareSpec{
					Interfaces: []tinkv1alpha1.Interface{
						{
							DHCP: &tinkv1alpha1.DHCP{
								MAC: "aa:bb:cc:dd:ee:01",
								IP: &tinkv1alpha1.IP{
									Address: "192.168.1.101",
								},
							},
						},
						{
							DHCP: &tinkv1alpha1.DHCP{
								MAC: "aa:bb:cc:dd:ee:02",
								IP: &tinkv1alpha1.IP{
									Address: "192.168.1.102",
								},
							},
						},
					},
				},
			},
			want: []templates.HardwareInterface{
				{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.101"},
				{MAC: "aa:bb:cc:dd:ee:02", IP: "192.168.1.102"},
			},
		},
		{
			name: "interface with nil DHCP",
			hardware: tinkv1alpha1.Hardware{
				Spec: tinkv1alpha1.HardwareSpec{
					Interfaces: []tinkv1alpha1.Interface{
						{
							DHCP: nil,
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "interface with empty MAC and IP skipped",
			hardware: tinkv1alpha1.Hardware{
				Spec: tinkv1alpha1.HardwareSpec{
					Interfaces: []tinkv1alpha1.Interface{
						{
							DHCP: &tinkv1alpha1.DHCP{
								MAC: "",
								IP:  nil,
							},
						},
					},
				},
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetHardwareInterfaces(tt.hardware)

			if len(got) != len(tt.want) {
				t.Errorf("GetHardwareInterfaces() returned %d interfaces, want %d", len(got), len(tt.want))
				return
			}

			for i, iface := range got {
				if iface.MAC != tt.want[i].MAC {
					t.Errorf("Interface[%d].MAC = %s, want %s", i, iface.MAC, tt.want[i].MAC)
				}
				if iface.IP != tt.want[i].IP {
					t.Errorf("Interface[%d].IP = %s, want %s", i, iface.IP, tt.want[i].IP)
				}
			}
		})
	}
}

func TestGetHardwareStatus(t *testing.T) {
	tests := []struct {
		name     string
		hardware tinkv1alpha1.Hardware
		want     string
	}{
		{
			name: "provisioning state",
			hardware: tinkv1alpha1.Hardware{
				Status: tinkv1alpha1.HardwareStatus{
					State: "provisioning",
				},
			},
			want: "Provisioning",
		},
		{
			name: "failed state",
			hardware: tinkv1alpha1.Hardware{
				Status: tinkv1alpha1.HardwareStatus{
					State: "failed",
				},
			},
			want: "Offline",
		},
		{
			name: "empty state defaults to Online",
			hardware: tinkv1alpha1.Hardware{
				Status: tinkv1alpha1.HardwareStatus{
					State: "",
				},
			},
			want: "Online",
		},
		{
			name: "unknown state defaults to Online",
			hardware: tinkv1alpha1.Hardware{
				Status: tinkv1alpha1.HardwareStatus{
					State: "something-else",
				},
			},
			want: "Online",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetHardwareStatus(tt.hardware)
			if got != tt.want {
				t.Errorf("GetHardwareStatus() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestGetBMCJobStatus(t *testing.T) {
	tests := []struct {
		name       string
		conditions []bmcv1alpha1.JobCondition
		want       string
	}{
		{
			name:       "no conditions",
			conditions: nil,
			want:       "Unknown",
		},
		{
			name: "completed condition",
			conditions: []bmcv1alpha1.JobCondition{
				{
					Type:   bmcv1alpha1.JobCompleted,
					Status: bmcv1alpha1.ConditionTrue,
				},
			},
			want: "Completed",
		},
		{
			name: "failed condition",
			conditions: []bmcv1alpha1.JobCondition{
				{
					Type:   bmcv1alpha1.JobFailed,
					Status: bmcv1alpha1.ConditionTrue,
				},
			},
			want: "Failed",
		},
		{
			name: "running condition",
			conditions: []bmcv1alpha1.JobCondition{
				{
					Type:   bmcv1alpha1.JobRunning,
					Status: bmcv1alpha1.ConditionTrue,
				},
			},
			want: "Running",
		},
		{
			name: "condition with false status ignored",
			conditions: []bmcv1alpha1.JobCondition{
				{
					Type:   bmcv1alpha1.JobCompleted,
					Status: bmcv1alpha1.ConditionFalse,
				},
			},
			want: "Unknown",
		},
		{
			name: "multiple conditions returns first true match",
			conditions: []bmcv1alpha1.JobCondition{
				{
					Type:   bmcv1alpha1.JobRunning,
					Status: bmcv1alpha1.ConditionFalse,
				},
				{
					Type:   bmcv1alpha1.JobCompleted,
					Status: bmcv1alpha1.ConditionTrue,
				},
				{
					Type:   bmcv1alpha1.JobFailed,
					Status: bmcv1alpha1.ConditionTrue,
				},
			},
			want: "Completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBMCJobStatus(tt.conditions)
			if got != tt.want {
				t.Errorf("GetBMCJobStatus() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestIsHTMXRequest(t *testing.T) {
	tests := []struct {
		name       string
		headerVal  string
		wantResult bool
	}{
		{
			name:       "htmx request with true",
			headerVal:  "true",
			wantResult: true,
		},
		{
			name:       "not htmx request - empty header",
			headerVal:  "",
			wantResult: false,
		},
		{
			name:       "not htmx request - false value",
			headerVal:  "false",
			wantResult: false,
		},
		{
			name:       "not htmx request - random value",
			headerVal:  "something",
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.headerVal != "" {
				c.Request.Header.Set("HX-Request", tt.headerVal)
			}

			got := IsHTMXRequest(c)
			if got != tt.wantResult {
				t.Errorf("IsHTMXRequest() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify constants are as expected
	if templates.AllNamespace != "All" {
		t.Errorf("AllNamespace = %s, want 'All'", templates.AllNamespace)
	}
	if DefaultItemsPerPage != 10 {
		t.Errorf("DefaultItemsPerPage = %d, want 10", DefaultItemsPerPage)
	}
	if MaxItemsPerPage != 100 {
		t.Errorf("MaxItemsPerPage = %d, want 100", MaxItemsPerPage)
	}
}

func TestValidateItemsPerPage(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{
			name:  "valid value returns same",
			input: 25,
			want:  25,
		},
		{
			name:  "zero returns default",
			input: 0,
			want:  DefaultItemsPerPage,
		},
		{
			name:  "negative returns default",
			input: -5,
			want:  DefaultItemsPerPage,
		},
		{
			name:  "exactly max allowed",
			input: 100,
			want:  100,
		},
		{
			name:  "exceeds max returns max",
			input: 200,
			want:  MaxItemsPerPage,
		},
		{
			name:  "minimum valid value",
			input: 1,
			want:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateItemsPerPage(tt.input)
			if got != tt.want {
				t.Errorf("ValidateItemsPerPage(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetKubeClientFromGinContext_Success(t *testing.T) {
	kubeClient := newFakeKubeClient()
	c, _ := setupTestContext("/", kubeClient)

	got, err := GetKubeClientFromGinContext(c)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if got != kubeClient {
		t.Error("returned client should match set client")
	}
}

func TestGetKubeClientFromGinContext_Missing(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	// Don't set kubeClient

	_, err := GetKubeClientFromGinContext(c)
	if err == nil {
		t.Error("expected error when kubeClient is not set")
	}
}

func TestGetKubeNamespaces_WithNamespaces(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestNamespace("kube-system"),
		newTestNamespace("tinkerbell"),
	)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	namespaces := GetKubeNamespaces(c.Request.Context(), c, kubeClient, testLog)

	if len(namespaces) != 3 {
		t.Errorf("len(namespaces) = %d, want 3", len(namespaces))
	}
}

func TestGetKubeNamespaces_NilClient(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	namespaces := GetKubeNamespaces(c.Request.Context(), c, nil, testLog)

	if len(namespaces) != 0 {
		t.Errorf("namespaces = %v, want []", namespaces)
	}
}
