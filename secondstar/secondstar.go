package secondstar

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
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
	BindAddr     netip.Addr
	SSHPort      int
	HostKey      ssh.Signer
	IPMITOOLPath string
	IdleTimeout  time.Duration
	Backend      Reader
}

func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	addrPort := fmt.Sprintf(":%d", c.SSHPort)
	if c.BindAddr.IsValid() && !c.BindAddr.IsUnspecified() {
		addrPort = fmt.Sprintf("%s:%d", c.BindAddr.String(), c.SSHPort)
	}
	log.Info("starting ssh server", "addrPort", addrPort)
	server := &gssh.Server{
		Addr:             addrPort,
		Handler:          internal.Handler(log, internal.NewKeyValueStore(), c.IPMITOOLPath),
		PublicKeyHandler: internal.PubkeyAuth(c.Backend, log),
		Banner:           "Second star to the right and straight on 'til morning\n[Use ~. to disconnect]\n",
		IdleTimeout:      c.IdleTimeout,
	}

	// when c.HostKey is nil, the server will generate a new host key on every start.
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
