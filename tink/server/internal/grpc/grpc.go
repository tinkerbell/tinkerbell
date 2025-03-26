package grpc

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/go-logr/logr"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	errInvalidWorkflowID     = "invalid workflow id"
	errInvalidTaskName       = "invalid task name"
	errInvalidActionName     = "invalid action name"
	errInvalidTaskReported   = "reported task name does not match the current action details"
	errInvalidActionReported = "reported action name does not match the current action details"
)

type BackendReadWriter interface {
	ReadAll(ctx context.Context, workerID string) ([]v1alpha1.Workflow, error)
	Read(ctx context.Context, workflowID, namespace string) (*v1alpha1.Workflow, error)
	Write(ctx context.Context, wf *v1alpha1.Workflow) error
}

// Handler is a server that implements a workflow API.
type Handler struct {
	Logger            logr.Logger
	BackendReadWriter BackendReadWriter
	NowFunc           func() time.Time
	AutoCapabilities  bool

	proto.UnimplementedWorkflowServiceServer
}

func (h *Handler) GetAction(ctx context.Context, req *proto.ActionRequest) (*proto.ActionResponse, error) {
	select {
	case <-ctx.Done():
		return nil, status.Error(codes.Unavailable, "server shutting down")
	default:
	}
	log := h.Logger.WithValues("worker", req.GetWorkerId())
	if req.GetWorkerId() == "" {
		// log.Info("invalid worker id")
		return nil, status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
	}

	wflows, err := h.getWorkflowsByID(ctx, req.GetWorkerId())
	if err != nil {
		// TODO: This is where we handle auto capabilities
		// log.Info("error getting workflows", "error", err)
		return nil, status.Errorf(codes.Internal, "error getting workflows: %v", err)
	}
	if len(wflows) == 0 {
		// log.Info("no workflows found", "worker", req.GetWorkerId())
		return nil, status.Error(codes.NotFound, "no workflows found")
	}
	wf := wflows[0]
	if len(wf.Status.Tasks) == 0 {
		// log.Info("no tasks found", "workflow", wf.Name)
		return nil, status.Error(codes.NotFound, "no tasks found")
	}
	// Don't serve Actions when in a v1alpha1.WorkflowStatePreparing state.
	// This is to prevent the worker from starting Actions before Workflow boot options are performed.
	if wf.Spec.BootOptions.BootMode != "" && wf.Status.State == v1alpha1.WorkflowStatePreparing {
		return nil, status.Error(codes.FailedPrecondition, "workflow is in preparing state")
	}
	if wf.Status.State != v1alpha1.WorkflowStatePending && wf.Status.State != v1alpha1.WorkflowStateRunning {
		// log.Info("workflow not in pending or running state", "workflowState", wf.Status.State)
		return nil, status.Error(codes.FailedPrecondition, "workflow not in pending or running state")
	}
	// only support workflows with a single task for now
	task := wf.Status.Tasks[0]
	if len(task.Actions) == 0 {
		// log.Info("no actions found", "workflow", wf.Name)
		return nil, status.Error(codes.NotFound, "no actions found")
	}
	if task.WorkerAddr != req.GetWorkerId() {
		// log.Info("task not assigned to worker", "taskWorkerAddr", task.WorkerAddr)
		return nil, status.Error(codes.NotFound, "task not assigned to worker")
	}
	var action *v1alpha1.Action
	// This is the first action handler
	if wf.Status.CurrentState == nil {
		if task.Actions[0].Status != v1alpha1.WorkflowStatePending {
			// log.Info("current action not in pending state", "actionStatus", task.Actions[0].Status)
			return nil, status.Error(codes.NotFound, "first action not in pending state")
		}
		action = &task.Actions[0]
	} else {
		// This handles Actions after the first one
		// Get the current Action. If it is not in a success state, return error.
		if wf.Status.CurrentState.State != v1alpha1.WorkflowStateSuccess {
			// log.Info("current action not in success state", "actionStatus", wf.Status.CurrentState.State)
			return nil, status.Error(codes.FailedPrecondition, "current action not in success state")
		}
		// Get the next Action after the one defined in the current state.
		for idx, act := range task.Actions {
			if act.ID == wf.Status.CurrentState.ActionID {
				// if the action is the last one in the task, return error
				if idx == len(task.Actions)-1 {
					// log.Info("last action in task", "actionID", action.ID)
					return nil, status.Error(codes.NotFound, "last action in task")
				}
				action = &task.Actions[idx+1]
				break
			}
		}
		if action == nil {
			// log.Info("no action found", "currentActionID", wf.Status.CurrentState.ActionID)
			return nil, status.Error(codes.NotFound, "no action found")
		}
	}

	// update the current state
	// populate the current state and then send the action to the client or the other way around?
	wf.Status.CurrentState = &v1alpha1.CurrentState{
		WorkerID:   req.GetWorkerId(),
		TaskID:     task.ID,
		ActionID:   action.ID,
		State:      action.Status,
		ActionName: action.Name,
	}
	if err := h.BackendReadWriter.Write(ctx, &wf); err != nil {
		// log.Error(err, "failed to write current state")
		return nil, status.Errorf(codes.Internal, "error writing current state: %v", err)
	}

	ar := &proto.ActionResponse{
		WorkflowId: toPtr(wf.Namespace + "/" + wf.Name),
		TaskId:     toPtr(task.ID),
		WorkerId:   toPtr(req.GetWorkerId()),
		ActionId:   toPtr(action.ID),
		Name:       toPtr(action.Name),
		Image:      toPtr(action.Image),
		Timeout:    toPtr(action.Timeout),
		Command:    action.Command,
		Volumes:    append(task.Volumes, action.Volumes...),
		Environment: func(env map[string]string) []string {
			resp := []string{}
			merged := map[string]string{}
			for k, v := range env {
				merged[k] = v
			}
			for k, v := range action.Environment {
				merged[k] = v
			}
			for k, v := range merged {
				resp = append(resp, fmt.Sprintf("%s=%s", k, v))
			}
			sort.Strings(resp)
			return resp
		}(task.Environment),
		Pid: toPtr(action.Pid),
	}

	log.Info("sending action", "action", ar, "actionID", action.ID)
	return ar, nil
}

