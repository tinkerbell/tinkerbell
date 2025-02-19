package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type Config struct {
	Log              logr.Logger
	TinkServerClient proto.WorkflowServiceClient
	WorkerID         string
	RetryInterval    time.Duration
	Actions          chan spec.Action
}

// wait for either ctx.Done() or d duration to elapse.
func wait(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(d):
		return
	}
}

func (c *Config) Start(ctx context.Context) error {
	log := c.Log.WithValues("retry_interval", c.RetryInterval.String())
	var inProcessAction *proto.WorkflowAction
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		stream, err := c.TinkServerClient.GetWorkflowContexts(ctx, &proto.WorkflowContextRequest{WorkerId: c.WorkerID})
		if err != nil {
			// TODO(jacobweinstock): how to handle unrecoverable errors?
			log.Info("error getting workflow contexts", "error", err)
			wait(ctx, c.RetryInterval)
			continue
		}

		request, err := stream.Recv()
		if err != nil && !errors.Is(err, io.EOF) {
			status.Code(err)
			log.Info("error receiving workflow context", "error", err, "eType", fmt.Sprintf("%T", err), "code", status.Code(err))
			wait(ctx, c.RetryInterval)
			continue
		}

		if request == nil || request.GetCurrentWorker() != c.WorkerID || request.GetCurrentActionState() != proto.State_STATE_PENDING {
			wait(ctx, c.RetryInterval)
			continue
		}

		actions, err := c.TinkServerClient.GetWorkflowActions(ctx, &proto.WorkflowActionsRequest{WorkflowId: request.GetWorkflowId()})
		if err != nil {
			log.Info("error getting workflow actions", "error", err)
			wait(ctx, c.RetryInterval)
			continue
		}

		curAction := actions.GetActionList()[request.GetCurrentActionIndex()]
		if curAction.String() == inProcessAction.String() {
			// Generally, when the curAction == inProcessAction, it means the action is still in progress.
			// But if the action state is pending then it means that the inProcessAction should be reset
			// so that processing of the Action can be started.
			if request.GetCurrentActionState() == proto.State_STATE_PENDING {
				inProcessAction = &proto.WorkflowAction{}
			}
			wait(ctx, c.RetryInterval)
			continue
		}

		action := spec.Action{
			TaskName:       request.GetCurrentTask(),
			ID:             request.GetWorkflowId(),
			Name:           curAction.Name,
			Image:          curAction.Image,
			Env:            []spec.Env{},
			Volumes:        []spec.Volume{},
			Namespaces:     spec.Namespaces{},
			Retries:        0,
			TimeoutSeconds: int(curAction.Timeout),
		}
		if len(curAction.Command) > 0 {
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
			action.Args = curAction.Command
		}
		for _, v := range curAction.Volumes {
			action.Volumes = append(action.Volumes, spec.Volume(v))
		}
		for _, v := range curAction.GetEnvironment() {
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
			action.Env = append(action.Env, env)
		}
		action.Namespaces.PID = curAction.GetPid()

		c.Actions <- action
		inProcessAction = curAction
	}
}

func (c *Config) Read(ctx context.Context) (spec.Action, error) {
	select {
	case <-ctx.Done():
		return spec.Action{}, context.Canceled
	case v := <-c.Actions:
		return v, nil
	}
}

func (c *Config) Write(ctx context.Context, event spec.Event) error {
	ar := &proto.WorkflowActionStatus{
		WorkflowId:   event.Action.ID,
		TaskName:     event.Action.TaskName,
		ActionName:   event.Action.Name,
		ActionStatus: specToProto(event.State),
		Seconds:      0,
		Message:      event.Message,
		WorkerId:     c.WorkerID,
	}
	_, err := c.TinkServerClient.ReportActionStatus(ctx, ar)
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

	conn, err := grpc.NewClient(authority, creds, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	if err != nil {
		return nil, fmt.Errorf("dial tinkerbell server: %w", err)
	}

	return conn, nil
}

func specToProto(inState spec.State) proto.State {
	switch inState {
	case spec.StateRunning:
		return proto.State_STATE_RUNNING
	case spec.StateSuccess:
		return proto.State_STATE_SUCCESS
	case spec.StateFailure:
		return proto.State_STATE_FAILED
	case spec.StateTimeout:
		return proto.State_STATE_TIMEOUT
	}

	return proto.State(-1)
}
