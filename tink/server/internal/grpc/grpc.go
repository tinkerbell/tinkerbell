package grpc

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	v1alpha1 "github.com/tinkerbell/tinkerbell/pkg/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	errInvalidWorkflowID = "invalid workflow id"
	errInvalidTaskName   = "invalid task name"
	errInvalidActionName = "invalid action name"
	errWritingToBackend  = "error writing to backend"
)

var (
	ErrBackendRead  = errors.New("error reading from backend")
	ErrBackendWrite = errors.New("error writing to backend")
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
	RetryOptions      []backoff.RetryOption

	proto.UnimplementedWorkflowServiceServer
}

func (h *Handler) GetAction(ctx context.Context, req *proto.ActionRequest) (*proto.ActionResponse, error) {
	operation := func() (*proto.ActionResponse, error) {
		return h.doGetAction(ctx, req)
	}
	if len(h.RetryOptions) == 0 {
		h.RetryOptions = []backoff.RetryOption{
			backoff.WithMaxElapsedTime(time.Minute * 5),
			backoff.WithBackOff(backoff.NewExponentialBackOff()),
		}
	}
	// We retry multiple times as we read-write to the Workflow Status and there can be caching and eventually consistent issues
	// that would cause the write to fail. A retry to get the latest Workflow resolves these types of issues.
	resp, err := backoff.Retry(ctx, operation, h.RetryOptions...)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (h *Handler) doGetAction(ctx context.Context, req *proto.ActionRequest) (*proto.ActionResponse, error) {
	select {
	case <-ctx.Done():
		return nil, status.Error(codes.Unavailable, "server shutting down")
	default:
	}

	log := h.Logger.WithValues("worker", req.GetWorkerId())
	if req.GetWorkerId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invalid worker id:")
	}

	wflows, err := h.BackendReadWriter.ReadAll(ctx, req.GetWorkerId())
	if err != nil {
		// TODO: This is where we handle auto capabilities
		return nil, errors.Join(ErrBackendRead, status.Errorf(codes.Internal, "error getting workflows: %v", err))
	}
	if len(wflows) == 0 {
		return nil, status.Error(codes.NotFound, "no workflows found")
	}
	wf := wflows[0]
	if len(wf.Status.Tasks) == 0 {
		return nil, status.Error(codes.NotFound, "no tasks found")
	}
	// Don't serve Actions when in a v1alpha1.WorkflowStatePreparing state.
	// This is to prevent the worker from starting Actions before Workflow boot options are performed.
	if wf.Spec.BootOptions.BootMode != "" && wf.Status.State == v1alpha1.WorkflowStatePreparing {
		return nil, status.Error(codes.FailedPrecondition, "workflow is in preparing state")
	}
	if wf.Status.State != v1alpha1.WorkflowStatePending && wf.Status.State != v1alpha1.WorkflowStateRunning {
		return nil, status.Error(codes.FailedPrecondition, "workflow not in pending or running state")
	}
	// only support workflows with a single task for now
	task := wf.Status.Tasks[0]
	if len(task.Actions) == 0 {
		return nil, status.Error(codes.NotFound, "no actions found")
	}
	if task.WorkerAddr != req.GetWorkerId() {
		return nil, status.Error(codes.NotFound, "task not assigned to worker")
	}
	var action *v1alpha1.Action
	// This is the first action handler
	if wf.Status.CurrentState == nil {
		if task.Actions[0].State != v1alpha1.WorkflowStatePending {
			return nil, status.Error(codes.FailedPrecondition, "first action not in pending state")
		}
		action = &task.Actions[0]
	} else {
		// This handles Actions after the first one
		// Get the current Action. If it is not in a success state, return error.
		if wf.Status.CurrentState.State != v1alpha1.WorkflowStateSuccess {
			return nil, status.Error(codes.FailedPrecondition, "current action not in success state")
		}
		// Get the next Action after the one defined in the current state.
		for idx, act := range task.Actions {
			if act.ID == wf.Status.CurrentState.ActionID {
				// if the action is the last one in the task, return error
				if idx == len(task.Actions)-1 {
					return nil, status.Error(codes.NotFound, "last action in task")
				}
				action = &task.Actions[idx+1]
				break
			}
		}
		if action == nil {
			return nil, status.Error(codes.NotFound, "no action found")
		}
	}

	// update the current state
	// populate the current state and then send the action to the client.
	wf.Status.CurrentState = &v1alpha1.CurrentState{
		WorkerID:   req.GetWorkerId(),
		TaskID:     task.ID,
		ActionID:   action.ID,
		State:      action.State,
		ActionName: action.Name,
	}

	if err := h.BackendReadWriter.Write(ctx, &wf); err != nil {
		return nil, errors.Join(ErrBackendWrite, status.Errorf(codes.Internal, "error writing current state: %v", err))
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
		Environment: func() []string {
			// add task environment variables to the action environment variables.
			joined := map[string]string{}
			maps.Copy(joined, task.Environment)
			maps.Copy(joined, action.Environment)
			resp := []string{}
			for k, v := range joined {
				resp = append(resp, fmt.Sprintf("%s=%s", k, v))
			}
			sort.Strings(resp)
			return resp
		}(),
		Pid: toPtr(action.Pid),
	}

	log.Info("sending action", "action", ar, "actionID", action.ID)
	return ar, nil
}

