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
	"google.golang.org/protobuf/encoding/protojson"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"quamina.net/go/quamina"
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

type BackendReadUpdater interface {
	ReadAll(ctx context.Context, workerID string) ([]v1alpha1.Workflow, error)
	Read(ctx context.Context, workflowID, namespace string) (*v1alpha1.Workflow, error)
	Update(ctx context.Context, wf *v1alpha1.Workflow) error
}

type AutoCapReadCreator interface {
	ReadAllWorkflowRuleSets(ctx context.Context, namespace string) ([]v1alpha1.WorkflowRuleSet, error)
	Create(ctx context.Context, wf *v1alpha1.Workflow) error
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
	ReadCreator AutoCapReadCreator
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
	//log.Info("debugging", "attributes", req.GetWorkerAttributes(), "attributesString", req.GetWorkerAttributes().String())
	if req.GetWorkerId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invalid worker id:")
	}

	wflows, err := h.BackendReadWriter.ReadAll(ctx, req.GetWorkerId())
	if err != nil {
		return nil, errors.Join(ErrBackendRead, status.Errorf(codes.Internal, "error getting workflows: %v", err))
	}
	if len(wflows) == 0 {
		// TODO: This is where we handle auto capabilities
		if h.AutoCapabilities.Discovery.Enabled {
			// Check if there is an existing Hardware Object.
			// If not, create one.
		}
		if h.AutoCapabilities.Enrollment.Enabled {
			// TODO: fail here if an enrollment workflow already exists. h.BackendReadWriter.ReadAll returns non-terminal workflows
			// so a successful enrollment Workflow will not be returned as part of this call.
			log.Info("debugging", "startingAutoEnrollment", true)
			// Get all WorkflowRuleSets and check if there is a match to the WorkerID or the Attributes (if Attributes are provided by request)
			// using github.com/timbray/quamina
			// If there is a match, create a Workflow for the WorkerID.
			wrs, err := h.AutoCapabilities.Enrollment.ReadCreator.ReadAllWorkflowRuleSets(ctx, "tink-system")
			if err != nil {
				log.Info("debugging", "error getting workflow rules", true, "error", err)
				return nil, errors.Join(ErrBackendRead, status.Errorf(codes.Internal, "error getting workflow rules: %v", err))
			}

			for _, wr := range wrs {
				q, err := quamina.New()
				if err != nil {
					log.Info("debugging", "error preparing WorkflowRuleSet parser", true, "error", err)
					return nil, status.Errorf(codes.Internal, "error preparing WorkflowRuleSet parser: %v", err)
				}
				for idx, r := range wr.Spec.Rules {
					if err := q.AddPattern(fmt.Sprintf("pattern-%v", idx), r); err != nil {
						log.Info("debugging", "error with pattern in WorkflowRuleSet", true, "error", err)
						return nil, status.Errorf(codes.Internal, "error with pattern in WorkflowRuleSet: %v", err)
					}
				}

				var jsonEvent []byte
				if req.GetWorkerAttributes() != nil {
					jsonBytes, err := protojson.Marshal(req.GetWorkerAttributes())
					if err != nil {
						log.Info("debugging", "error marshalling attributes to json", true, "error", err)
						return nil, status.Errorf(codes.Internal, "error marshalling attributes to json: %v", err)
					}
					//log.Info("debugging", "jsonEvent", string(jsonBytes))
					jsonEvent = jsonBytes
				}
				matches, err := q.MatchesForEvent(jsonEvent)
				if err != nil {
					log.Info("debugging", "error matching pattern", true, "error", err)
					return nil, status.Errorf(codes.Internal, "error matching pattern: %v", err)
				}
				if len(matches) > 0 {
					// Create a Workflow for the WorkerID
					awf := &v1alpha1.Workflow{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("enrollment-%s", strings.ReplaceAll(req.GetWorkerId(), ":", "-")),
							Namespace: "tink-system",
						},
						Spec: wr.Spec.Workflow,
					}
					awf.Spec.HardwareMap["worker_id"] = req.GetWorkerId()
					if err := h.AutoCapabilities.Enrollment.ReadCreator.Create(ctx, awf); err != nil {
						log.Info("debugging", "error creating enrollment workflow", true, "error", err)
						return nil, errors.Join(ErrBackendWrite, status.Errorf(codes.Internal, "error creating enrollment workflow: %v", err))
					}
					log.Info("debugging", "enrollmentWorkflowCreated", true)
					return nil, backoff.Permanent(status.Error(codes.Unavailable, "enrollment workflow created, please try again"))
				}
			}
			// If there is no match, return an error.
			log.Info("debugging", "noWorkflowRuleSetMatch", true)
			return nil, status.Errorf(codes.NotFound, "no Workflow Rule Sets found or matched for worker %s", req.GetWorkerId())
		}
		log.Info("debugging", "noWorkflowsFound", true)
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

	if err := h.BackendReadWriter.Update(ctx, &wf); err != nil {
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
