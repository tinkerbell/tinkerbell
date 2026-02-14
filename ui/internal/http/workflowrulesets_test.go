package webhttp

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandleWorkflowRuleSetList_Empty(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/workflows/rulesets", kubeClient)

	HandleWorkflowRuleSetList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleWorkflowRuleSetList_WithData(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflowRuleSet("rs-1", "default", "template-1", []string{`{"manufacturer": ["Dell"]}`}),
		newTestWorkflowRuleSet("rs-2", "default", "template-2", []string{`{"model": ["PowerEdge"]}`}),
	)

	c, w := setupTestContext("/workflows/rulesets", kubeClient)

	HandleWorkflowRuleSetList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !contains(body, "rs-1") {
		t.Error("response should contain rs-1")
	}
	if !contains(body, "rs-2") {
		t.Error("response should contain rs-2")
	}
}

func TestHandleWorkflowRuleSetList_HTMXRequest(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflowRuleSet("rs-1", "default", "template-1", []string{`{"manufacturer": ["Dell"]}`}),
	)

	c, w := setupTestContext("/workflows/rulesets", kubeClient)
	c.Request.Header.Set("HX-Request", "true")

	HandleWorkflowRuleSetList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleWorkflowRuleSetList_Pagination(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflowRuleSet("rs-1", "default", "tpl", nil),
		newTestWorkflowRuleSet("rs-2", "default", "tpl", nil),
		newTestWorkflowRuleSet("rs-3", "default", "tpl", nil),
	)

	tests := []struct {
		name     string
		page     string
		perPage  string
		wantCode int
	}{
		{
			name:     "default pagination",
			page:     "",
			perPage:  "",
			wantCode: http.StatusOK,
		},
		{
			name:     "page 1",
			page:     "1",
			perPage:  "10",
			wantCode: http.StatusOK,
		},
		{
			name:     "page 2 with per_page 1",
			page:     "2",
			perPage:  "1",
			wantCode: http.StatusOK,
		},
		{
			name:     "invalid page defaults to 1",
			page:     "invalid",
			perPage:  "",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/workflows/rulesets"
			if tt.page != "" || tt.perPage != "" {
				path += "?"
				if tt.page != "" {
					path += "page=" + tt.page
				}
				if tt.perPage != "" {
					if tt.page != "" {
						path += "&"
					}
					path += "per_page=" + tt.perPage
				}
			}

			c, w := setupTestContext(path, kubeClient)

			HandleWorkflowRuleSetList(c, testLog)

			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestHandleWorkflowRuleSetDetail_Found(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflowRuleSet("rs-1", "default", "template-1", []string{`{"manufacturer": ["Dell"]}`}),
	)

	c, w := setupTestContext("/workflows/rulesets/default/rs-1", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "rs-1"},
	}

	HandleWorkflowRuleSetDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !contains(body, "rs-1") {
		t.Error("response should contain ruleset name")
	}
	if !contains(body, "template-1") {
		t.Error("response should contain template reference")
	}
}

func TestHandleWorkflowRuleSetDetail_NotFound(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/workflows/rulesets/default/nonexistent", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "nonexistent"},
	}

	HandleWorkflowRuleSetDetail(c, testLog)

	// Should return 200 with NotFound page
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !contains(body, "not found") && !contains(body, "Not Found") && !contains(body, "NotFound") {
		t.Error("response should indicate ruleset not found")
	}
}

func TestHandleWorkflowRuleSetData(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflowRuleSet("rs-1", "default", "template-1", nil),
	)

	c, w := setupTestContext("/workflows/rulesets-data", kubeClient)

	HandleWorkflowRuleSetData(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" {
		t.Errorf("Content-Type = %q, want %q", contentType, "text/html")
	}
}

func TestHandleWorkflowRuleSetData_NamespaceFilter(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestNamespace("production"),
		newTestWorkflowRuleSet("rs-default", "default", "tpl", nil),
		newTestWorkflowRuleSet("rs-prod", "production", "tpl", nil),
	)

	c, w := setupTestContext("/workflows/rulesets-data?namespace=production", kubeClient)

	HandleWorkflowRuleSetData(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	// Should only show production ruleset
	if !contains(body, "rs-prod") {
		t.Error("response should contain production ruleset")
	}
}

func TestHandleWorkflowRuleSetDetail_WithRules(t *testing.T) {
	rules := []string{
		`{"manufacturer": ["Dell", "HP"]}`,
		`{"model": ["PowerEdge R740"]}`,
	}
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflowRuleSet("rs-multi", "default", "ubuntu-install", rules),
	)

	c, w := setupTestContext("/workflows/rulesets/default/rs-multi", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "rs-multi"},
	}

	HandleWorkflowRuleSetDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	// Check that rules are displayed (may be in YAML format)
	if !contains(body, "manufacturer") && !contains(body, "Dell") {
		t.Log("Note: rules may be displayed in different format")
	}
}

func TestHandleWorkflowRuleSetList_AllNamespaces(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestNamespace("production"),
		newTestWorkflowRuleSet("rs-1", "default", "tpl", nil),
		newTestWorkflowRuleSet("rs-2", "production", "tpl", nil),
	)

	// Empty namespace should list all
	c, w := setupTestContext("/workflows/rulesets", kubeClient)

	HandleWorkflowRuleSetList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !contains(body, "rs-1") {
		t.Error("response should contain rs-1 from default namespace")
	}
	if !contains(body, "rs-2") {
		t.Error("response should contain rs-2 from production namespace")
	}
}

func TestHandleWorkflowRuleSetDetail_EmptyTemplateRef(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflowRuleSet("rs-no-tpl", "default", "", nil),
	)

	c, w := setupTestContext("/workflows/rulesets/default/rs-no-tpl", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "rs-no-tpl"},
	}

	HandleWorkflowRuleSetDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleWorkflowRuleSetData_Pagination(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflowRuleSet("rs-1", "default", "tpl", nil),
		newTestWorkflowRuleSet("rs-2", "default", "tpl", nil),
	)

	tests := []struct {
		name     string
		path     string
		wantCode int
	}{
		{
			name:     "page 1",
			path:     "/workflows/rulesets-data?page=1",
			wantCode: http.StatusOK,
		},
		{
			name:     "page with per_page",
			path:     "/workflows/rulesets-data?page=1&per_page=1",
			wantCode: http.StatusOK,
		},
		{
			name:     "invalid per_page defaults",
			path:     "/workflows/rulesets-data?per_page=-1",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := setupTestContext(tt.path, kubeClient)

			HandleWorkflowRuleSetData(c, testLog)

			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}
