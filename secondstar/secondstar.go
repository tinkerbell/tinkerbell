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
	"github.com/tinkerbell/tinkerbell/pkg/listener"
	"github.com/tinkerbell/tinkerbell/secondstar/internal"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

type Reader interface {
	FilterBMCMachine(ctx context.Context, opts data.HardwareFilter) (*data.BMCMachine, error)
}

type Config struct {
	// V4 and V6 are the per-family listen addresses. An invalid Addr leaves that
	// family unserved.
	V4           netip.AddrPort
	V6           netip.AddrPort
	HostKey      ssh.Signer
	IPMITOOLPath string
	IdleTimeout  time.Duration
	Backend      Reader
}

func (c *Config) Start(ctx context.Context, log logr.Logger) error {
	var addrPorts []netip.AddrPort
	for _, ap := range []netip.AddrPort{c.V4, c.V6} {
		if ap.Addr().IsValid() {
			addrPorts = append(addrPorts, ap)
		}
	}
	if len(addrPorts) == 0 {
		return errors.New("secondstar has no IPv4 or IPv6 bind address")
	}

	g, ctx := errgroup.WithContext(ctx)
	for _, addrPort := range addrPorts {
		lis, err := listener.TCP(ctx, addrPort.Addr(), int(addrPort.Port()))
		if err != nil {
			return err
		}

		log.Info("starting ssh server", "addrPort", addrPort.String())
		server := &gssh.Server{
			Handler:          internal.Handler(log, internal.NewKeyValueStore(), c.IPMITOOLPath),
			PublicKeyHandler: internal.PubkeyAuth(c.Backend, log),
			Banner:           "Second star to the right and straight on 'til morning\n[Use ~. to disconnect]\n",
			IdleTimeout:      c.IdleTimeout,
		}

		// when c.HostKey is nil, the server will generate a new host key on every start.
		if c.HostKey != nil {
			server.AddHostKey(c.HostKey)
		}

		go func() { //nolint:gosec // G118: ctx is already cancelled here; a fresh context is needed for graceful shutdown
			<-ctx.Done()
			log.Info("shutting down ssh server", "addrPort", addrPort.String())
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Error(err, "error shutting down ssh server")
			}
		}()

		g.Go(func() error {
			if err := server.Serve(lis); err != nil && !errors.Is(err, gssh.ErrServerClosed) {
				return fmt.Errorf("failed to listen: %w", err)
			}
			return nil
		})
	}

	return g.Wait()
}

func sshAddrPort(addr netip.Addr, port int) string {
	addrPort := fmt.Sprintf(":%d", port)
	if addr.IsValid() && (!addr.Is4() || !addr.IsUnspecified()) && port >= 0 && port <= 65535 {
		addrPort = netip.AddrPortFrom(addr, uint16(port)).String()
	}
	return addrPort
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
