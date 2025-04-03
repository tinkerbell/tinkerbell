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
	errBackendRead  = errors.New("error reading from backend")
	errBackendWrite = errors.New("error writing to backend")
)

type BackendReadWriter interface {
	ReadAll(ctx context.Context, agentID string) ([]v1alpha1.Workflow, error)
	Read(ctx context.Context, workflowID, namespace string) (*v1alpha1.Workflow, error)
	Update(ctx context.Context, wf *v1alpha1.Workflow) error
}

type AutoReadCreator interface {
	WorkflowRuleSetReader
	WorkflowCreator
}

type WorkflowRuleSetReader interface {
	ReadWorkflowRuleSets(ctx context.Context) ([]v1alpha1.WorkflowRuleSet, error)
}

type WorkflowCreator interface {
	CreateWorkflow(ctx context.Context, wf *v1alpha1.Workflow) error
}

// Handler is a server that implements a workflow API.
type Handler struct {
	Logger            logr.Logger
	BackendReadWriter BackendReadUpdater
	NowFunc           func() time.Time
	AutoCapabilities  AutoCapabilities
	RetryOptions      []backoff.RetryOption

	proto.UnimplementedWorkflowServiceServer
}

type AutoCapabilities struct {
	Enrollment AutoEnrollment
	Discovery  AutoDiscovery
}

// AutoEnrollmentE is a struct that contains the auto enrollment configuration.
// Auto Enrollment is defined as automatically running a Workflow for an Agent that
// does not have a Workflow assigned to it. The Agent may or may not have a Hardware
// Object defined.
type AutoEnrollment struct {
	Enabled     bool
	ReadCreator AutoReadCreator
}

// AutoDiscovery is a struct that contains the auto discovery configuration.
// Auto Discovery is defined as automatically creating a Hardware Object for an
// Agent that does not have a Workflow or a Hardware Object assigned to it.
// The Namespace defines the namespace to use when creating the Hardware Object.
// An empty namespace will cause all Hardware Objects to be created in the same
// namespace as the Tink Server.
type AutoDiscovery struct {
	Enabled   bool
	Namespace string
}

func (h *Handler) GetAction(ctx context.Context, req *proto.ActionRequest) (*proto.ActionResponse, error) {
	operation := func() (*proto.ActionResponse, error) {
		return h.doGetAction(ctx, req)
	}
	if len(h.RetryOptions) == 0 {
		h.RetryOptions = []backoff.RetryOption{
			backoff.WithMaxElapsedTime(time.Minute * 1),
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

	ctx = journal.New(ctx)
	log := h.Logger.WithValues("agent", req.GetWorkerId())
	defer func() {
		log.V(1).Info("GetAction code flow journal", "journal", journal.Journal(ctx))
	}()
	if req.GetWorkerId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invalid Agent ID")
	}

	wfs, err := h.BackendReadWriter.ReadAll(ctx, req.GetWorkerId())
	if err != nil {
		return nil, errors.Join(errBackendRead, status.Errorf(codes.Internal, "error getting workflows: %v", err))
	}
	nonTerminatedWflows := func() []v1alpha1.Workflow {
		wfs := []v1alpha1.Workflow{}
		for _, wf := range wflows {
			if wf.Status.State != v1alpha1.WorkflowStateRunning && wf.Status.State != v1alpha1.WorkflowStatePending {
				return wfs
			}
			wfs = append(wfs, wf)
		}
		return wfs
	}()
	if len(nonTerminatedWflows) == 0 {
		// TODO: This is where we handle auto capabilities
		if h.AutoCapabilities.Discovery.Enabled {
			// Check if there is an existing Hardware Object.
			// If not, create one.
		}
		if h.AutoCapabilities.Enrollment.Enabled {
			wfns := func() wflowNamespace {
				wfs := wflowNamespace{}
				for _, wf := range wflows {
					wfs[wf.Namespace] = wf.Namespace
				}
				return wfs
			}()
			return h.enroll(ctx, req.GetWorkerId(), req.GetWorkerAttributes(), wfns)
		}
		log.Info("debugging", "noWorkflowsFound", true)
		return nil, status.Error(codes.NotFound, "no workflows found")
	}
	var wf v1alpha1.Workflow
	for _, w := range wfs {
		if len(w.Status.Tasks) == 0 {
			continue
		}
		// Don't serve Actions when in a v1alpha1.WorkflowStatePreparing state.
		// This is to prevent the Agent from starting Actions before Workflow boot options are performed.
		if w.Spec.BootOptions.BootMode != "" && w.Status.State == v1alpha1.WorkflowStatePreparing {
			return nil, status.Error(codes.FailedPrecondition, "Workflow is in preparing state")
		}
		if w.Status.State != v1alpha1.WorkflowStatePending && w.Status.State != v1alpha1.WorkflowStateRunning {
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

	var task *v1alpha1.Task
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

	var action *v1alpha1.Action
	if isFirstAction(*task) {
		if task.Actions[0].State != v1alpha1.WorkflowStatePending {
			journal.Log(ctx, "first Action not in pending state")
			return nil, status.Error(codes.FailedPrecondition, "first Action not in pending state")
		}
		journal.Log(ctx, "first Action")
		action = &task.Actions[0]
	} else {
		// This handles Actions after the first one
		// Get the current Action. If it is not in a success state, return error.
		if wf.Status.CurrentState.State != v1alpha1.WorkflowStateSuccess {
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
	if task.WorkerAddr != req.GetWorkerId() {
		journal.Log(ctx, "Task not assigned to Agent")
		return nil, status.Error(codes.NotFound, "Task not assigned to Agent")
	}

	// update the current state
	// populate the current state and then send the action to the client.
	wf.Status.CurrentState = &v1alpha1.CurrentState{
		WorkerID:   req.GetWorkerId(),
		TaskID:     task.ID,
		ActionID:   action.ID,
		State:      action.State,
		ActionName: action.Name,
		TaskName:   task.Name,
	}

	if err := h.BackendReadWriter.Update(ctx, &wf); err != nil {
		return nil, errors.Join(errBackendWrite, status.Errorf(codes.Internal, "error writing current state: %v", err))
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
	journal.Log(ctx, "sending Action", "action", ar)
	return ar, nil
}

// isFirstAction checks if the Task is at the first Action.
func isFirstAction(t v1alpha1.Task) bool {
	if len(t.Actions) == 0 {
		return false
	}
	if t.Actions[0].State == v1alpha1.WorkflowStatePending {
		return true
	}
	return false
}

func isTaskSuccessful(t v1alpha1.Task) bool {
	if len(t.Actions) == 0 {
		return true
	}
	if t.Actions[len(t.Actions)-1].State == v1alpha1.WorkflowStateSuccess {
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
		return nil, errors.Join(errBackendRead, status.Errorf(codes.Internal, "error getting workflow: %v", err))
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
				if req.GetActionState() != proto.ActionStatusRequest_SUCCESS {
					wf.Status.State = wf.Status.Tasks[ti].Actions[ai].State
				}
				if len(wf.Status.Tasks) == ti+1 && len(task.Actions) == ai+1 && req.GetActionState() == proto.ActionStatusRequest_SUCCESS {
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
