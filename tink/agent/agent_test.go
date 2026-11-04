package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func (m *mock) Read(_ context.Context) (spec.Action, error) {
	return spec.Action{}, nil
}

func (m *mock) Execute(_ context.Context, _ spec.Action) error {
	return nil
}

func (m *mock) Write(_ context.Context, _ spec.Event) error {
	return nil
}

// TestRun verifies Run stops after cancellation even when every call succeeds and ignores ctx.
func TestRun(t *testing.T) {
	c := &Config{TransportReader: &mock{}, RuntimeExecutor: &mock{}, TransportWriter: &mock{}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx, logr.Discard())
		close(done)
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(testWaitTimeout):
		t.Fatal("Run did not exit after context cancellation")
	}
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
		return spec.Action{}, classified{noAction: true}
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
		return classified{permanent: true}
	})

	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: executor,
		TransportWriter: writer,
		Retry:           testRetry(),
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
		return spec.Action{}, classified{noAction: true}
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
		Retry:           testRetry(),
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

func testRetry() Retry {
	return Retry{PollInterval: time.Millisecond, MaxInterval: time.Millisecond}
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
		Retry:           testRetry(),
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

// permanentWriter always returns a permanent error on completion writes.
type permanentWriter struct {
	calls atomic.Int32
}

func (w *permanentWriter) Write(_ context.Context, _ spec.Event) error {
	n := int(w.calls.Add(1))
	if n == 1 {
		return nil // "running" event succeeds
	}
	return classified{permanent: true}
}

func TestRunReturnsToPollingAfterPermanentReportError(t *testing.T) {
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
		return spec.Action{}, classified{noAction: true}
	})
	writer := &permanentWriter{}
	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: &mock{},
		TransportWriter: writer,
		Retry:           testRetry(),
	}

	done := make(chan struct{})
	go func() {
		c.Run(ctx, logr.Discard())
		close(done)
	}()

	select {
	case <-repolled:
	case <-time.After(testWaitTimeout):
		t.Fatal("agent did not return to polling after the final report was rejected")
	}
	// 1 "running" write + 1 rejected completion write, which is not retried.
	if got := writer.calls.Load(); got != 2 {
		t.Errorf("expected 2 Write calls, got %d", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(testWaitTimeout):
		t.Fatal("agent did not exit after context cancellation")
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

// singleActionReader serves one action, then reports that nothing else is available. It signals
// once the agent has come back to poll again, which is how tests know a full iteration finished.
type singleActionReader struct {
	action   spec.Action
	reads    atomic.Int32
	repolled chan struct{}
	once     sync.Once
}

// testNoActionError is what a transport returns when there is no Action to run.
type testNoActionError struct{}

func (testNoActionError) Error() string  { return "no action" }
func (testNoActionError) NoAction() bool { return true }

func newSingleActionReader(action spec.Action) *singleActionReader {
	return &singleActionReader{action: action, repolled: make(chan struct{})}
}

func (r *singleActionReader) Read(context.Context) (spec.Action, error) {
	if r.reads.Add(1) == 1 {
		return r.action, nil
	}
	r.once.Do(func() { close(r.repolled) })
	return spec.Action{}, testNoActionError{}
}

func (r *singleActionReader) waitForRepoll(t *testing.T) {
	t.Helper()
	select {
	case <-r.repolled:
	case <-time.After(testWaitTimeout):
		t.Fatal("agent did not return to polling")
	}
}

// collectStates records the state of every reported event in order.
func collectStates(states *[]spec.State, mu *sync.Mutex) writerFunc {
	return func(_ context.Context, event spec.Event) error {
		mu.Lock()
		defer mu.Unlock()
		*states = append(*states, event.State)
		return nil
	}
}

func TestRunDispatchesScriptActionsToScriptExecutor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := newSingleActionReader(spec.Action{ID: "action-1", Run: "echo hi", TimeoutSeconds: 5})

	var runtimeCalls, scriptCalls atomic.Int32
	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: executorFunc(func(context.Context, spec.Action) error {
			runtimeCalls.Add(1)
			return nil
		}),
		ScriptExecutor: executorFunc(func(context.Context, spec.Action) error {
			scriptCalls.Add(1)
			return nil
		}),
		TransportWriter: writerFunc(func(context.Context, spec.Event) error { return nil }),
		Retry:           testRetry(),
	}
	go c.Run(ctx, logr.Discard())

	reader.waitForRepoll(t)
	if got := scriptCalls.Load(); got != 1 {
		t.Errorf("script executor called %d times, want 1", got)
	}
	if got := runtimeCalls.Load(); got != 0 {
		t.Errorf("container runtime called %d times for a run action, want 0", got)
	}
}

