package secondstar

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net"
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

	// Bind every family before serving any of them so a later bind failure
	// cannot leave an already started server running after Start returns.
	var listeners []net.Listener
	for _, addrPort := range addrPorts {
		lis, err := listener.TCP(ctx, addrPort.Addr(), int(addrPort.Port()))
		if err != nil {
			closeListeners(listeners)
			return err
		}
		listeners = append(listeners, lis)
	}

	hostKey := c.HostKey
	if hostKey == nil {
		// One generated key, so both families present the same SSH identity.
		var err error
		if hostKey, err = generateHostKey(); err != nil {
			closeListeners(listeners)
			return err
		}
	}

	// One store for every family, so a session arriving over the other family
	// attaches to the running SOL session instead of starting a second one.
	handler := internal.Handler(log, internal.NewKeyValueStore(), c.IPMITOOLPath)
	pubkeyAuth := internal.PubkeyAuth(c.Backend, log)

	g, ctx := errgroup.WithContext(ctx)
	for _, lis := range listeners {
		server := &gssh.Server{
			Handler:          handler,
			PublicKeyHandler: pubkeyAuth,
			Banner:           "Second star to the right and straight on 'til morning\n[Use ~. to disconnect]\n",
			IdleTimeout:      c.IdleTimeout,
		}
		server.AddHostKey(hostKey)

		log.Info("starting ssh server", "addrPort", lis.Addr().String())

		go func() { //nolint:gosec // G118: ctx is already cancelled here; a fresh context is needed for graceful shutdown
			<-ctx.Done()
			log.Info("shutting down ssh server", "addrPort", lis.Addr().String())
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

func closeListeners(listeners []net.Listener) {
	for _, lis := range listeners {
		_ = lis.Close()
	}
}

// generateHostKey matches the key gliderlabs/ssh generates when none is set.
func generateHostKey() (ssh.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("error generating host key: %w", err)
	}

	return ssh.NewSignerFromKey(key)
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
