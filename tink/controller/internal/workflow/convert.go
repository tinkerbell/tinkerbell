package workflow

import (
	"github.com/oklog/ulid/v2"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
)

func YAMLToStatus(wf *Workflow) *v1alpha1.WorkflowStatus {
	if wf == nil {
		return nil
	}
	tasks := []v1alpha1.Task{}
	agentID := ""
	for _, task := range wf.Tasks {
		actions := []v1alpha1.Action{}
		for _, action := range task.Actions {
			actions = append(actions, v1alpha1.Action{
				ID:          ulid.Make().String(),
				Name:        action.Name,
				Image:       action.Image,
				Timeout:     action.Timeout,
				Command:     action.Command,
				Volumes:     action.Volumes,
				State:       v1alpha1.WorkflowState(proto.ActionStatusRequest_PENDING.String()),
				Environment: action.Environment,
				Pid:         action.Pid,
			})
		}
		tasks = append(tasks, v1alpha1.Task{
			Name:        task.Name,
			AgentID:     task.WorkerAddr,
			ID:          ulid.Make().String(),
			Volumes:     task.Volumes,
			Environment: task.Environment,
			Actions:     actions,
		})
		// only use the first Task's agentID. At the moment only support single Task Workflows.
		if agentID == "" {
			agentID = task.WorkerAddr
		}
	}
	return &v1alpha1.WorkflowStatus{
		GlobalTimeout: int64(wf.GlobalTimeout),
		Tasks:         tasks,
		AgentID:       agentID,
	}
}
