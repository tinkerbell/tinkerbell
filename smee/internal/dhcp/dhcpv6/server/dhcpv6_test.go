package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/insomniacslk/dhcp/dhcpv6"
)

type recordingHandler struct {
	handle func(context.Context)
}

func (r recordingHandler) Handle(ctx context.Context, _ net.PacketConn, _ net.Addr, _ dhcpv6.DHCPv6) {
	if r.handle != nil {
		r.handle(ctx)
	}
}

func TestEnqueueDropsWhenQueueFull(t *testing.T) {
	s := NewServer("", &net.UDPAddr{})
	s.HandlerWorkers = 1
	s.QueueSize = 1
	s.setDefaults()

	jobs := make(chan handlerJob, s.QueueSize)
	if !s.enqueue(context.Background(), jobs, handlerJob{}) {
		t.Fatal("expected first job to enqueue")
	}
	if s.enqueue(context.Background(), jobs, handlerJob{}) {
		t.Fatal("expected second job to drop when queue is full")
	}
}

func TestEnqueueLogsWhenQueueFull(t *testing.T) {
	sink := &spySink{}
	s := NewServer("", &net.UDPAddr{})
	s.SetLogger(logr.New(sink))
	s.HandlerWorkers = 1
	s.QueueSize = 1
	s.setDefaults()

	jobs := make(chan handlerJob, s.QueueSize)
	if !s.enqueue(context.Background(), jobs, handlerJob{}) {
		t.Fatal("expected first job to enqueue")
	}
	if s.enqueue(context.Background(), jobs, handlerJob{}) {
		t.Fatal("expected second job to drop when queue is full")
	}
	if !sink.called {
		t.Fatal("expected queue drop to be logged")
	}
}

func TestHandleAppliesHandlerTimeout(t *testing.T) {
	timeout := 25 * time.Millisecond
	done := make(chan error, 1)
	s := NewServer("", &net.UDPAddr{})
	s.HandlerTimeout = timeout
	job := handlerJob{
		handler: recordingHandler{
			handle: func(ctx context.Context) {
				<-ctx.Done()
				done <- ctx.Err()
			},
		},
	}

	start := time.Now()
	s.handle(context.Background(), job)

	if elapsed := time.Since(start); elapsed < timeout {
		t.Fatalf("handler returned before timeout: elapsed %s, timeout %s", elapsed, timeout)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("handler context error = %v, want %v", err, context.DeadlineExceeded)
		}
	default:
		t.Fatal("handler did not observe context cancellation")
	}
}

type spySink struct {
	called bool
}

func (s *spySink) Init(logr.RuntimeInfo)          {}
func (s *spySink) Enabled(int) bool               { return true }
func (s *spySink) Info(int, string, ...any)       { s.called = true }
func (s *spySink) Error(error, string, ...any)    { s.called = true }
func (s *spySink) WithValues(...any) logr.LogSink { return s }
func (s *spySink) WithName(string) logr.LogSink   { return s }
