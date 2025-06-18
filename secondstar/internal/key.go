package internal

import (
	"context"

	"github.com/gliderlabs/ssh"
	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	gossh "golang.org/x/crypto/ssh"
)

type Reader interface {
	ReadBMCMachine(ctx context.Context, name string) (*data.BMCMachine, error)
}

type contextKey string

const (
	BMCDataKey contextKey = "bmc"
)

// PubkeyAuth is a function that returns a function that can be used as a ssh.PublicKeyHandler
// We always return true so that the session handler can print a helpful error message to the user.
// The session handler must check the context for the error value and close the session if it is set.
func PubkeyAuth(r Reader, log logr.Logger) func(ssh.Context, ssh.PublicKey) bool {
	return func(ctx ssh.Context, key ssh.PublicKey) bool {
		hw, err := r.ReadBMCMachine(ctx, ctx.User())
		if err != nil {
			log.Info("error reading bmc machine", "error", err)
			return false
		}

		if hw == nil {
			log.Info("no bmc machine is nil", "user", ctx.User())
			return false
		}

		for _, k := range hw.SSHPublicKeys {
			pkey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(k))
			if err != nil {
				log.Info("error parsing authorized key", "error", err, "key", k)
				continue
			}
			if ssh.KeysEqual(key, pkey) {
				ctx.SetValue(BMCDataKey, *hw)
				return true
			}
		}

		log.V(1).Info("no matching key found", "user", ctx.User(), "publicKey", string(gossh.MarshalAuthorizedKey(key)))
		return false
	}
}
