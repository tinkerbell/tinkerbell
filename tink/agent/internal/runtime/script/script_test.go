package script

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

// recorder captures log calls along with the stream each came from, so tests can assert that
// stdout and stderr stay distinguishable and that output is split into one entry per line.
type recorder struct {
	mu    sync.Mutex
	lines []line
}

type line struct {
	msg    string
	stream string
}

type recorderSink struct {
	rec *recorder
}

func (s *recorderSink) Init(logr.RuntimeInfo)             {}
func (s *recorderSink) Enabled(int) bool                  { return true }
func (s *recorderSink) WithValues(...any) logr.LogSink    { return s }
func (s *recorderSink) WithName(string) logr.LogSink      { return s }
func (s *recorderSink) Error(_ error, _ string, _ ...any) {}

func (s *recorderSink) Info(_ int, msg string, keysAndValues ...any) {
	stream := ""
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		if k, ok := keysAndValues[i].(string); ok && k == "stream" {
			stream, _ = keysAndValues[i+1].(string)
		}
	}
	s.rec.mu.Lock()
	defer s.rec.mu.Unlock()
	s.rec.lines = append(s.rec.lines, line{msg: msg, stream: stream})
}

func (r *recorder) logger() logr.Logger {
	return logr.New(&recorderSink{rec: r})
}

func (r *recorder) messages(stream string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []string{}
	for _, l := range r.lines {
		if stream == "" || l.stream == stream {
			out = append(out, l.msg)
		}
	}
	return out
}

