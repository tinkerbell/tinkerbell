package gxff

import (
	"context"
	"net"
	"os"
	"strings"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/packethost/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// converts a list of subnets' string to a list of net.IPNet.
func toMasks(ips []string) ([]net.IPNet, error) {
	var nets []net.IPNet
	for _, cidr := range ips {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		nets = append(nets, *network)
	}
	return nets, nil
}

func updateRemote(ctx context.Context, l log.Logger, masks []net.IPNet) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		l.Info("no metadata")
		return ctx
	}

	l = l.With("ctx", ctx)
	xffs := md.Get("x-forwarded-for")
	if xffs == nil {
		l.Info("no x-forwarded-for")
		return ctx
	}

	l = l.With("xff", xffs)
	remote, ok := peer.FromContext(ctx)
	if !ok {
		l.Info("could not get peer")
		return ctx
	}

	tcpAddr := remote.Addr.(*net.TCPAddr)
	rip := tcpAddr.IP
	l = l.With("remote", remote)

	allowed := false
	for _, net := range masks {
		if net.Contains(rip) {
			allowed = true
			break
		}
	}
	if !allowed {
		l.With("masks", masks).Info("remote host not in allowed list")
		return ctx
	}

	var ip *net.IPAddr
	var err error
	for _, xff := range xffs {
		ip, err = net.ResolveIPAddr("ip", xff)
		if ip != nil {
			break
		}
	}
	if err != nil || ip == nil {
		l.Info("could not resolve x-forwarded-for as an ip")
		return ctx
	}

	newRemote := &peer.Peer{
		Addr: &net.TCPAddr{
			IP:   ip.IP,
			Port: tcpAddr.Port,
			Zone: ip.Zone,
		},
		AuthInfo: remote.AuthInfo,
	}
	ctx = peer.NewContext(ctx, newRemote)
	return ctx
}

func ParseTrustedProxies() []string {
	var result []string

	trustedProxies := os.Getenv("TRUSTED_PROXIES")
	for _, cidr := range strings.Split(trustedProxies, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			// Its not a cidr, but maybe its an IP
			if ip := net.ParseIP(cidr); ip != nil {
				if ip.To4() != nil {
					cidr = cidr + "/32"
				} else {
					cidr = cidr + "/128"
				}
			} else {
				// not an IP, panic
				panic("invalid ip cidr in TRUSTED_PROXIES cidr=" + cidr)
			}
		}
		result = append(result, cidr)
	}
	return result
}

// New returns a set of grpc interceptors that will replace peer.IP with X-FORWARDED-FOR value if the peer ip within a subnet in allowedSubnets
// If allowedSubnets is nil it will look for subents in the TRUSTED_PROXIES env var.
// If allowedSubnets is nil and TRUSTED_PROXIES is empty then X-FORWARDED-FOR will be ignored (no proxy is trusted).
func New(l log.Logger, allowedSubnets []string) (grpc.StreamServerInterceptor, grpc.UnaryServerInterceptor) {
	if allowedSubnets == nil {
		allowedSubnets = ParseTrustedProxies()
		if allowedSubnets == nil {
			streamer := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				return handler(srv, ss)
			}
			unaryer := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
				return handler(ctx, req)
			}
			return streamer, unaryer
		}
	}

	masks, err := toMasks(allowedSubnets)
	if err != nil {
		return nil, nil
	}

	streamer := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		wrapped := grpc_middleware.WrapServerStream(ss)
		wrapped.WrappedContext = updateRemote(ss.Context(), l, masks)
		return handler(srv, wrapped)
	}
	unaryer := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		return handler(updateRemote(ctx, l, masks), req)
	}
	return streamer, unaryer
}
