// Package script executes an Action's inline script directly on the host the Agent runs on,
// outside any container runtime. It is the counterpart to the docker, containerd and kubernetes
// runtimes and satisfies the same executor contract: Execute blocks until the script completes.
//
// This exists for Agents that run as root on a capable host OS, where wrapping a handful of shell
// commands in an OCI image, publishing it to a registry and pulling it back is pure overhead. It
// is selected per Action, by the Action carrying a run field rather than an image, so a single
// Workflow can freely mix container Actions and host script Actions.
package script

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/pkg/conv"
	"github.com/tinkerbell/tinkerbell/tink/agent/internal/spec"
)

// defaultShell is the interpreter argv used when an Action does not specify one. -e aborts the
// script at the first failing command so a partial run cannot be reported as a success, and -x
// traces every command to stderr so the Agent log shows what actually ran.
var defaultShell = []string{"bash", "-x", "-e"}

// scriptMode is the permission the script file is written with: readable and executable by the
// owner only. Action scripts routinely carry credentials and are written to a shared directory.
const scriptMode os.FileMode = 0o700

type Config struct {
	Log logr.Logger
	// ScriptDir is the directory Action scripts are written to. Defaults to os.TempDir().
	ScriptDir string
}

// Execute writes the Action's script to a file and runs it with the Action's shell, blocking until
// it exits. Stdout and stderr are streamed line by line to the Agent log, matching the container
// runtimes, which likewise surface Action output only through the log.
func (c *Config) Execute(ctx context.Context, a spec.Action) error {
	// The controller rejects these when rendering the Template. Repeat the check here because the
	// file and NATS transports read Actions straight off disk or the wire without that validation.
	if a.Run == "" {
		return errors.New("script: action has no run script")
	}
	if a.Image != "" {
		return errors.New("script: image and run are mutually exclusive")
	}

	shell := a.Shell
	if len(shell) == 0 {
		shell = defaultShell
	}

	dir := c.ScriptDir
	if dir == "" {
		dir = os.TempDir()
	}
	scriptPath := filepath.Join(dir, conv.ParseName(a.ID, a.Name)+".sh")
	if err := os.WriteFile(scriptPath, []byte(a.Run), scriptMode); err != nil {
		return fmt.Errorf("script: writing script file: %w", err)
	}
	defer func() {
		if err := os.Remove(scriptPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.Log.Info("couldn't remove action script", "script", scriptPath, "error", err)
		}
	}()

	args := append(append([]string{}, shell[1:]...), scriptPath)
	// The command is built from the Action's own shell and script: running operator-supplied code
	// is this runtime's entire purpose, not an injection risk it could defend against.
	cmd := exec.CommandContext(ctx, shell[0], args...)

	// Inherit the Agent's environment so the script sees the host's PATH and friends, then overlay
	// the Action's own variables on top so they win on conflict.
	cmd.Env = append(os.Environ(), conv.ParseEnv(a.Env)...)

	stdout := newLogWriter(c.Log, a.Name, "stdout")
	stderr := newLogWriter(c.Log, a.Name, "stderr")
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// Put the script in its own process group so cancellation reaches the whole tree. A shell
	// script's real work usually happens in child processes, and killing only the shell would
	// orphan them to keep running against a machine the Workflow has moved on from.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	runErr := cmd.Run()

	// Emit whatever the script left without a trailing newline before reporting the result, so the
	// log carries the final line of a script that died mid-write.
	stdout.Flush()
	stderr.Flush()

	if runErr != nil {
		// Report the context's own error when it ended the script, so the Agent can tell a timeout
		// apart from a failure. cmd.Cancel kills the process, which otherwise surfaces as a plain
		// signal error with no hint that a deadline caused it.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("script: %w", ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return fmt.Errorf("script: exited %d, see the logs for more information", exitErr.ExitCode())
		}
		return fmt.Errorf("script: %w", runErr)
	}

	return nil
}