func (r *recorder) contains(stream, want string) bool {
	for _, m := range r.messages(stream) {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}

// aliveByPID reports whether a process exists. Signal 0 performs the permission and existence
// checks without delivering anything.
func aliveByPID(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func requireBash(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

func TestExecuteDefaultShell(t *testing.T) {
	requireBash(t)
	rec := &recorder{}
	c := &Config{Log: rec.logger(), ScriptDir: t.TempDir()}

	err := c.Execute(t.Context(), spec.Action{ID: "a1", Name: "hello", Run: "echo hello world\n"})
	if err != nil {
		t.Fatalf("Execute() = %v, want nil", err)
	}
	if !rec.contains("stdout", "hello world") {
		t.Errorf("stdout missing script output, got %v", rec.messages("stdout"))
	}
	// The default shell includes -x, which traces every command to stderr.
	if !rec.contains("stderr", "+ echo hello world") {
		t.Errorf("stderr missing xtrace output, got %v", rec.messages("stderr"))
	}
}

func TestExecuteDefaultShellAbortsOnFirstFailure(t *testing.T) {
	requireBash(t)
	rec := &recorder{}
	c := &Config{Log: rec.logger(), ScriptDir: t.TempDir()}

	// The default shell includes -e, so the second command must never run.
	err := c.Execute(t.Context(), spec.Action{ID: "a1", Name: "fail", Run: "false\necho unreachable\n"})
	if err == nil {
		t.Fatal("Execute() = nil, want error")
	}
	if !strings.Contains(err.Error(), "exited 1") {
		t.Errorf("Execute() = %v, want error mentioning exit code 1", err)
	}
	if rec.contains("stdout", "unreachable") {
		t.Error("script continued past a failing command; -e was not applied")
	}
}

func TestExecuteCustomShell(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	rec := &recorder{}
	c := &Config{Log: rec.logger(), ScriptDir: t.TempDir()}

	err := c.Execute(t.Context(), spec.Action{
		ID:    "a1",
		Name:  "custom",
		Shell: []string{"sh", "-u"},
		Run:   "echo from sh\n",
	})
	if err != nil {
		t.Fatalf("Execute() = %v, want nil", err)
	}
	if !rec.contains("stdout", "from sh") {
		t.Errorf("stdout missing script output, got %v", rec.messages("stdout"))
	}
	// A custom shell replaces the default entirely, so there is no -x trace.
	if rec.contains("stderr", "+ echo") {
		t.Errorf("custom shell should not inherit the default -x, got %v", rec.messages("stderr"))
	}
}

func TestExecuteEnvironment(t *testing.T) {
	requireBash(t)
	t.Setenv("SCRIPT_TEST_INHERITED", "inherited-value")
	t.Setenv("SCRIPT_TEST_OVERRIDDEN", "from-agent")

	rec := &recorder{}
	c := &Config{Log: rec.logger(), ScriptDir: t.TempDir()}

	err := c.Execute(t.Context(), spec.Action{
		ID:   "a1",
		Name: "env",
		Run:  "set +x\necho \"inherited=$SCRIPT_TEST_INHERITED\"\necho \"overridden=$SCRIPT_TEST_OVERRIDDEN\"\necho \"own=$SCRIPT_TEST_OWN\"\n",
		Env: []spec.Env{
			{Key: "SCRIPT_TEST_OVERRIDDEN", Value: "from-action"},
			{Key: "SCRIPT_TEST_OWN", Value: "own-value"},
		},
	})
	if err != nil {
		t.Fatalf("Execute() = %v, want nil", err)
	}
	for _, want := range []string{"inherited=inherited-value", "overridden=from-action", "own=own-value"} {
		if !rec.contains("stdout", want) {
			t.Errorf("stdout missing %q, got %v", want, rec.messages("stdout"))
		}
	}
}

func TestExecuteExitCode(t *testing.T) {
	requireBash(t)
	c := &Config{Log: logr.Discard(), ScriptDir: t.TempDir()}

	err := c.Execute(t.Context(), spec.Action{ID: "a1", Name: "exit", Run: "exit 42\n"})
	if err == nil {
		t.Fatal("Execute() = nil, want error")
	}
	if !strings.Contains(err.Error(), "exited 42") {
		t.Errorf("Execute() = %v, want error mentioning exit code 42", err)
	}
}

func TestExecuteStreamsAreSeparatedAndLineSplit(t *testing.T) {
	requireBash(t)
	rec := &recorder{}
	c := &Config{Log: rec.logger(), ScriptDir: t.TempDir()}

	// The last line has no trailing newline; it must still reach the log via the flush.
	err := c.Execute(t.Context(), spec.Action{
		ID:    "a1",
		Name:  "streams",
		Shell: []string{"bash", "-e"},
		Run:   "echo out-one\necho out-two\necho err-one >&2\nprintf 'no-newline'\n",
	})
	if err != nil {
		t.Fatalf("Execute() = %v, want nil", err)
	}

	stdout := rec.messages("stdout")
	want := []string{"out-one", "out-two", "no-newline"}
	if len(stdout) != len(want) {
		t.Fatalf("stdout = %v, want %v", stdout, want)
	}
	for i, w := range want {
		if stdout[i] != w {
			t.Errorf("stdout[%d] = %q, want %q", i, stdout[i], w)
		}
	}
	if got := rec.messages("stderr"); len(got) != 1 || got[0] != "err-one" {
		t.Errorf("stderr = %v, want [err-one]", got)
	}
}

func TestExecuteCancellationKillsProcessGroup(t *testing.T) {
	requireBash(t)
	dir := t.TempDir()
	// The script records the PID of a long-lived grandchild, then waits on it. Killing only the
	// shell would leave that grandchild running.
	pidFile := filepath.Join(dir, "child.pid")
	c := &Config{Log: logr.Discard(), ScriptDir: dir}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- c.Execute(ctx, spec.Action{
			ID:   "a1",
			Name: "sleeper",
			Run:  "sleep 60 &\necho $! > " + pidFile + "\nwait\n",
		})
	}()

	childPID := waitForPIDFile(t, pidFile)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Execute() = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Execute did not return after cancellation")
	}

	// The grandchild should be gone. Signal 0 only checks for existence.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !aliveByPID(childPID) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("child process %d survived cancellation", childPID)
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child pid file %s never appeared", path)
	return 0
}

func TestExecuteRejectsInvalidActions(t *testing.T) {
	c := &Config{Log: logr.Discard(), ScriptDir: t.TempDir()}

	tests := map[string]spec.Action{
		"no run script": {ID: "a1", Name: "empty"},
		"image and run": {ID: "a1", Name: "both", Image: "alpine", Run: "echo hi"},
	}
	for name, action := range tests {
		t.Run(name, func(t *testing.T) {
			if err := c.Execute(t.Context(), action); err == nil {
				t.Error("Execute() = nil, want error")
			}
		})
	}
}

func TestExecuteRemovesScriptFile(t *testing.T) {
	requireBash(t)
	dir := t.TempDir()
	c := &Config{Log: logr.Discard(), ScriptDir: dir}

	if err := c.Execute(t.Context(), spec.Action{ID: "a1", Name: "cleanup", Run: "true\n"}); err != nil {
		t.Fatalf("Execute() = %v, want nil", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("script directory not cleaned up, still holds %v", entries)
	}
}
