package nats

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/go-logr/logr"
	"github.com/nats-io/nats.go"
	"github.com/tinkerbell/tinkerbell/agent/internal/spec"
	"gopkg.in/yaml.v3"
)

type Config struct {
	StreamName     string
	EventsSubject  string
	ActionsSubject string
	IPPort         netip.AddrPort
	Log            logr.Logger
	AgentID        string
	Actions        chan spec.Action
	conn           *nats.Conn
	cancel         chan bool
}

func (c *Config) Start(ctx context.Context) error {
	c.cancel = make(chan bool)
	opts := []nats.Option{
		nats.Name(c.AgentID),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
	}

	nc, err := nats.Connect(fmt.Sprintf("nats://%v", c.IPPort.String()), opts...)
	if err != nil {
		return err
	}
	defer nc.Close()
	c.conn = nc

	// create a retry configuration
	rc := &retry.Config{}
	ropts := []retry.Option{
		retry.Attempts(0),
		retry.MaxDelay(30 * time.Second),
		retry.MaxJitter(time.Second * 10),
	}
	for _, opt := range ropts {
		opt(rc)
	}

	// wait until connected
	for !nc.IsConnected() {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(retry.RandomDelay(0, errors.New(""), rc)):
			c.Log.Info("waiting for NATS connection", "server", c.IPPort.String())
			continue
		}
	}

	c.Log.Info("connected to NATS", "status", nc.Status().String())

	base := fmt.Sprintf("%v.%v", c.StreamName, c.AgentID)
	subj := fmt.Sprintf("%v.%v", base, c.ActionsSubject)
	sub, err := nc.SubscribeSync(subj)
	if err != nil {
		return err
	}
	defer func() {
		_ = sub.Unsubscribe()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		msg, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			continue
		}

		actions := []spec.Action{}
		if err := yaml.Unmarshal(msg.Data, &actions); err != nil {
			continue
		}
		for _, action := range actions {
			select {
			case <-ctx.Done():
			case <-c.cancel:
			case c.Actions <- action:
				continue
			}
			break
		}
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

func (c *Config) Write(_ context.Context, event spec.Event) error {
	if event.State == spec.StateFailure || event.State == spec.StateTimeout {
		c.Actions = make(chan spec.Action)
		c.cancel <- true
	}
	return c.conn.PublishMsg(&nats.Msg{
		Subject: fmt.Sprintf("%v.%v.%v", c.StreamName, c.AgentID, c.EventsSubject),
		Data:    []byte(event.String()),
	})
}
