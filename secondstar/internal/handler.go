package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/gliderlabs/ssh"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

// State is the internal State needed to track multiple sessions
// and provide a way to share stdin between sessions.
type State struct {
	// initialClosed is a channel to signal to all connected sessions that the initial session has closed.
	initialClosed chan struct{}
	// wg is used to wait for all connected sessions to close before closing
	// the main session and removing state from the global state.
	wg sync.WaitGroup
	// additionalSessions is a map of all the additional sessions connected to the initial session.
	additionalSessions atomic.Int32
	// multiwriter allows multiple sessions to have the same stdout.
	multiwriter *MultiWriter
	// stdin is the shared stdin for all connected sessions. A multiwriter is not needed here because
	// each ssh session has its own stdin.
	stdin io.Writer
}

// Handler returns a function that can be used as the ssh.Handler for the gliderlabs/ssh server.
func Handler(log logr.Logger, globalState *KeyValueStore, ipmitoolPath string) func(s ssh.Session) {
	return func(s ssh.Session) {
		if st, found := globalState.Get(s.User()); found {
			additionalSession(log, s, st)
			return
		}
		initialSession(log, s, globalState, ipmitoolPath)
	}
}

// initialSession is the handler for the initial or first session connected to the ssh server for a specific host.
func initialSession(log logr.Logger, s ssh.Session, globalState *KeyValueStore, ipmitoolPath string) {
	log = log.WithValues("user", s.User(), "sessionName", s.User(), "mainSession", true)
	log.V(2).Info("new session")
	// Get the bmc ref from the context
	// lookup the machine.bmc object from the cluster. This gives us the host and port and secret reference.
	// lookup the secret object from the cluster. This gives us the user and pass.
	// session user will eventually be the Hardware name and will be used to lookup all credential info. Also, maybe ssh key for validation.
	bmc, ok := s.Context().Value(BMCDataKey).(data.BMCMachine)
	if !ok {
		log.V(2).Info("error getting bmc info, exiting session")
		if err := s.Exit(1); err != nil {
			log.Error(err, "error closing session")
		}
		return
	}

	ipmitoolCMD := []string{ipmitoolPath, "-I", "lanplus", "-E", "-H", bmc.Host, "-U", bmc.User, "-p", strconv.Itoa(bmc.Port), "sol", "activate"}
	cmd := exec.CommandContext(s.Context(), ipmitoolCMD[0], ipmitoolCMD[1:]...)
	ptyReq, _, _ := s.Pty()
	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
	cmd.Env = append(cmd.Env, fmt.Sprintf("IPMITOOL_PASSWORD=%s", bmc.Pass))
	cmd.Env = append(cmd.Env, fmt.Sprintf("IPMITOOL_USERNAME=%s", bmc.User))
	cmd.Env = append(cmd.Env, fmt.Sprintf("IPMITOOL_CIPHER_SUITE=%s", bmc.CipherSuite))
	cmd.Env = append(cmd.Env, fmt.Sprintf("IPMITOOL_PORT=%d", bmc.Port))
	cmd.Env = append(cmd.Env, fmt.Sprintf("IPMITOOL_HOST=%s", bmc.Host))

	in, err := cmd.StdinPipe()
	if err != nil {
		log.Error(err, "error getting stdin pipe")
		if err := s.Exit(2); err != nil {
			log.Error(err, "error closing session")
		}
		return
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		log.Error(err, "error getting stdout pipe")
		if err := s.Exit(2); err != nil {
			log.Error(err, "error closing session")
		}
		return
	}
	if err := cmd.Start(); err != nil {
		log.Error(err, "error starting command")
		if err := s.Exit(2); err != nil {
			log.Error(err, "error closing session")
		}
		return
	}

	escapeReader, escapeWriter := io.Pipe()
	mw := io.MultiWriter(in, escapeWriter)

	exp := NewMultiWriter()
	wr := io.MultiWriter(s, exp)
	globalState.Set(s.User(), &State{
		wg:                 sync.WaitGroup{},
		additionalSessions: atomic.Int32{},
		initialClosed:      make(chan struct{}),
		multiwriter:        exp,
		stdin:              in,
	})

	// watch for escape sequences
	// escape sequence is ~.
	// if ~. is detected, close the session
	go func() {
		for {
			b := make([]byte, 1)
			_, err := escapeReader.Read(b)
			if err != nil {
				log.Error(err, "error reading escape sequence")
				return
			}
			if b[0] == '~' {
				_, err := escapeReader.Read(b)
				if err != nil {
					log.Error(err, "error reading escape sequence")
					return
				}
				if b[0] == '.' {
					log.V(2).Info("escape sequence detected")
					if err := s.Exit(0); err != nil {
						log.Error(err, "error closing session")
					}
					return
				}
			}
		}
	}()

	go func() {
		if _, err := io.Copy(mw, s); err != nil { // stdin
			log.Error(err, "error copying stdin")
		}
	}()

	go func() {
		if _, err := io.Copy(wr, out); err != nil { // stdout
			log.Error(err, "error copying stdout")
		}
	}()

	if err := cmd.Wait(); err != nil {
		ps := cmd.ProcessState
		status, ok := ps.Sys().(syscall.WaitStatus)
		if !ok {
			log.Error(err, "error getting process state")
		}
		switch {
		case status.Exited():
			log.V(2).Info("process exited", "status", status.ExitStatus())
		case status.Signaled():
			log.V(2).Info("process signaled", "signal", status.Signal().String())
		case status.Stopped():
			log.V(2).Info("process stopped", "signal", status.Signal().String())
		default:
			log.Error(err, "error waiting for command")
		}
	}

	// if there are any connected sessions, we need to signal for them to close.
	v, ok := globalState.Get(s.User())
	if ok && v.additionalSessions.Load() > 0 {
		s, ok := globalState.Get(s.User())
		if ok {
			s.initialClosed <- struct{}{}
		}
	}
	v.wg.Wait()
	globalState.Delete(s.User())

	deactivateArgs := []string{ipmitoolPath, "-I", "lanplus", "-E", "-H", bmc.Host, "-U", bmc.User, "-p", strconv.Itoa(bmc.Port), "sol", "deactivate"}
	deactivateCmd := exec.CommandContext(context.Background(), deactivateArgs[0], deactivateArgs[1:]...)
	deactivateCmd.Env = append(deactivateCmd.Env, fmt.Sprintf("IPMITOOL_PASSWORD=%s", bmc.Pass))
	if out, err := deactivateCmd.CombinedOutput(); err != nil {
		// TODO: Check if the error is due to the sol already being deactivated
		log.Error(err, "error deactivating sol", "output", string(out))
	}

	log.V(2).Info("session closed")
}

