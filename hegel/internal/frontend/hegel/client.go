//go:build ignore

package hegel

import "context"

// Client is the old hardware client before frontend-backend refactor.
type Client interface {
	IsHealthy(ctx context.Context) bool
	ByIP(ctx context.Context, ip string) (Hardware, error)
	GetDataModel() int
}

type Hardware interface {
	Export() ([]byte, error)
	ID() (string, error)
}