func (h *Handler) ReportActionStatus(ctx context.Context, req *proto.ActionStatusRequest) (*proto.ActionStatusResponse, error) {
	// h.Logger.Info("reporting action status", "actionStatusRequest", req.String())
	// 1. Validate the request
	if req.GetWorkflowId() == "" {
		//	h.Logger.Info("invalid workflow id")
		return nil, status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
	}
	if req.GetTaskId() == "" {
		h.Logger.Info("invalid task name")
		return nil, status.Errorf(codes.InvalidArgument, errInvalidTaskName)
	}
	if req.GetActionId() == "" {
		h.Logger.Info("invalid action name")
		return nil, status.Errorf(codes.InvalidArgument, errInvalidActionName)
	}
	// 2. Get the workflow
	if err := retry.Do(func() error {
		wf, err := h.getWorkflowByName(ctx, req.GetWorkflowId())
		if err != nil {
			h.Logger.Error(err, "get workflow")
			return status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
		}
		// 3. Find the Action in the workflow from the request
		for ti, task := range wf.Status.Tasks {
			for ai, action := range task.Actions {
				// action IDs match or this is the first action in a task
				if action.ID == req.GetActionId() {
					wf.Status.Tasks[ti].Actions[ai].Status = v1alpha1.WorkflowState(req.GetActionState().String())
					wf.Status.Tasks[ti].Actions[ai].StartedAt = &metav1.Time{Time: req.GetCreatedAt().AsTime()}
					wf.Status.Tasks[ti].Actions[ai].DurationSeconds = req.GetExecutionSeconds()
					wf.Status.Tasks[ti].Actions[ai].Message = req.GetMessage().GetMessage()

					// 4. Write the updated workflow
					h.Logger.Info("updating workflow in Kubernetes", "workflow", wf)

					// overall success state is handled else where. where?
					if req.GetActionState() != proto.StateType_STATE_SUCCESS {
						wf.Status.State = v1alpha1.WorkflowState(req.GetActionState().String())
					}
					if len(wf.Status.Tasks) == ti+1 && len(task.Actions) == ai+1 && wf.Status.Tasks[ti].Actions[ai].Status == v1alpha1.WorkflowStateSuccess {
						// This is the last action in the last task
						wf.Status.State = v1alpha1.WorkflowStatePost
					}
					// update the status current state
					wf.Status.CurrentState = &v1alpha1.CurrentState{
						WorkerID:   req.GetWorkerId(),
						TaskID:     req.GetTaskId(),
						ActionID:   req.GetActionId(),
						State:      v1alpha1.WorkflowState(req.GetActionState().String()),
						ActionName: req.GetActionName(),
					}
					if err := h.BackendReadWriter.Write(ctx, wf); err != nil {
						h.Logger.Error(err, "failed to write action status")
						return status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
					}
					return nil
				}
				// h.Logger.Info("action doesnt match", "gotActionID", req.GetActionId(), "wantActionID", action.ID)
			}
		}
		return nil
	}, retry.Attempts(10), retry.Context(ctx)); err != nil {
		return &proto.ActionStatusResponse{}, err
	}

	return &proto.ActionStatusResponse{}, nil
}

func toPtr[T any](v T) *T {
	return &v
}

func (h *Handler) getWorkflowsByID(ctx context.Context, workerID string) ([]v1alpha1.Workflow, error) {
	stored, err := h.BackendReadWriter.ReadAll(ctx, workerID)
	if err != nil {
		return nil, err
	}

	wfs := []v1alpha1.Workflow{}
	for _, wf := range stored {
		// If the current assigned or running action is assigned to the requested worker, include it
		if wf.Status.Tasks[0].WorkerAddr == workerID && wf.Status.State != v1alpha1.WorkflowStatePost {
			wfs = append(wfs, wf)
		}
	}
	return wfs, nil
}

func (h *Handler) getWorkflowByName(ctx context.Context, workflowID string) (*v1alpha1.Workflow, error) {
	workflowNamespace, workflowName, _ := strings.Cut(workflowID, "/")
	wflw, err := h.BackendReadWriter.Read(ctx, workflowName, workflowNamespace)
	if err != nil {
		h.Logger.Error(err, "get client", "workflow", workflowID)
		return nil, err
	}
	return wflw, nil
}
