package webhttp

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
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
