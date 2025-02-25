package internal

import (
	"context"
	"log"

	"github.com/gliderlabs/ssh"
	"github.com/tinkerbell/tinkerbell/pkg/data"
)

type Reader interface {
	ReadBMCMachine(ctx context.Context, name string) (*data.BMCMachine, error)
}

func PubkeyAuth(r Reader) func(ssh.Context, ssh.PublicKey) bool {
	return func(ctx ssh.Context, key ssh.PublicKey) bool {
		hw, err := r.ReadBMCMachine(ctx, ctx.User())
		if err != nil {
			log.Printf("error getting hardware object: %v\n", err)
			return false
		}

		if hw == nil {
			log.Println("hardware object is nil")
			return false
		}

		ctx.SetValue("bmc", *hw)
		for _, k := range hw.SSHPublicKeys {
			pkey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(k))
			if err != nil {
				continue
			}
			if ssh.KeysEqual(key, pkey) {
				return true
			}
		}

		return false
	}
}
