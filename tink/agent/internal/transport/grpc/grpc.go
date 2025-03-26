package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/proto"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Config struct {
	Log              logr.Logger
	TinkServerClient proto.WorkflowServiceClient
	WorkerID         string
	RetryInterval    time.Duration
	Actions          chan spec.Action
	Attributes       *proto.WorkerAttributes
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

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		response, err := c.TinkServerClient.GetAction(ctx, &proto.ActionRequest{WorkerId: toPtr(c.WorkerID), WorkerAttributes: c.Attributes})
		if err != nil {
			// TODO(jacobweinstock): how to handle unrecoverable errors?
			// if the error is not found no need to log it.
			if status.Code(err) != codes.NotFound {
				log.Info("error getting action", "error", err)
			}
			// log.Info("error getting workflow contexts", "error", err)
			wait(ctx, c.RetryInterval)
			continue
		}
		log.Info("connected to server, ready to stream actions")

		as := spec.Action{
			TaskID:         response.GetTaskId(),
			ID:             response.GetActionId(),
			WorkerID:       response.GetWorkerId(),
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

		c.Actions <- as
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
	ar := &proto.ActionStatusRequest{
		WorkflowId:       &event.Action.WorkflowID,
		WorkerId:         &event.Action.WorkerID,
		TaskId:           &event.Action.TaskID,
		ActionId:         &event.Action.ID,
		ActionName:       &event.Action.Name,
		ActionState:      specToProto(event.State),
		ExecutionSeconds: toPtr(int64(event.Action.Duration.Seconds())),
		Message:          &proto.ActionMessage{Message: toPtr(event.Message)},
		CreatedAt:        timestamppb.New(event.Action.StartedAt),
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

func specToProto(inState spec.State) *proto.StateType {
	switch inState {
	case spec.StateRunning:
		return toPtr(proto.StateType_STATE_RUNNING)
	case spec.StateSuccess:
		return toPtr(proto.StateType_STATE_SUCCESS)
	case spec.StateFailure:
		return toPtr(proto.StateType_STATE_FAILED)
	case spec.StateTimeout:
		return toPtr(proto.StateType_STATE_TIMEOUT)
	default:
		return toPtr(proto.StateType_STATE_UNSPECIFIED)
	}
}

func toPtr[T any](v T) *T {
	return &v
}
