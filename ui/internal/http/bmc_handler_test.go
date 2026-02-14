package webhttp

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	bmcv1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/bmc"
)

// BMC Machine handler tests.

func TestHandleBMCMachineList_Empty(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/bmc/machines", kubeClient)

	HandleBMCMachineList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleBMCMachineList_WithData(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestBMCMachine("bmc-1", "default", "192.168.1.100", bmcv1alpha1.On),
		newTestBMCMachine("bmc-2", "default", "192.168.1.101", bmcv1alpha1.Off),
	)

	c, w := setupTestContext("/bmc/machines", kubeClient)

	HandleBMCMachineList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "bmc-1") {
		t.Error("response should contain bmc-1")
	}
}

func TestHandleBMCMachineDetail_Found(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestBMCMachine("bmc-1", "default", "192.168.1.100", bmcv1alpha1.On),
	)

	c, w := setupTestContext("/bmc/machines/default/bmc-1", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "bmc-1"},
	}

	HandleBMCMachineDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleBMCMachineDetail_NotFound(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/bmc/machines/default/nonexistent", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "nonexistent"},
	}

	HandleBMCMachineDetail(c, testLog)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// BMC Job handler tests.

func TestHandleBMCJobList_Empty(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/bmc/jobs", kubeClient)

	HandleBMCJobList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleBMCJobList_WithData(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestBMCJob("job-1", "default", "bmc-1", []bmcv1alpha1.JobCondition{
			{Type: bmcv1alpha1.JobCompleted, Status: bmcv1alpha1.ConditionTrue},
		}),
	)

	c, w := setupTestContext("/bmc/jobs", kubeClient)

	HandleBMCJobList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "job-1") {
		t.Error("response should contain job-1")
	}
}

func TestHandleBMCJobDetail_Found(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestBMCJob("job-1", "default", "bmc-1", []bmcv1alpha1.JobCondition{
			{Type: bmcv1alpha1.JobCompleted, Status: bmcv1alpha1.ConditionTrue},
		}),
	)

	c, w := setupTestContext("/bmc/jobs/default/job-1", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "job-1"},
	}

	HandleBMCJobDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleBMCJobDetail_NotFound(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/bmc/jobs/default/nonexistent", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "nonexistent"},
	}

	HandleBMCJobDetail(c, testLog)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// BMC Task handler tests.

func TestHandleBMCTaskList_Empty(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/bmc/tasks", kubeClient)

	HandleBMCTaskList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleBMCTaskList_WithData(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestBMCTask("task-1", "default", bmcv1alpha1.PowerOn),
	)

	c, w := setupTestContext("/bmc/tasks", kubeClient)

	HandleBMCTaskList(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	if !contains(body, "task-1") {
		t.Error("response should contain task-1")
	}
}

func TestHandleBMCTaskDetail_Found(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
		newTestBMCTask("task-1", "default", bmcv1alpha1.PowerOn),
	)

	c, w := setupTestContext("/bmc/tasks/default/task-1", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "task-1"},
	}

	HandleBMCTaskDetail(c, testLog)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleBMCTaskDetail_NotFound(t *testing.T) {
	kubeClient := newFakeKubeClient(
		newTestNamespace("default"),
	)

	c, w := setupTestContext("/bmc/tasks/default/nonexistent", kubeClient)
	c.Params = gin.Params{
		{Key: "namespace", Value: "default"},
		{Key: "name", Value: "nonexistent"},
	}

	HandleBMCTaskDetail(c, testLog)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
