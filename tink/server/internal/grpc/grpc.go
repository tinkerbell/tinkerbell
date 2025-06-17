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
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
	"github.com/tinkerbell/tinkerbell/pkg/journal"
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
	ReadAll(ctx context.Context, agentID string) ([]tinkerbell.Workflow, error)
	Read(ctx context.Context, workflowID, namespace string) (*tinkerbell.Workflow, error)
	Update(ctx context.Context, wf *tinkerbell.Workflow) error
}

// Handler is a server that implements a workflow API.
type Handler struct {
	Logger            logr.Logger
	BackendReadWriter BackendReadWriter
	NowFunc           func() time.Time
	AutoCapabilities  AutoCapabilities
	RetryOptions      []backoff.RetryOption

	proto.UnimplementedWorkflowServiceServer
}

type options struct {
	AutoCapabilities AutoCapabilities
}

func (h *Handler) GetAction(ctx context.Context, req *proto.ActionRequest) (*proto.ActionResponse, error) {
	operation := func() (*proto.ActionResponse, error) {
		opts := &options{
			AutoCapabilities: h.AutoCapabilities,
		}
		return h.doGetAction(ctx, req, opts)
	}
	if len(h.RetryOptions) == 0 {
		h.RetryOptions = []backoff.RetryOption{
			backoff.WithMaxElapsedTime(time.Minute),
			backoff.WithBackOff(backoff.NewConstantBackOff(time.Second)),
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

func (h *Handler) doGetAction(ctx context.Context, req *proto.ActionRequest, opts *options) (*proto.ActionResponse, error) {
	select {
	case <-ctx.Done():
		return nil, status.Error(codes.Unavailable, "server shutting down")
	default:
	}

	ctx = journal.New(ctx)
	log := h.Logger.WithValues("agent", req.GetAgentId())
	defer func() {
		log.V(1).Info("GetAction code flow journal", "journal", journal.Journal(ctx))
	}()
	if req.GetAgentId() == "" {
		journal.Log(ctx, "invalid Agent ID")
		return nil, status.Errorf(codes.InvalidArgument, "invalid Agent ID")
	}

	var hwRef *string
	// handle auto discovery
	if opts != nil && opts.AutoCapabilities.Discovery.Enabled {
		// Check if there is an existing Hardware Object.
		// If not, create one.
		hw, err := h.Discover(ctx, req.GetAgentId(), convert(req.GetAgentAttributes()))
		if err != nil {
			journal.Log(ctx, "error auto discovering Hardware", "error", err)
			log.Info("error auto discovering Hardware", "error", err)
			return nil, status.Errorf(codes.Internal, "error auto discovering Hardware: %v", err)
		}
		hwRef = &hw.Name
	}

	wfs, err := h.BackendReadWriter.ReadAll(ctx, req.GetAgentId())
	if err != nil {
		// TODO: This is where we handle auto capabilities
		journal.Log(ctx, "error getting Workflows", "error", err)
		return nil, errors.Join(ErrBackendRead, status.Errorf(codes.Internal, "error getting workflows: %v", err))
	}
	if len(wfs) == 0 {
		if opts != nil && opts.AutoCapabilities.Enrollment.Enabled {
			journal.Log(ctx, "auto enrollment triggered")
			return h.enroll(ctx, req.GetAgentId(), convert(req.GetAgentAttributes()), hwRef)
		}
		journal.Log(ctx, "no Workflow found")
		return nil, status.Error(codes.NotFound, "no Workflows found")
	}
	journal.Log(ctx, "found Workflows", "workflows", len(wfs))
	var wf tinkerbell.Workflow
	for _, w := range wfs {
		if len(w.Status.Tasks) == 0 {
			continue
		}
		// Don't serve Actions when in a tinkerbell.WorkflowStatePreparing state.
		// This is to prevent the Agent from starting Actions before Workflow boot options are performed.
		if w.Spec.BootOptions.BootMode != "" && w.Status.State == tinkerbell.WorkflowStatePreparing {
			journal.Log(ctx, "Workflow is in preparing state")
			return nil, status.Error(codes.FailedPrecondition, "Workflow is in preparing state")
		}
		if w.Status.State != tinkerbell.WorkflowStatePending && w.Status.State != tinkerbell.WorkflowStateRunning {
			journal.Log(ctx, "Workflow not in pending or running state")
			return nil, status.Error(codes.FailedPrecondition, "Workflow not in pending or running state")
		}
		wf = w
		journal.Log(ctx, "found Workflow", "workflow", wf.Name)
		break
	}
	if len(wf.Status.Tasks) == 0 {
		journal.Log(ctx, "no Tasks found in Workflow")
		return nil, status.Error(codes.NotFound, "no Tasks found in Workflow")
	}

	var task *tinkerbell.Task
	if isFirstAction(wf.Status.Tasks[0]) {
		task = &wf.Status.Tasks[0]
		journal.Log(ctx, "first Task, first Action")
	} else {
		for _, t := range wf.Status.Tasks {
			// check if all actions have been run successfully in this task.
			// if so continue to the next task.
			if isTaskSuccessful(t) {
				continue
			}
			task = &t
			journal.Log(ctx, "found Task", "taskID", t.ID)
			break
		}
		if task == nil {
			journal.Log(ctx, "no Tasks found")
			return nil, status.Error(codes.NotFound, "no Tasks found")
		}
	}

	if len(task.Actions) == 0 {
		journal.Log(ctx, "no Actions found")
		return nil, status.Error(codes.NotFound, "no Actions found")
	}

	var action *tinkerbell.Action
	if isFirstAction(*task) {
		if task.Actions[0].State != tinkerbell.WorkflowStatePending {
			journal.Log(ctx, "first Action not in pending state")
			return nil, status.Error(codes.FailedPrecondition, "first Action not in pending state")
		}
		journal.Log(ctx, "first Action")
		action = &task.Actions[0]
	} else {
		// This handles Actions after the first one
		// Get the current Action. If it is not in a success state, return error.
		if wf.Status.CurrentState.State != tinkerbell.WorkflowStateSuccess {
			journal.Log(ctx, "current Action not in success state")
			return nil, status.Error(codes.FailedPrecondition, "current Action not in success state")
		}
		// Get the next Action after the one defined in the current state.
		for idx, act := range task.Actions {
			if act.ID == wf.Status.CurrentState.ActionID {
				// if the action is the last one in the task, return error
				if idx == len(task.Actions)-1 {
					journal.Log(ctx, "last Action in task")
					// if the workflow has another task, then return the next action in that task.
					return nil, status.Error(codes.NotFound, "last Action in task")
				}
				action = &task.Actions[idx+1]
				journal.Log(ctx, "found Action", "actionID", action.ID)
				break
			}
		}
		if action == nil {
			journal.Log(ctx, "no Action found")
			return nil, status.Error(codes.NotFound, "no Action found")
		}
	}
	// This check goes after the action is found, so that multi task Workflows can be handled.
	if task.AgentID != req.GetAgentId() {
		journal.Log(ctx, "Task not assigned to Agent")
		return nil, status.Error(codes.NotFound, "Task not assigned to Agent")
	}

	// update the current state
	// populate the current state and then send the action to the client.
	wf.Status.CurrentState = &tinkerbell.CurrentState{
		AgentID:    req.GetAgentId(),
		TaskID:     task.ID,
		ActionID:   action.ID,
		State:      action.State,
		ActionName: action.Name,
		TaskName:   task.Name,
	}

	if err := h.BackendReadWriter.Update(ctx, &wf); err != nil {
		return nil, errors.Join(ErrBackendWrite, status.Errorf(codes.Internal, "error writing current state: %v", err))
	}

	ar := &proto.ActionResponse{
		WorkflowId: toPtr(wf.Namespace + "/" + wf.Name),
		TaskId:     toPtr(task.ID),
		AgentId:    toPtr(req.GetAgentId()),
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
	journal.Log(ctx, "sending Action", "action", ar)
	return ar, nil
}

// isFirstAction checks if the Task is at the first Action.
func isFirstAction(t tinkerbell.Task) bool {
	if len(t.Actions) == 0 {
		return false
	}
	if t.Actions[0].State == tinkerbell.WorkflowStatePending {
		return true
	}
	return false
}

func isTaskSuccessful(t tinkerbell.Task) bool {
	if len(t.Actions) == 0 {
		return true
	}
	if t.Actions[len(t.Actions)-1].State == tinkerbell.WorkflowStateSuccess {
		return true
	}
	return false
}

func (h *Handler) ReportActionStatus(ctx context.Context, req *proto.ActionStatusRequest) (*proto.ActionStatusResponse, error) {
	operation := func() (*proto.ActionStatusResponse, error) {
		return h.doReportActionStatus(ctx, req)
	}
	if len(h.RetryOptions) == 0 {
		h.RetryOptions = []backoff.RetryOption{
			backoff.WithMaxElapsedTime(time.Minute),
			backoff.WithBackOff(backoff.NewConstantBackOff(time.Second)),
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
			if action.ID == req.GetActionId() && task.AgentID == req.GetAgentId() {
				wf.Status.Tasks[ti].Actions[ai].State = tinkerbell.WorkflowState(req.GetActionState().String())
				wf.Status.Tasks[ti].Actions[ai].ExecutionStart = &metav1.Time{Time: req.GetExecutionStart().AsTime()}
				wf.Status.Tasks[ti].Actions[ai].ExecutionStop = &metav1.Time{Time: req.GetExecutionStop().AsTime()}
				wf.Status.Tasks[ti].Actions[ai].ExecutionDuration = req.GetExecutionDuration()
				wf.Status.Tasks[ti].Actions[ai].Message = req.GetMessage().GetMessage()

				// 4. Write the updated workflow
				if req.GetActionState() != proto.ActionStatusRequest_SUCCESS {
					wf.Status.State = wf.Status.Tasks[ti].Actions[ai].State
				}
				if len(wf.Status.Tasks) == ti+1 && len(task.Actions) == ai+1 && req.GetActionState() == proto.ActionStatusRequest_SUCCESS {
					// This is the last action in the last task
					wf.Status.State = tinkerbell.WorkflowStatePost
				}

				// update the status current state
				wf.Status.CurrentState = &tinkerbell.CurrentState{
					AgentID:    req.GetAgentId(),
					TaskID:     req.GetTaskId(),
					ActionID:   req.GetActionId(),
					State:      wf.Status.Tasks[ti].Actions[ai].State,
					ActionName: req.GetActionName(),
					TaskName:   wf.Status.Tasks[ti].Name,
				}
				if err := h.BackendReadWriter.Update(ctx, wf); err != nil {
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
