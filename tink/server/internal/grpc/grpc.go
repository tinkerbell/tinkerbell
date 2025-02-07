package grpc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/api/v1alpha1/tinkerbell"
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
	ReadAll(ctx context.Context, workerID string) ([]tinkerbell.Workflow, error)
	Read(ctx context.Context, workflowID, namespace string) (*tinkerbell.Workflow, error)
	Write(ctx context.Context, wf *tinkerbell.Workflow) error
}

// Handler is a server that implements a workflow API.
type Handler struct {
	Logger            logr.Logger
	BackendReadWriter BackendReadWriter
	NowFunc           func() time.Time

	proto.UnimplementedWorkflowServiceServer
}

func getWorkflowContext(wf tinkerbell.Workflow) *proto.WorkflowContext {
	return &proto.WorkflowContext{
		WorkflowId:           wf.Namespace + "/" + wf.Name,
		CurrentWorker:        wf.GetCurrentWorker(),
		CurrentTask:          wf.GetCurrentTask(),
		CurrentAction:        wf.GetCurrentAction(),
		CurrentActionIndex:   int64(wf.GetCurrentActionIndex()),
		CurrentActionState:   proto.State(proto.State_value[string(wf.GetCurrentActionState())]),
		TotalNumberOfActions: int64(wf.GetTotalNumberOfActions()),
	}
}

func (s *Handler) getCurrentAssignedNonTerminalWorkflowsForWorker(ctx context.Context, workerID string) ([]tinkerbell.Workflow, error) {
	stored, err := s.BackendReadWriter.ReadAll(ctx, workerID)
	if err != nil {
		return nil, err
	}

	wfs := []tinkerbell.Workflow{}
	for _, wf := range stored {
		// If the current assigned or running action is assigned to the requested worker, include it
		if wf.Status.Tasks[wf.GetCurrentTaskIndex()].WorkerAddr == workerID {
			wfs = append(wfs, wf)
		}
	}
	return wfs, nil
}

func (s *Handler) getWorkflowByName(ctx context.Context, workflowID string) (*tinkerbell.Workflow, error) {
	workflowNamespace, workflowName, _ := strings.Cut(workflowID, "/")
	wflw, err := s.BackendReadWriter.Read(ctx, workflowName, workflowNamespace)
	if err != nil {
		s.Logger.Error(err, "get client", "workflow", workflowID)
		return nil, err
	}
	return wflw, nil
}

// The following APIs are used by the worker.