func (h *Handler) ReportActionStatus(ctx context.Context, req *proto.ActionStatusRequest) (*proto.ActionStatusResponse, error) {
	operation := func() (*proto.ActionStatusResponse, error) {
		return h.doReportActionStatus(ctx, req)
	}
	if len(h.RetryOptions) == 0 {
		h.RetryOptions = []backoff.RetryOption{
			backoff.WithMaxElapsedTime(time.Minute * 5),
			backoff.WithBackOff(backoff.NewExponentialBackOff()),
		}
	}
	// We retry multiple times as we read-write to the Workflow Status and there can be caching and eventually consistent issues
	// that would cause the write to fail and a retry to get the latest Workflow resolves these types of issues.
	resp, err := backoff.Retry(ctx, operation, h.RetryOptions...)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (h *Handler) doReportActionStatus(ctx context.Context, req *proto.ActionStatusRequest) (*proto.ActionStatusResponse, error) {
	// 1. Validate the request
	if req.GetWorkflowId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
	}
	if req.GetTaskId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, errInvalidTaskName)
	}
	if req.GetActionId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, errInvalidActionName)
	}
	// 2. Get the workflow
	namespace, name, _ := strings.Cut(req.GetWorkflowId(), "/")
	wf, err := h.BackendReadWriter.Read(ctx, name, namespace)
	if err != nil {
		return nil, errors.Join(ErrBackendRead, status.Errorf(codes.Internal, "error getting workflow: %v", err))
	}
	// 3. Find the Action in the workflow from the request
	for ti, task := range wf.Status.Tasks {
		for ai, action := range task.Actions {
			// action IDs match or this is the first action in a task
			if action.ID == req.GetActionId() && task.WorkerAddr == req.GetWorkerId() {
				wf.Status.Tasks[ti].Actions[ai].State = v1alpha1.WorkflowState(req.GetActionState().String())
				wf.Status.Tasks[ti].Actions[ai].ExecutionStart = &metav1.Time{Time: req.GetExecutionStart().AsTime()}
				wf.Status.Tasks[ti].Actions[ai].ExecutionStop = &metav1.Time{Time: req.GetExecutionStop().AsTime()}
				wf.Status.Tasks[ti].Actions[ai].ExecutionDuration = req.GetExecutionDuration()
				wf.Status.Tasks[ti].Actions[ai].Message = req.GetMessage().GetMessage()

				// 4. Write the updated workflow
				if req.GetActionState() != proto.StateType_SUCCESS {
					wf.Status.State = wf.Status.Tasks[ti].Actions[ai].State
				}
				if len(wf.Status.Tasks) == ti+1 && len(task.Actions) == ai+1 && req.GetActionState() == proto.StateType_SUCCESS {
					// This is the last action in the last task
					wf.Status.State = v1alpha1.WorkflowStatePost
				}

				// update the status current state
				wf.Status.CurrentState = &v1alpha1.CurrentState{
					WorkerID:   req.GetWorkerId(),
					TaskID:     req.GetTaskId(),
					ActionID:   req.GetActionId(),
					State:      wf.Status.Tasks[ti].Actions[ai].State,
					ActionName: req.GetActionName(),
				}
				if err := h.BackendReadWriter.Write(ctx, wf); err != nil {
					return nil, status.Errorf(codes.Internal, "error writing report status: %v", err)
				}
				return &proto.ActionStatusResponse{}, nil
			}
		}
	}

	return &proto.ActionStatusResponse{}, status.Error(codes.NotFound, "action not found")
}

func toPtr[T any](v T) *T {
	return &v
}
