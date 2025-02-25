package secondstar

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	"github.com/tinkerbell/tinkerbell/secondstar/internal"
	"golang.org/x/crypto/ssh"
)

type Reader interface {
	ReadBMCMachine(ctx context.Context, name string) (*data.BMCMachine, error)
}

type Config struct {
	SSHPort      int
	HostKey      ssh.Signer
	IPMITOOLPath string
	Backend      Reader
}

func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	tracker := make(map[string]internal.State, 0)
	handler := internal.Handler(ctx, log, tracker, c.IPMITOOLPath)

	log.Info("starting ssh server", "port", c.SSHPort)

	server := &gssh.Server{
		Addr:             fmt.Sprintf(":%d", c.SSHPort),
		Handler:          handler,
		PublicKeyHandler: internal.PubkeyAuth(c.Backend),
		Banner:           "Welcome to SecondStar\n",
	}

	if c.HostKey != nil {
		server.AddHostKey(c.HostKey)
	}

	go func() {
		<-ctx.Done()
		log.Info("shutting down ssh server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Error(err, "error shutting down ssh server")
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, gssh.ErrServerClosed) {
		return fmt.Errorf("failed to listen: %w", err)
	}
	return nil
}

// HostKeyFrom reads a host key from a file and returns a signer.
func HostKeyFrom(filePath string) (ssh.Signer, error) {
	hostKey, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading host key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(hostKey)
	if err != nil {
		return nil, fmt.Errorf("error parsing host key: %w", err)
	}

	return signer, nil
}
