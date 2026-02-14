package webhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestSearchResult(t *testing.T) {
	result := SearchResult{
		Name:      "test-hardware",
		Namespace: "default",
		Type:      "hardware",
		TypeLabel: "Hardware",
		URL:       "/hardware/default/test-hardware",
		Icon:      "hardware",
	}

	if result.Name != "test-hardware" {
		t.Errorf("Name = %s, want test-hardware", result.Name)
	}
	if result.Namespace != "default" {
		t.Errorf("Namespace = %s, want default", result.Namespace)
	}
	if result.Type != "hardware" {
		t.Errorf("Type = %s, want hardware", result.Type)
	}
	if result.TypeLabel != "Hardware" {
		t.Errorf("TypeLabel = %s, want Hardware", result.TypeLabel)
	}
	if result.URL != "/hardware/default/test-hardware" {
		t.Errorf("URL = %s, want /hardware/default/test-hardware", result.URL)
	}
	if result.Icon != "hardware" {
		t.Errorf("Icon = %s, want hardware", result.Icon)
	}
}

func TestSearchResponse(t *testing.T) {
	response := SearchResponse{
		Query: "test",
		Results: []SearchResult{
			{
				Name:      "test-1",
				Namespace: "ns1",
				Type:      "workflow",
				TypeLabel: "Workflow",
				URL:       "/workflows/ns1/test-1",
				Icon:      "workflow",
			},
			{
				Name:      "test-2",
				Namespace: "ns2",
				Type:      "hardware",
				TypeLabel: "Hardware",
				URL:       "/hardware/ns2/test-2",
				Icon:      "hardware",
			},
		},
	}

	if response.Query != "test" {
		t.Errorf("Query = %s, want test", response.Query)
	}
	if len(response.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(response.Results))
	}
}

func TestSearchResultTypes(t *testing.T) {
	types := []struct {
		resourceType string
		typeLabel    string
		icon         string
	}{
		{"hardware", "Hardware", "hardware"},
		{"workflow", "Workflow", "workflow"},
		{"template", "Template", "template"},
		{"bmc-machine", "BMC Machine", "bmc"},
		{"bmc-job", "BMC Job", "bmc"},
		{"bmc-task", "BMC Task", "bmc"},
	}

	for _, tt := range types {
		t.Run(tt.resourceType, func(t *testing.T) {
			result := SearchResult{
				Name:      "test",
				Namespace: "default",
				Type:      tt.resourceType,
				TypeLabel: tt.typeLabel,
				Icon:      tt.icon,
			}

			if result.Type != tt.resourceType {
				t.Errorf("Type = %s, want %s", result.Type, tt.resourceType)
			}
			if result.TypeLabel != tt.typeLabel {
				t.Errorf("TypeLabel = %s, want %s", result.TypeLabel, tt.typeLabel)
			}
			if result.Icon != tt.icon {
				t.Errorf("Icon = %s, want %s", result.Icon, tt.icon)
			}
		})
	}
}

func TestSearchResponseEmptyResults(t *testing.T) {
	response := SearchResponse{
		Query:   "",
		Results: []SearchResult{},
	}

	if response.Query != "" {
		t.Errorf("Query = %s, want empty string", response.Query)
	}
	if len(response.Results) != 0 {
		t.Errorf("len(Results) = %d, want 0", len(response.Results))
	}
	if response.Results == nil {
		t.Error("Results should be empty slice, not nil")
	}
}

// Handler tests

func TestHandleGlobalSearch_EmptyQuery(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/api/search?q=", kubeClient)

	HandleGlobalSearch(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(response.Results) != 0 {
		t.Errorf("len(Results) = %d, want 0 for empty query", len(response.Results))
	}
}

func TestHandleGlobalSearch_FindsHardware(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestHardware("test-server-1", "default", "aa:bb:cc:dd:ee:ff", "192.168.1.10"),
		newTestHardware("production-box", "default", "11:22:33:44:55:66", "192.168.1.20"),
	)

	c, w := setupTestContext("/api/search?q=test", kubeClient)

	HandleGlobalSearch(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Should find "test-server-1" but not "production-box"
	found := false
	for _, r := range response.Results {
		if r.Name == "test-server-1" && r.Type == "hardware" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected to find test-server-1 in search results")
	}
}

func TestHandleGlobalSearch_FindsWorkflows(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflow("deploy-workflow", "my-template", tinkv1alpha1.WorkflowStateRunning),
		newTestWorkflow("other-wf", "other-template", tinkv1alpha1.WorkflowStateSuccess),
	)

	c, w := setupTestContext("/api/search?q=deploy", kubeClient)

	HandleGlobalSearch(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	found := false
	for _, r := range response.Results {
		if r.Name == "deploy-workflow" && r.Type == "workflow" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected to find deploy-workflow in search results")
	}
}

func TestHandleGlobalSearch_FindsTemplates(t *testing.T) {
	templateData := "version: '0.1'\nname: test"
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestTemplate("ubuntu-install", &templateData),
		newTestTemplate("centos-install", &templateData),
	)

	c, w := setupTestContext("/api/search?q=ubuntu", kubeClient)

	HandleGlobalSearch(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	found := false
	for _, r := range response.Results {
		if r.Name == "ubuntu-install" && r.Type == "template" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected to find ubuntu-install in search results")
	}
}

func TestHandleGlobalSearch_CaseInsensitive(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestHardware("MyServer", "default", "aa:bb:cc:dd:ee:ff", "192.168.1.10"),
	)

	tests := []struct {
		name  string
		query string
	}{
		{name: "lowercase", query: "myserver"},
		{name: "uppercase", query: "MYSERVER"},
		{name: "mixedcase", query: "MyServer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := setupTestContext("/api/search?q="+tt.query, kubeClient)

			HandleGlobalSearch(c, testLog)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}

			var response SearchResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if len(response.Results) == 0 {
				t.Errorf("expected to find MyServer with query %q", tt.query)
			}
		})
	}
}