func (s *Handler) GetWorkflowContexts(req *proto.WorkflowContextRequest, stream proto.WorkflowService_GetWorkflowContextsServer) error {
	// if spec.Netboot is true, and allowPXE: false in the hardware then don't serve a workflow context
	// if spec.ToggleHardwareNetworkBooting is true, and any associated bmc jobs dont exists or have not completed successfully then don't serve a workflow context
	if req.GetWorkerId() == "" {
		return status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
	}
	wflows, err := s.getCurrentAssignedNonTerminalWorkflowsForWorker(stream.Context(), req.WorkerId)
	if err != nil {
		return err
	}
	for _, wf := range wflows {
		// Don't serve Actions when in a v1alpha1.WorkflowStatePreparing state.
		// This is to prevent the worker from starting Actions before Workflow boot options are performed.
		if wf.Spec.BootOptions.BootMode != "" && wf.Status.State == tinkerbell.WorkflowStatePreparing {
			continue
		}
		if err := stream.Send(getWorkflowContext(wf)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Handler) GetWorkflowActions(ctx context.Context, req *proto.WorkflowActionsRequest) (*proto.WorkflowActionList, error) {
	wfID := req.GetWorkflowId()
	if wfID == "" {
		return nil, status.Errorf(codes.NotFound, errInvalidWorkflowID)
	}
	wf, err := s.getWorkflowByName(ctx, wfID)
	if err != nil {
		return nil, err
	}
	return ActionListCRDToProto(wf), nil
}

func ActionListCRDToProto(wf *tinkerbell.Workflow) *proto.WorkflowActionList {
	if wf == nil {
		return nil
	}
	resp := &proto.WorkflowActionList{
		ActionList: []*proto.WorkflowAction{},
	}
	for _, task := range wf.Status.Tasks {
		for _, action := range task.Actions {
			resp.ActionList = append(resp.ActionList, &proto.WorkflowAction{
				TaskName: task.Name,
				Name:     action.Name,
				Image:    action.Image,
				Timeout:  action.Timeout,
				Command:  action.Command,
				WorkerId: task.WorkerAddr,
				Volumes:  append(task.Volumes, action.Volumes...),
				// TODO: (micahhausler) Dedupe task volume targets overridden in the action volumes?
				//   Also not sure how Docker handles nested mounts (ex: "/foo:/foo" and "/bar:/foo/bar")
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
				Pid: action.Pid,
			})
		}
	}
	return resp
}

// Modifies a workflow for a given workflowContext.
func (s *Handler) modifyWorkflowState(wf *tinkerbell.Workflow, wfContext *proto.WorkflowContext) error {
	if wf == nil {
		return errors.New("no workflow provided")
	}
	if wfContext == nil {
		return errors.New("no workflow context provided")
	}
	var (
		taskIndex   = -1
		actionIndex = -1
	)

	seenActions := 0
	for ti, task := range wf.Status.Tasks {
		if wfContext.CurrentTask == task.Name {
			taskIndex = ti
			for ai, action := range task.Actions {
				if action.Name == wfContext.CurrentAction && (wfContext.CurrentActionIndex == int64(ai) || wfContext.CurrentActionIndex == int64(seenActions)) {
					actionIndex = ai
					goto cont
				}
				seenActions++
			}
		}
		seenActions += len(task.Actions)
	}
cont:

	if taskIndex < 0 {
		return errors.New("task not found")
	}
	if actionIndex < 0 {
		return errors.New("action not found")
	}
	wf.Status.Tasks[taskIndex].Actions[actionIndex].Status = tinkerbell.WorkflowState(proto.State_name[int32(wfContext.CurrentActionState)])

	switch wfContext.CurrentActionState {
	case proto.State_STATE_RUNNING:
		// Workflow is running, so set the start time to now
		wf.Status.State = tinkerbell.WorkflowState(proto.State_name[int32(wfContext.CurrentActionState)])
		wf.Status.Tasks[taskIndex].Actions[actionIndex].StartedAt = func() *metav1.Time {
			t := metav1.NewTime(s.NowFunc())
			return &t
		}()
	case proto.State_STATE_FAILED, proto.State_STATE_TIMEOUT:
		// Handle terminal statuses by updating the workflow state and time
		wf.Status.State = tinkerbell.WorkflowState(proto.State_name[int32(wfContext.CurrentActionState)])
		if wf.Status.Tasks[taskIndex].Actions[actionIndex].StartedAt != nil {
			wf.Status.Tasks[taskIndex].Actions[actionIndex].Seconds = int64(s.NowFunc().Sub(wf.Status.Tasks[taskIndex].Actions[actionIndex].StartedAt.Time).Seconds())
		}
	case proto.State_STATE_SUCCESS:
		// Handle a success by marking the task as complete
		if wf.Status.Tasks[taskIndex].Actions[actionIndex].StartedAt != nil {
			wf.Status.Tasks[taskIndex].Actions[actionIndex].Seconds = int64(s.NowFunc().Sub(wf.Status.Tasks[taskIndex].Actions[actionIndex].StartedAt.Time).Seconds())
		}
		// Mark success on last action success
		if wfContext.CurrentActionIndex+1 == wfContext.TotalNumberOfActions {
			// Set the state to POST instead of Success to allow any post tasks to run.
			wf.Status.State = tinkerbell.WorkflowStatePost
		}
	case proto.State_STATE_PENDING:
		// This is probably a client bug?
		return errors.New("no update requested")
	}
	return nil
}

func validateActionStatusRequest(req *proto.WorkflowActionStatus) error {
	if req.GetWorkflowId() == "" {
		return status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
	}
	if req.GetTaskName() == "" {
		return status.Errorf(codes.InvalidArgument, errInvalidTaskName)
	}
	if req.GetActionName() == "" {
		return status.Errorf(codes.InvalidArgument, errInvalidActionName)
	}
	return nil
}

func getWorkflowContextForRequest(req *proto.WorkflowActionStatus, wf *tinkerbell.Workflow) *proto.WorkflowContext {
	wfContext := getWorkflowContext(*wf)
	wfContext.CurrentWorker = req.GetWorkerId()
	wfContext.CurrentTask = req.GetTaskName()
	wfContext.CurrentActionState = req.GetActionStatus()
	wfContext.CurrentActionIndex = int64(wf.GetCurrentActionIndex())
	return wfContext
}

func (s *Handler) ReportActionStatus(ctx context.Context, req *proto.WorkflowActionStatus) (*proto.Empty, error) {
	err := validateActionStatusRequest(req)
	if err != nil {
		return nil, err
	}
	wfID := req.GetWorkflowId()
	l := s.Logger.WithValues("actionName", req.GetActionName(), "status", req.GetActionStatus(), "workflowID", req.GetWorkflowId(), "taskName", req.GetTaskName(), "worker", req.WorkerId)

	wf, err := s.getWorkflowByName(ctx, wfID)
	if err != nil {
		l.Error(err, "get workflow")
		return nil, status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
	}
	if req.GetTaskName() != wf.GetCurrentTask() {
		return nil, status.Errorf(codes.InvalidArgument, errInvalidTaskReported)
	}
	if req.GetActionName() != wf.GetCurrentAction() {
		return nil, status.Errorf(codes.InvalidArgument, errInvalidActionReported)
	}

	wfContext := getWorkflowContextForRequest(req, wf)
	err = s.modifyWorkflowState(wf, wfContext)
	if err != nil {
		l.Error(err, "modify workflow state")
		return nil, status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
	}
	l.Info("updating workflow in Kubernetes")
	if err := s.BackendReadWriter.Write(ctx, wf); err != nil {
		l.Error(err, "writing workflow")
		return nil, status.Errorf(codes.InvalidArgument, errInvalidWorkflowID)
	}

	return &proto.Empty{}, nil
}