func TestRunDispatchesImageActionsToRuntimeExecutor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := newSingleActionReader(spec.Action{ID: "action-1", Image: "alpine", TimeoutSeconds: 5})

	var runtimeCalls, scriptCalls atomic.Int32
	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: executorFunc(func(context.Context, spec.Action) error {
			runtimeCalls.Add(1)
			return nil
		}),
		ScriptExecutor: executorFunc(func(context.Context, spec.Action) error {
			scriptCalls.Add(1)
			return nil
		}),
		TransportWriter: writerFunc(func(context.Context, spec.Event) error { return nil }),
		Retry:           testRetry(),
	}
	go c.Run(ctx, logr.Discard())

	reader.waitForRepoll(t)
	if got := runtimeCalls.Load(); got != 1 {
		t.Errorf("container runtime called %d times, want 1", got)
	}
	if got := scriptCalls.Load(); got != 0 {
		t.Errorf("script executor called %d times for an image action, want 0", got)
	}
}

func TestRunBackgroundActionReportsSuccessBeforeExecuting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := newSingleActionReader(spec.Action{ID: "action-1", Image: "alpine", Background: true, TimeoutSeconds: 5})

	// The executor blocks until released, so if the success report waited on execution the test
	// would time out rather than pass by luck.
	release := make(chan struct{})
	executed := make(chan struct{})
	executor := executorFunc(func(context.Context, spec.Action) error {
		<-release
		close(executed)
		return nil
	})

	var mu sync.Mutex
	states := []spec.State{}

	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: executor,
		ScriptExecutor:  executor,
		TransportWriter: collectStates(&states, &mu),
		Retry:           testRetry(),
	}
	go c.Run(ctx, logr.Discard())

	reader.waitForRepoll(t)

	mu.Lock()
	got := append([]spec.State{}, states...)
	mu.Unlock()
	want := []spec.State{spec.StateRunning, spec.StateSuccess}
	if len(got) != len(want) {
		t.Fatalf("reported states = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reported states = %v, want %v", got, want)
		}
	}

	close(release)
	select {
	case <-executed:
	case <-time.After(testWaitTimeout):
		t.Fatal("background action never executed")
	}
}

func TestRunBackgroundActionFailureIsNotReported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader := newSingleActionReader(spec.Action{ID: "action-1", Run: "false", Background: true})

	failed := make(chan struct{})
	var mu sync.Mutex
	states := []spec.State{}

	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: executorFunc(func(context.Context, spec.Action) error { return nil }),
		ScriptExecutor: executorFunc(func(context.Context, spec.Action) error {
			close(failed)
			return errors.New("boom")
		}),
		TransportWriter: collectStates(&states, &mu),
		Retry:           testRetry(),
	}
	go c.Run(ctx, logr.Discard())

	select {
	case <-failed:
	case <-time.After(testWaitTimeout):
		t.Fatal("background action never executed")
	}
	reader.waitForRepoll(t)

	mu.Lock()
	defer mu.Unlock()
	for _, s := range states {
		if s == spec.StateFailure || s == spec.StateTimeout {
			t.Fatalf("background failure leaked into the workflow: reported states = %v", states)
		}
	}
}

func TestRunZeroTimeoutDoesNotTimeOut(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TimeoutSeconds is deliberately unset: a zero timeout means no timeout, not an instantly
	// expired one.
	reader := newSingleActionReader(spec.Action{ID: "action-1", Image: "alpine"})

	var mu sync.Mutex
	states := []spec.State{}

	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: executorFunc(func(ctx context.Context, _ spec.Action) error {
			return ctx.Err()
		}),
		TransportWriter: collectStates(&states, &mu),
		Retry:           testRetry(),
	}
	go c.Run(ctx, logr.Discard())

	reader.waitForRepoll(t)

	mu.Lock()
	defer mu.Unlock()
	want := []spec.State{spec.StateRunning, spec.StateSuccess}
	if len(states) != len(want) || states[len(states)-1] != spec.StateSuccess {
		t.Fatalf("reported states = %v, want %v", states, want)
	}
}

func TestRunReportsFailureWhenNoExecutorIsConfigured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A run action with no ScriptExecutor wired up must fail the action, not panic the agent.
	reader := newSingleActionReader(spec.Action{ID: "action-1", Run: "echo hi"})

	var mu sync.Mutex
	states := []spec.State{}

	c := &Config{
		TransportReader: reader,
		RuntimeExecutor: executorFunc(func(context.Context, spec.Action) error { return nil }),
		TransportWriter: collectStates(&states, &mu),
		Retry:           testRetry(),
	}
	go c.Run(ctx, logr.Discard())

	reader.waitForRepoll(t)

	mu.Lock()
	defer mu.Unlock()
	if len(states) == 0 || states[len(states)-1] != spec.StateFailure {
		t.Fatalf("reported states = %v, want the last to be %v", states, spec.StateFailure)
	}
}
