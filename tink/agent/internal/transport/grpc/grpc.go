package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	epb "google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	gbackoff "google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Config struct {
	Log              logr.Logger
	TinkServerClient proto.WorkflowServiceClient
	AgentID          string
	Actions          chan spec.Action
	Attributes       *data.AgentAttributes
}

func (c *Config) Read(ctx context.Context) (spec.Action, error) {
	return c.doRead(ctx)
}

func (c *Config) doRead(ctx context.Context) (spec.Action, error) {
	response, err := c.TinkServerClient.GetAction(ctx, &proto.ActionRequest{AgentId: toPtr(c.AgentID), AgentAttributes: ToProto(c.Attributes)})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return spec.Action{}, &NoWorkflowError{}
		}

		e := fmt.Errorf("error getting action: %w", err)
		if isPFVNoActionsAvailable(err) {
			e = newNoActionsError("no actions available", err)
		}

		return spec.Action{}, e
	}

	as := spec.Action{
		TaskID:         response.GetTaskId(),
		ID:             response.GetActionId(),
		AgentID:        response.GetAgentId(),
		WorkflowID:     response.GetWorkflowId(),
		Name:           response.GetName(),
		Image:          response.GetImage(),
		Env:            []spec.Env{},
		Volumes:        []spec.Volume{},
		Namespaces:     spec.Namespaces{},
		Retries:        0,
		TimeoutSeconds: int(response.GetTimeout()),
	}
	if len(response.GetCommand()) > 0 {
		// action.Cmd is the entrypoint in a container.
		// action.Args are the arguments to the entrypoint.

		// This would allow the Action to override the entrypoint.
		// This is useful as the current v1alpha1 spec doesn't have a way to override the entrypoint.
		// But this changes the behavior of using `command` in an Action that is not clear and is not backward compatible.
		// This is commented out until we have a clear way to handle this.
		/*
			action.Cmd = curAction.Command[0]
			if len(curAction.Command) > 1 {
				action.Args = curAction.Command[1:]
			}
		*/
		as.Args = response.GetCommand()
	}
	for _, v := range response.GetVolumes() {
		as.Volumes = append(as.Volumes, spec.Volume(v))
	}
	for _, v := range response.GetEnvironment() {
		kv := strings.SplitN(v, "=", 2)
		env := spec.Env{}
		switch len(kv) {
		case 1:
			env = spec.Env{
				Key:   kv[0],
				Value: "",
			}
		case 2:
			env = spec.Env{
				Key:   kv[0],
				Value: kv[1],
			}
		}
		as.Env = append(as.Env, env)
	}
	as.Namespaces.PID = response.GetPid()

	return as, nil
}

func (c *Config) Write(ctx context.Context, event spec.Event) error {
	return c.doWrite(ctx, event)
}

func (c *Config) doWrite(ctx context.Context, event spec.Event) error {
	ar := &proto.ActionStatusRequest{
		WorkflowId:        &event.Action.WorkflowID,
		AgentId:           &event.Action.AgentID,
		TaskId:            &event.Action.TaskID,
		ActionId:          &event.Action.ID,
		ActionName:        &event.Action.Name,
		ActionState:       specToProto(event.State),
		ExecutionStart:    timestamppb.New(event.Action.ExecutionStart),
		ExecutionStop:     timestamppb.New(event.Action.ExecutionStop),
		ExecutionDuration: toPtr(event.Action.ExecutionDuration),
		Message:           &proto.ActionMessage{Message: toPtr(event.Message)},
	}
	_, err := c.TinkServerClient.ReportActionStatus(ctx, ar)
	if status.Code(err) == codes.Internal {
		return backoff.Permanent(err)
	}
	if err != nil {
		return fmt.Errorf("error reporting action: %v: %w", ar, err)
	}

	return nil
}

func NewClientConn(authority string, tlsEnabled bool, tlsInsecure bool) (*grpc.ClientConn, error) {
	if authority == "" {
		return nil, errors.New("the Tinkerbell server address is required, none provided")
	}
	var creds grpc.DialOption
	if tlsEnabled { // #nosec G402
		creds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: tlsInsecure}))
	} else {
		creds = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.NewClient(authority, creds, grpc.WithStatsHandler(otelgrpc.NewClientHandler()), grpc.WithConnectParams(grpc.ConnectParams{Backoff: gbackoff.DefaultConfig}))
	if err != nil {
		return nil, fmt.Errorf("dial tinkerbell server: %w", err)
	}

	return conn, nil
}

func specToProto(inState spec.State) *proto.ActionStatusRequest_StateType {
	switch inState {
	case spec.StateRunning:
		return toPtr(proto.ActionStatusRequest_RUNNING)
	case spec.StateSuccess:
		return toPtr(proto.ActionStatusRequest_SUCCESS)
	case spec.StateFailure:
		return toPtr(proto.ActionStatusRequest_FAILED)
	case spec.StateTimeout:
		return toPtr(proto.ActionStatusRequest_TIMEOUT)
	default:
		return toPtr(proto.ActionStatusRequest_UNSPECIFIED)
	}
}

func isPFVNoActionsAvailable(err error) bool {
	st := status.Convert(err)
	if st.Details() != nil {
		for _, detail := range st.Details() {
			if violation, ok := detail.(*epb.PreconditionFailure); ok {
				for _, v := range violation.GetViolations() {
					if v.GetType() == proto.PreconditionFailureViolation_PRECONDITION_FAILURE_VIOLATION_NO_ACTION_AVAILABLE.String() {
						return true
					}
				}
			}
		}
	}
	if st.Code() == codes.FailedPrecondition {
		return true
	}
	return false
}

func toPtr[T any](v T) *T {
	return &v
}

// noActionsError represents a structured error with additional context.
type noActionsError struct {
	Message string
	Inner   error
}

// Error implements the error interface.
func (e *noActionsError) Error() string {
	if e.Inner != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Inner)
	}
	return e.Message
}

// Unwrap implements the Wrapper interface.
func (e *noActionsError) Unwrap() error {
	return e.Inner
}

// As implements type assertion support.
func (e *noActionsError) As(target interface{}) bool {
	if target == nil {
		return false
	}
	_, ok := target.(**noActionsError)
	return ok
}

// NoAction implements the NoAction interface in the agent package.
func (e *noActionsError) NoAction() bool {
	return true
}

// newNoActionsError creates a new instance of NoActionsError.
func newNoActionsError(message string, inner error) *noActionsError {
	return &noActionsError{
		Message: message,
		Inner:   inner,
	}
}
