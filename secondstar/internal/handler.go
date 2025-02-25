package internal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/gliderlabs/ssh"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

type Handle struct{}

// State is the internal State needed to track multiple sessions
// and provide a way to share stdin between sessions.
type State struct {
	file       *os.File
	mainClosed chan struct{}
	// connectedSessions is a map of all the extra sessions connected to the main session
	connectedSessions map[string]struct{}
	multiwriter       *MultiWriter
	stdin             io.Writer
}

func Handler(ctx context.Context, log logr.Logger, globalState map[string]State, ipmitoolPath string) func(s ssh.Session) {
	return func(s ssh.Session) {
		// search for an ipmitool session that is already running with the same user and host

		log.Info("new session", "user", s.User())

		if st, found := globalState[s.User()]; found {
			log.Info("connecting to existing session")
			//_, winCh, _ := s.Pty()
			//go func() {
			//	for win := range winCh {
			//		setWinsize(st.file, win.Width, win.Height)
			//	}
			//}()

			name := fmt.Sprintf("%v-%v", s.User(), len(st.connectedSessions)+1)
			st.connectedSessions[name] = struct{}{}
			st.multiwriter.Add(s) // stdout
			defer func() {
				st.multiwriter.Remove(s)
				delete(st.connectedSessions, name)
			}()
			escapeReader, escapeWriter := io.Pipe()
			mw := io.MultiWriter(st.stdin, escapeWriter)
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
							log.Info("escape sequence detected")
							s.Exit(0)
							return
						}
					}
				}
			}()
			go io.Copy(mw, s) // stdin
			select {
			case <-st.mainClosed:
				log.Info("main session closed", "name", name)
				s.Exit(0)
				return
			case <-ctx.Done():
				log.Info("context done", "name", name)
				s.Exit(0)
				return
			case <-s.Context().Done():
				log.Info("session context done", "name", name)
				s.Exit(0)
				return
			}
		}
		// Get the bmc ref from the context
		// lookup the machine.bmc object from the cluster. This gives us the host and port and secret reference.
		// lookup the secret object from the cluster. This gives us the user and pass.
		// session user will eventually be the Hardware name and will be used to lookup all credential info. Also, maybe ssh key for validation.
		bmc, ok := s.Context().Value("bmc").(data.BMCMachine)
		if !ok {
			log.Info("error getting bmc info, exiting session")
			s.Exit(1)
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
			s.Exit(2)
			return
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			log.Error(err, "error getting stdout pipe")
			s.Exit(2)
			return
		}
		if err := cmd.Start(); err != nil {
			log.Error(err, "error starting command")
			s.Exit(2)
			return
		}

		escapeReader, escapeWriter := io.Pipe()
		mw := io.MultiWriter(in, escapeWriter)

		exp := New()
		wr := io.MultiWriter(s, exp)
		globalState[s.User()] = State{
			connectedSessions: make(map[string]struct{}),
			mainClosed:        make(chan struct{}),
			multiwriter:       exp,
			stdin:             in,
		}

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
						log.Info("escape sequence detected")
						s.Exit(0)
						return
					}
				}
			}
		}()

		go func() {
			io.Copy(mw, s) // stdin
		}()

		io.Copy(wr, out) // stdout

		if err := cmd.Wait(); err != nil {
			ps := cmd.ProcessState
			status := ps.Sys().(syscall.WaitStatus)
			switch {
			case status.Exited():
				log.Info("process exited", "status", status.ExitStatus())
			case status.Signaled():
				log.Info("process signaled", "signal", status.Signal().String())
			case status.Stopped():
				log.Info("process stopped", "signal", status.Signal().String())
			default:
				log.Error(err, "error waiting for command")
			}
		}

		if len(globalState[s.User()].connectedSessions) > 0 {
			globalState[s.User()].mainClosed <- struct{}{}
		}
		// close all the channels in the tracker
		close(globalState[s.User()].mainClosed)
		delete(globalState, s.User())
		log.Info("initial session closed")

		deactivateArgs := []string{ipmitoolPath, "-I", "lanplus", "-E", "-H", bmc.Host, "-U", bmc.User, "-p", strconv.Itoa(bmc.Port), "sol", "deactivate"}
		deactivateCmd := exec.CommandContext(context.Background(), deactivateArgs[0], deactivateArgs[1:]...)
		deactivateCmd.Env = append(deactivateCmd.Env, fmt.Sprintf("IPMITOOL_PASSWORD=%s", bmc.Pass))
		if out, err := deactivateCmd.CombinedOutput(); err != nil {
			// TODO: Check if the error is due to the sol already being deactivated
			log.Error(err, "error deactivating sol", "output", string(out))
		}

		log.Info("session closed")
	}
}