// additionalSession is the handler for all additional sessions connected to an initial session.
func additionalSession(log logr.Logger, s ssh.Session, st *State) {
	num := st.additionalSessions.Add(1)
	name := fmt.Sprintf("%v-%v", s.User(), num)
	log = log.WithValues("sessionName", name, "user", s.User(), "mainSession", false)
	log.V(2).Info("connecting to an existing session", "user", s.User())
	st.wg.Add(1)
	defer st.wg.Done()
	st.multiwriter.Add(s) // stdout
	defer func() {
		st.multiwriter.Remove(s)
		st.additionalSessions.Add(-1)
	}()
	escapeReader, escapeWriter := io.Pipe()
	mw := io.MultiWriter(st.stdin, escapeWriter)
	exit := make(chan struct{})
	// watch for escape sequences
	// escape sequence is ~.
	// if ~. is detected, close the session
	go func() {
		for {
			b := make([]byte, 1)
			_, err := escapeReader.Read(b)
			if err != nil {
				log.Error(err, "error reading escape sequence")
				return
			}
			if b[0] == '~' {
				_, err := escapeReader.Read(b)
				if err != nil {
					log.Error(err, "error reading escape sequence")
					return
				}
				if b[0] == '.' {
					exit <- struct{}{}
					return
				}
			}
		}
	}()
	go func() {
		if _, err := io.Copy(mw, s); err != nil { // stdin
			log.Error(err, "error copying stdin")
		}
	}()
	select {
	case <-st.initialClosed:
		log.V(2).Info("closing additional session", "reason", "the main session has closed")
		if err := s.Exit(0); err != nil {
			log.Error(err, "error closing session")
		}
		return
	case <-s.Context().Done():
		log.V(2).Info("closing additional session", "reason", "context done")
		if err := s.Exit(0); err != nil && !errors.Is(err, io.EOF) {
			log.Error(err, "error closing session")
		}
		return
	case <-exit:
		log.V(2).Info("closing additional session", "reason", "escape sequence detected")
		if err := s.Exit(0); err != nil {
			log.Error(err, "error closing session")
		}
		return
	}
}