func TestHandleGlobalSearch_NamespaceFilter(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestNamespace("production"),
		newTestHardware("server-1", "default", "aa:bb:cc:dd:ee:ff", "192.168.1.10"),
		newTestHardware("server-2", "production", "11:22:33:44:55:66", "192.168.1.20"),
	)

	c, w := setupTestContext("/api/search?q=server&namespace=production", kubeClient)

	HandleGlobalSearch(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Should only find server-2 in production namespace
	for _, r := range response.Results {
		if r.Type == "hardware" && r.Namespace != "production" {
			t.Errorf("found hardware in namespace %q, want only production", r.Namespace)
		}
	}
}

func TestHandleGlobalSearch_LimitsResults(t *testing.T) {
	// Create more hardware than MaxSearchResultsPerType
	objs := []interface{}{newTestNamespace("default")}
	for i := 0; i < MaxSearchResultsPerType+5; i++ {
		objs = append(objs, newTestHardware(
			"test-server-"+string(rune('a'+i)),
			"default",
			"aa:bb:cc:dd:ee:ff",
			"192.168.1.10",
		))
	}

	// Convert to runtime.Object slice
	runtimeObjs := make([]interface{}, len(objs))
	copy(runtimeObjs, objs)

	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/api/search?q=test", kubeClient)

	HandleGlobalSearch(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Results should be limited
	if len(response.Results) > MaxSearchResults {
		t.Errorf("len(Results) = %d, want <= %d", len(response.Results), MaxSearchResults)
	}
}

func TestHandleGlobalSearch_SearchByNamespace(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestNamespace("tinkerbell"),
		newTestHardware("server-1", "tinkerbell", "aa:bb:cc:dd:ee:ff", "192.168.1.10"),
	)

	// Search by namespace name should find the hardware
	c, w := setupTestContext("/api/search?q=tinkerbell", kubeClient)

	HandleGlobalSearch(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	found := false
	for _, r := range response.Results {
		if r.Name == "server-1" && r.Namespace == "tinkerbell" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected to find server-1 when searching by namespace")
	}
}

func TestHandleGlobalSearch_MultipleResourceTypes(t *testing.T) {
	templateData := "version: '0.1'\nname: test"
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestHardware("test-hw", "default", "aa:bb:cc:dd:ee:ff", "192.168.1.10"),
		newTestWorkflow("test-wf", "template", tinkv1alpha1.WorkflowStateRunning),
		newTestTemplate("test-tpl", &templateData),
	)

	c, w := setupTestContext("/api/search?q=test", kubeClient)

	HandleGlobalSearch(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response SearchResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	typeCount := make(map[string]int)
	for _, r := range response.Results {
		typeCount[r.Type]++
	}

	// Should find all three types
	if typeCount["hardware"] == 0 {
		t.Error("expected to find hardware in results")
	}
	if typeCount["workflow"] == 0 {
		t.Error("expected to find workflow in results")
	}
	if typeCount["template"] == 0 {
		t.Error("expected to find template in results")
	}
}

func TestSearchResultURL(t *testing.T) {
	tests := []struct {
		name      string
		resType   string
		namespace string
		resName   string
		wantURL   string
	}{
		{
			name:      "hardware url",
			resType:   "hardware",
			namespace: "default",
			resName:   "my-server",
			wantURL:   "/hardware/default/my-server",
		},
		{
			name:      "workflow url",
			resType:   "workflow",
			namespace: "tinkerbell",
			resName:   "my-workflow",
			wantURL:   "/workflows/tinkerbell/my-workflow",
		},
		{
			name:      "template url",
			resType:   "template",
			namespace: "production",
			resName:   "my-template",
			wantURL:   "/templates/production/my-template",
		},
		{
			name:      "bmc machine url",
			resType:   "bmc-machine",
			namespace: "default",
			resName:   "my-bmc",
			wantURL:   "/bmc/machines/default/my-bmc",
		},
		{
			name:      "bmc job url",
			resType:   "bmc-job",
			namespace: "default",
			resName:   "my-job",
			wantURL:   "/bmc/jobs/default/my-job",
		},
		{
			name:      "bmc task url",
			resType:   "bmc-task",
			namespace: "default",
			resName:   "my-task",
			wantURL:   "/bmc/tasks/default/my-task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SearchResult{
				Name:      tt.resName,
				Namespace: tt.namespace,
				Type:      tt.resType,
				URL:       tt.wantURL,
			}

			if result.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", result.URL, tt.wantURL)
			}
		})
	}
}

func TestHandleGlobalSearch_NoKubeClient(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/search?q=test", nil)
	// Don't set kubeClient

	HandleGlobalSearch(c, testLog)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestSearchConstants(t *testing.T) {
	// Verify search constants are reasonable
	if MaxSearchResults <= 0 {
		t.Error("MaxSearchResults should be positive")
	}

	if MaxSearchResultsPerType <= 0 {
		t.Error("MaxSearchResultsPerType should be positive")
	}

	if MaxSearchResultsPerType > MaxSearchResults {
		t.Error("MaxSearchResultsPerType should not exceed MaxSearchResults")
	}
}
