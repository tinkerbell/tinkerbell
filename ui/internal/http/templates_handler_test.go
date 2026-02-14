package webhttp

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandleTemplateList_Empty(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/templates", kubeClient)

	HandleTemplateList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleTemplateList_WithData(t *testing.T) {
	data := "actions:\n  - name: test"
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestTemplate("tmpl-1", &data),
		newTestTemplate("tmpl-2", nil),
	)

	c, w := setupTestContext("/templates", kubeClient)

	HandleTemplateList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "tmpl-1") {
		t.Error("response should contain tmpl-1")
	}
}

func TestHandleTemplateDetail_Found(t *testing.T) {
	data := "actions:\n  - name: test"
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestTemplate("tmpl-1", &data),
	)

	c, w := setupTestContext("/templates/default/tmpl-1", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "tmpl-1"},
	}

	HandleTemplateDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleTemplateDetail_NotFound(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/templates/default/nonexistent", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "nonexistent"},
	}

	HandleTemplateDetail(c, testLog)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
