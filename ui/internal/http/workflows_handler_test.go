package webhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	tinkv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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

func TestHandleWorkflowList_DisabledWorkflowsAcrossNamespaces(t *testing.T) {
	disabled := true
	wfDefault := newTestWorkflow("wf-1", "template-1", tinkv1alpha1.WorkflowStateSuccess)
	wfDefault.Spec.Disabled = &disabled
	wfOther := newTestWorkflow("wf-2", "template-2", tinkv1alpha1.WorkflowStateSuccess)
	wfOther.Namespace = "other"
	wfOther.Spec.Disabled = &disabled

	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestNamespace("other"),
		wfDefault,
		wfOther,
	)

	c, w := setupTestContext("/workflows", kubeClient)

	HandleWorkflowList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "wf-1") || !contains(body, "wf-2") {
		t.Error("response should contain both disabled workflows across namespaces")
	}
}

func TestHandleWorkflowEnable_Row(t *testing.T) {
	wf := newTestWorkflow("wf-1", "template-1", tinkv1alpha1.WorkflowStateSuccess)
	disabled := true
	wf.Spec.Disabled = &disabled
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		wf,
	)

	c, w := setupTestContext("/workflows/default/wf-1/enable", kubeClient)
	c.Request = httptest.NewRequest(http.MethodPost, "/workflows/default/wf-1/enable", nil)
	c.Request.PostForm = map[string][]string{"render": {"row"}}
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "wf-1"},
	}

	HandleWorkflowEnable(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "<tr") {
		t.Error("response should contain a <tr> row fragment")
	}
	if contains(body, "Enable this Workflow") {
		t.Error("enabled row should no longer show the Enable action")
	}
}

func TestHandleWorkflowEnable_DetailFragment(t *testing.T) {
	wf := newTestWorkflow("wf-1", "template-1", tinkv1alpha1.WorkflowStateSuccess)
	disabled := true
	wf.Spec.Disabled = &disabled
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		wf,
	)

	c, w := setupTestContext("/workflows/default/wf-1/enable", kubeClient)
	c.Request = httptest.NewRequest(http.MethodPost, "/workflows/default/wf-1/enable", nil)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "wf-1"},
	}

	HandleWorkflowEnable(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "workflow-disabled-control") {
		t.Error("response should contain the disabled-control fragment")
	}
}

func TestHandleWorkflowEnable_Forbidden(t *testing.T) {
	wf := newTestWorkflow("wf-1", "template-1", tinkv1alpha1.WorkflowStateSuccess)
	disabled := true
	wf.Spec.Disabled = &disabled

	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(newTestNamespace("default"), wf).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				return apierrors.NewForbidden(schema.GroupResource{Group: "tinkerbell.org", Resource: "workflows"}, "wf-1", nil)
			},
		}).
		Build()
	kubeClient := &KubeClient{Client: fakeClient}

	c, w := setupTestContext("/workflows/default/wf-1/enable", kubeClient)
	c.Request = httptest.NewRequest(http.MethodPost, "/workflows/default/wf-1/enable", nil)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "wf-1"},
	}

	HandleWorkflowEnable(c, testLog)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleWorkflowEnable_NotFound(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/workflows/default/nonexistent/enable", kubeClient)
	c.Request = httptest.NewRequest(http.MethodPost, "/workflows/default/nonexistent/enable", nil)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "nonexistent"},
	}

	HandleWorkflowEnable(c, testLog)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
