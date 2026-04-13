package agent

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

type mock struct{}

func (m *mock) Read(_ context.Context) (spec.Action, error) {
	return spec.Action{}, nil
}

func (m *mock) Execute(_ context.Context, _ spec.Action) error {
	return nil
}

func (m *mock) Write(_ context.Context, _ spec.Event) error {
	return nil
}

func TestRun(_ *testing.T) {
	c := &Config{TransportReader: &mock{}, RuntimeExecutor: &mock{}, TransportWriter: &mock{}}
	ctx, cancel := context.WithCancel(context.Background())
	go c.Run(ctx, logr.Discard())
	<-time.After(1 * time.Second)
	cancel()
}

// oneActionReader returns one action then blocks until ctx is cancelled.
type oneActionReader struct {
	sent atomic.Bool
}

func (r *oneActionReader) Read(ctx context.Context) (spec.Action, error) {
	if r.sent.CompareAndSwap(false, true) {
		return spec.Action{ID: "a1", TimeoutSeconds: 5}, nil
	}
	<-ctx.Done()
	return spec.Action{}, ctx.Err()
}

// failNWriter fails the first N completion writes then succeeds and cancels the context.
type failNWriter struct {
	failCount int
	calls     atomic.Int32
	cancel    context.CancelFunc
}

func (w *failNWriter) Write(_ context.Context, _ spec.Event) error {
	n := int(w.calls.Add(1))
	// The first write is the "running" event, always succeed.
	// Subsequent writes are completion reports; fail the first failCount of those.
	if n == 1 {
		return nil
	}
	completionCall := n - 1
	if completionCall <= w.failCount {
		return fmt.Errorf("transient error %d", completionCall)
	}
	// Successful completion write — cancel so Run exits promptly.
	if w.cancel != nil {
		w.cancel()
	}
	return nil
}

func TestRunRetriesWriteFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	writer := &failNWriter{failCount: 3, cancel: cancel}
	c := &Config{
		TransportReader: &oneActionReader{},
		RuntimeExecutor: &mock{},
		TransportWriter: writer,
		Backoff: &backoff.ExponentialBackOff{
			InitialInterval:     time.Millisecond,
			RandomizationFactor: 0,
			Multiplier:          1,
			MaxInterval:         time.Millisecond,
		},
	}

	done := make(chan struct{})
	go func() {
		c.Run(ctx, logr.Discard())
		close(done)
	}()

	select {
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not complete after retried writes")
	case <-done:
	}

	// 1 "running" write + (failCount) failed completion writes + 1 successful completion write
	want := int32(1 + writer.failCount + 1)
	got := writer.calls.Load()
	if got != want {
		t.Errorf("expected %d Write calls, got %d", want, got)
	}
}

// permanentWriter always returns a backoff.Permanent error on completion writes.
type permanentWriter struct {
	calls atomic.Int32
}

func (w *permanentWriter) Write(_ context.Context, _ spec.Event) error {
	n := int(w.calls.Add(1))
	if n == 1 {
		return nil // "running" event succeeds
	}
	return backoff.Permanent(fmt.Errorf("unrecoverable"))
}

func TestRunHaltsOnPermanentWriteError(t *testing.T) {
	writer := &permanentWriter{}
	c := &Config{
		TransportReader: &oneActionReader{},
		RuntimeExecutor: &mock{},
		TransportWriter: writer,
		Backoff: &backoff.ExponentialBackOff{
			InitialInterval:     time.Millisecond,
			RandomizationFactor: 0,
			Multiplier:          1,
			MaxInterval:         time.Millisecond,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		c.Run(ctx, logr.Discard())
		close(done)
	}()

	select {
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not halt after permanent error")
	case <-done:
	}

	// 1 "running" write + 1 permanent-error completion write = 2 total
	got := writer.calls.Load()
	if got != 2 {
		t.Errorf("expected 2 Write calls, got %d", got)
	}
}
