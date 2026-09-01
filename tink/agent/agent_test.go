package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

const testWaitTimeout = 5 * time.Second

type mock struct{}

type readerFunc func(context.Context) (spec.Action, error)

func (f readerFunc) Read(ctx context.Context) (spec.Action, error) {
	return f(ctx)
}

type executorFunc func(context.Context, spec.Action) error

func (f executorFunc) Execute(ctx context.Context, action spec.Action) error {
	return f(ctx, action)
}

type writerFunc func(context.Context, spec.Event) error

func (f writerFunc) Write(ctx context.Context, event spec.Event) error {
	return f(ctx, event)
}

type testNoActionError struct{}

func (testNoActionError) Error() string  { return "no action" }
func (testNoActionError) NoAction() bool { return true }

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

func TestRunSkipsRuntimeWhenRunningReportIsRejected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repolled := make(chan struct{})
	var repollOnce sync.Once
	var reads atomic.Int32
	reader := readerFunc(func(context.Context) (spec.Action, error) {
		if reads.Add(1) == 1 {
			return spec.Action{ID: "action-1", TimeoutSeconds: 5}, nil
		}
		repollOnce.Do(func() { close(repolled) })
		return spec.Action{}, testNoActionError{}
	})

	var executions atomic.Int32
	executor := executorFunc(func(context.Context, spec.Action) error {
		executions.Add(1)
		return nil
	})

	var writes atomic.Int32
	writer := writerFunc(func(_ context.Context, event spec.Event) error {
		writes.Add(1)
		if event.State != spec.StateRunning {
			t.Errorf("unexpected completion report after RUNNING was rejected: %s", event.State)
		}
		return backoff.Permanent(fmt.Errorf("workflow is failed"))
	})

	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: executor,
		TransportWriter: writer,
		Backoff:         testBackoff(),
	}
	done := make(chan struct{})
	go func() {
		c.Run(ctx, logr.Discard())
		close(done)
	}()

	select {
	case <-repolled:
	case <-time.After(testWaitTimeout):
		t.Fatal("agent did not return to polling after the RUNNING report was rejected")
	}
	if got := executions.Load(); got != 0 {
		t.Fatalf("runtime executed %d times after the RUNNING report was rejected", got)
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("expected one rejected RUNNING report, got %d writes", got)
	}
	select {
	case <-done:
		t.Fatal("agent exited instead of remaining alive and polling")
	default:
	}

	cancel()
	select {
	case <-done:
	case <-time.After(testWaitTimeout):
		t.Fatal("agent did not exit after context cancellation")
	}
}

func TestRunAcknowledgedCompletionReturnsToPolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repolled := make(chan struct{})
	var repollOnce sync.Once
	var reads atomic.Int32
	reader := readerFunc(func(context.Context) (spec.Action, error) {
		if reads.Add(1) == 1 {
			return spec.Action{ID: "action-1", TimeoutSeconds: 5}, nil
		}
		repollOnce.Do(func() { close(repolled) })
		return spec.Action{}, testNoActionError{}
	})

	runtimeStarted := make(chan struct{})
	releaseRuntime := make(chan struct{})
	var executions atomic.Int32
	executor := executorFunc(func(ctx context.Context, _ spec.Action) error {
		executions.Add(1)
		close(runtimeStarted)
		select {
		case <-releaseRuntime:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	completionAcknowledged := make(chan struct{})
	var completionOnce sync.Once
	writer := writerFunc(func(_ context.Context, event spec.Event) error {
		if event.State != spec.StateRunning {
			completionOnce.Do(func() { close(completionAcknowledged) })
		}
		return nil
	})

	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: executor,
		TransportWriter: writer,
		Backoff:         testBackoff(),
	}
	done := make(chan struct{})
	go func() {
		c.Run(ctx, logr.Discard())
		close(done)
	}()

	select {
	case <-runtimeStarted:
	case <-time.After(testWaitTimeout):
		t.Fatal("runtime did not start after the RUNNING report was accepted")
	}
	close(releaseRuntime)
	select {
	case <-completionAcknowledged:
	case <-time.After(testWaitTimeout):
		t.Fatal("agent did not report the in-flight action completion")
	}
	select {
	case <-repolled:
	case <-time.After(testWaitTimeout):
		t.Fatal("agent did not return to polling after completion was acknowledged")
	}
	if got := executions.Load(); got != 1 {
		t.Fatalf("expected one runtime execution, got %d", got)
	}
	select {
	case <-done:
		t.Fatal("agent exited instead of remaining alive and polling")
	default:
	}

	cancel()
	select {
	case <-done:
	case <-time.After(testWaitTimeout):
		t.Fatal("agent did not exit after context cancellation")
	}
}

func testBackoff() *backoff.ExponentialBackOff {
	return &backoff.ExponentialBackOff{
		InitialInterval:     time.Millisecond,
		RandomizationFactor: 0,
		Multiplier:          1,
		MaxInterval:         time.Millisecond,
	}
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

func TestRuntimeTypeSet(t *testing.T) {
	tests := map[string]struct {
		in      string
		want    RuntimeType
		wantErr bool
	}{
		"lowercase docker":         {in: "docker", want: DockerRuntimeType},
		"lowercase containerd":     {in: "containerd", want: ContainerdRuntimeType},
		"lowercase kubernetes":     {in: "kubernetes", want: KubernetesRuntimeType},
		"mixed case is normalized": {in: "Kubernetes", want: KubernetesRuntimeType},
		"unknown value rejected":   {in: "bogus", wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var r RuntimeType
			err := r.Set(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if r != tt.want {
				t.Errorf("Set(%q): got %q, want %q", tt.in, r, tt.want)
			}
		})
	}
}
