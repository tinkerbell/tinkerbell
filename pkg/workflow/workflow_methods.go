package workflow

import (
	v1alpha1 "github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetStartTime returns the start time, for the first action of the first task.
func GetStartTime(w *v1alpha1.Workflow) *metav1.Time {
	if len(w.Status.Tasks) > 0 {
		if len(w.Status.Tasks[0].Actions) > 0 {
			return w.Status.Tasks[0].Actions[0].StartedAt
		}
	}
	return nil
}

type taskInfo struct {
	CurrentWorker        string
	CurrentTask          string
	CurrentTaskIndex     int
	CurrentAction        string
	CurrentActionIndex   int
	CurrentActionState   v1alpha1.WorkflowState
	TotalNumberOfActions int
}

func GetCurrentWorker(w *v1alpha1.Workflow) string {
	return getTaskActionInfo(w).CurrentWorker
}

func GetCurrentTask(w *v1alpha1.Workflow) string {
	return getTaskActionInfo(w).CurrentTask
}

func GetCurrentTaskIndex(w *v1alpha1.Workflow) int {
	return getTaskActionInfo(w).CurrentTaskIndex
}

func GetCurrentAction(w *v1alpha1.Workflow) string {
	return getTaskActionInfo(w).CurrentAction
}

func GetCurrentActionIndex(w *v1alpha1.Workflow) int {
	return getTaskActionInfo(w).CurrentActionIndex
}

func GetCurrentActionState(w *v1alpha1.Workflow) v1alpha1.WorkflowState {
	return getTaskActionInfo(w).CurrentActionState
}

func GetTotalNumberOfActions(w *v1alpha1.Workflow) int {
	return getTaskActionInfo(w).TotalNumberOfActions
}

// helper function for task info.
func getTaskActionInfo(w *v1alpha1.Workflow) taskInfo {
	var (
		found           bool
		taskIndex       = -1
		actionIndex     int
		actionTaskIndex int
		actionCount     int
	)
	for ti, task := range w.Status.Tasks {
		actionCount += len(task.Actions)
		if found {
			continue
		}
	INNER:
		for ai, action := range task.Actions {
			// Find the first non-successful action
			switch action.Status { //nolint:exhaustive // WorkflowStateWaiting is only used in Workflows not Actions.
			case v1alpha1.WorkflowStateSuccess:
				actionIndex++
				continue
			case v1alpha1.WorkflowStatePending, v1alpha1.WorkflowStateRunning, v1alpha1.WorkflowStateFailed, v1alpha1.WorkflowStateTimeout:
				taskIndex = ti
				actionTaskIndex = ai
				found = true
				break INNER
			}
		}
	}

	ti := taskInfo{
		TotalNumberOfActions: actionCount,
		CurrentActionIndex:   actionIndex,
	}
	if taskIndex >= 0 {
		ti.CurrentWorker = w.Status.Tasks[taskIndex].WorkerAddr
		ti.CurrentTask = w.Status.Tasks[taskIndex].Name
		ti.CurrentTaskIndex = taskIndex
	}
	if taskIndex >= 0 && actionIndex >= 0 {
		ti.CurrentAction = w.Status.Tasks[taskIndex].Actions[actionTaskIndex].Name
		ti.CurrentActionState = w.Status.Tasks[taskIndex].Actions[actionTaskIndex].Status
	}

	return ti
}
