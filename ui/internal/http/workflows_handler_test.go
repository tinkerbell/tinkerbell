package webhttp

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
)

func TestHandleWorkflowList_Empty(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/workflows", kubeClient)

	HandleWorkflowList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleWorkflowList_WithData(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflow("wf-1", "template-1", tinkv1alpha1.WorkflowStateRunning),
		newTestWorkflow("wf-2", "template-2", tinkv1alpha1.WorkflowStateSuccess),
	)

	c, w := setupTestContext("/workflows", kubeClient)

	HandleWorkflowList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "wf-1") {
		t.Error("response should contain wf-1")
	}
	if !contains(body, "wf-2") {
		t.Error("response should contain wf-2")
	}
}

func TestHandleWorkflowList_HTMXRequest(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflow("wf-1", "template-1", tinkv1alpha1.WorkflowStateRunning),
	)

	c, w := setupTestContext("/workflows", kubeClient)
	c.Request.Header.Set("HX-Request", "true")

	HandleWorkflowList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleWorkflowDetail_Found(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestWorkflow("wf-1", "template-1", tinkv1alpha1.WorkflowStateRunning),
	)

	c, w := setupTestContext("/workflows/default/wf-1", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "wf-1"},
	}

	HandleWorkflowDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleWorkflowDetail_NotFound(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/workflows/default/nonexistent", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "nonexistent"},
	}

	HandleWorkflowDetail(c, testLog)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
