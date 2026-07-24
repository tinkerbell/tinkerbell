package webhttp

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandleHardwareList_Empty(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/hardware", kubeClient)

	HandleHardwareList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("Content-Type = %s, want text/html", ct)
	}
}

func TestHandleHardwareList_WithData(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestHardware("hw-1", "default", "aa:bb:cc:dd:ee:01", "192.168.1.1"),
		newTestHardware("hw-2", "default", "aa:bb:cc:dd:ee:02", "192.168.1.2"),
	)

	c, w := setupTestContext("/hardware", kubeClient)

	HandleHardwareList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "hw-1") {
		t.Error("response should contain hw-1")
	}
	if !contains(body, "hw-2") {
		t.Error("response should contain hw-2")
	}
}

func TestHandleHardwareList_HTMXRequest(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestHardware("hw-1", "default", "aa:bb:cc:dd:ee:01", "192.168.1.1"),
	)

	c, w := setupTestContext("/hardware", kubeClient)
	c.Request.Header.Set("HX-Request", "true")

	HandleHardwareList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	// HTMX requests return partial content (table only)
	body := w.Body.String()
	if !contains(body, "hw-1") {
		t.Error("HTMX response should contain hw-1")
	}
}

func TestHandleHardwareList_Pagination(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestHardware("hw-1", "default", "aa:bb:cc:dd:ee:01", "192.168.1.1"),
		newTestHardware("hw-2", "default", "aa:bb:cc:dd:ee:02", "192.168.1.2"),
	)

	c, w := setupTestContext("/hardware?page=1&per_page=1", kubeClient)

	HandleHardwareList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleHardwareList_NamespaceFilter(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("ns1"),
		newTestNamespace("ns2"),
		newTestHardware("hw-1", "ns1", "aa:bb:cc:dd:ee:01", "192.168.1.1"),
		newTestHardware("hw-2", "ns2", "aa:bb:cc:dd:ee:02", "192.168.1.2"),
	)

	c, w := setupTestContext("/hardware?namespace=ns1", kubeClient)

	HandleHardwareList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "hw-1") {
		t.Error("response should contain hw-1 (ns1)")
	}
}

func TestHandleHardwareDetail_Found(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestHardware("hw-1", "default", "aa:bb:cc:dd:ee:01", "192.168.1.1"),
	)

	c, w := setupTestContext("/hardware/default/hw-1", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "hw-1"},
	}

	HandleHardwareDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "hw-1") {
		t.Error("response should contain hw-1")
	}
}

func TestHandleHardwareDetail_WithBMCInventory(t *testing.T) {
	hw := newTestHardware("hw-1", "default", "aa:bb:cc:dd:ee:01", "192.168.1.1")
	hw.Status.BMCInventory = &tinkv1alpha1.BMCInventory{
		LastUpdated:      &metav1.Time{},
		CollectionMethod: "redfish",
		BIOS: &tinkv1alpha1.BMCFirmwareComponent{
			Vendor:            "Dell Inc.",
			FirmwareInstalled: "2.10.2",
		},
		PSUs: []tinkv1alpha1.BMCPSUComponent{
			{
				Vendor: "Dell",
				Status: &tinkv1alpha1.BMCStatus{Health: "OK"},
			},
		},
	}

	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		hw,
	)

	c, w := setupTestContext("/hardware/default/hw-1", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "hw-1"},
	}

	HandleHardwareDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "BMC Inventory") {
		t.Error("response should contain the BMC Inventory section")
	}
	if !contains(body, "Dell Inc.") {
		t.Error("response should contain the BIOS vendor")
	}
	if !contains(body, "2.10.2") {
		t.Error("response should contain the BIOS firmware version")
	}
}

func TestHandleHardwareDetail_WithoutBMCInventory(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestHardware("hw-1", "default", "aa:bb:cc:dd:ee:01", "192.168.1.1"),
	)

	c, w := setupTestContext("/hardware/default/hw-1", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "hw-1"},
	}

	HandleHardwareDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if contains(body, "BMC Inventory") {
		t.Error("response should not contain the BMC Inventory section when bmcInventory is unset")
	}
}

func TestHandleHardwareDetail_NotFound(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/hardware/default/nonexistent", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "nonexistent"},
	}

	HandleHardwareDetail(c, testLog)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleHome_Empty(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/", kubeClient)

	HandleHome(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleHome_WithHardware(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestHardware("hw-1", "default", "aa:bb:cc:dd:ee:01", "192.168.1.1"),
	)

	c, w := setupTestContext("/", kubeClient)

	HandleHome(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "hw-1") {
		t.Error("response should contain hw-1")
	}
}
