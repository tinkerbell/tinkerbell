package hardware

import (
	"context"
	"net"
)

// Resolver looks up hardware by MAC or by client IP and returns the
// translated Info struct. Wraps the lower-level BackendReader so callers
// don't depend on the data.HardwareFilter shape.
type Resolver interface {
	ByMAC(ctx context.Context, mac net.HardwareAddr) (Info, error)
	ByIP(ctx context.Context, ip net.IP) (Info, error)
}

// BackendResolver adapts a BackendReader to the Resolver interface.
type BackendResolver struct {
	Backend BackendReader
}

func (r BackendResolver) ByMAC(ctx context.Context, mac net.HardwareAddr) (Info, error) {
	return GetByMac(ctx, mac, r.Backend)
}

func (r BackendResolver) ByIP(ctx context.Context, ip net.IP) (Info, error) {
	return GetByIP(ctx, ip, r.Backend)
}
