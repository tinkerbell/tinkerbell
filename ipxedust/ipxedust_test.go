package ipxedust

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestListenAndServe(t *testing.T) {
	tests := []struct {
		name   string
		tftp   ServerSpec
		http   ServerSpec
		nilErr bool
	}{
		{
			name:   "success",
			tftp:   ServerSpec{Addr: netip.AddrPortFrom(netip.IPv4Unspecified(), 6969), Timeout: 5 * time.Second},
			nilErr: true,
		},
		{
			name:   "fail bind permission denied",
			tftp:   ServerSpec{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 69), Timeout: 5 * time.Second},
			nilErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				TFTP:                 tt.tftp,
				HTTP:                 tt.http,
				EnableTFTPSinglePort: true,
			}
			ctx, cn := context.WithCancel(context.Background())
			go time.AfterFunc(time.Millisecond, cn)
			err := s.ListenAndServe(ctx)
			if (err != nil) == tt.nilErr {
				t.Errorf("got: ListenAndServe() = %v, err should be nil: %v", err, tt.nilErr)
			}
		})
	}
}

func TestServe(t *testing.T) {
	tests := []struct {
		name       string
		tftp       ServerSpec
		nilErr     bool
		wantTCPErr bool
		wantUDPErr bool
	}{
		{
			name:   "success",
			tftp:   ServerSpec{Addr: netip.AddrPortFrom(netip.IPv4Unspecified(), 6868), Timeout: 5 * time.Second},
			nilErr: false,
		},
		{
			name:       "fail tcp listener",
			tftp:       ServerSpec{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 69), Timeout: 5 * time.Second},
			nilErr:     false,
			wantTCPErr: true,
		},
		{
			name:       "fail udp listener",
			tftp:       ServerSpec{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 69), Timeout: 5 * time.Second},
			nilErr:     false,
			wantUDPErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{
				TFTP:                 tt.tftp,
				EnableTFTPSinglePort: true,
			}
			ctx, cn := context.WithCancel(context.Background())

			var conn net.Listener
			var uconn net.PacketConn
			var err error
			if !tt.wantTCPErr {
				conn, err = net.Listen("tcp", "0.0.0.0:0")
				if err != nil {
					t.Fatal(err)
				}
			}
			if !tt.wantUDPErr {
				a, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
				if err != nil {
					t.Fatal(err)
				}
				uconn, err = net.ListenUDP("udp", a)
				if err != nil {
					t.Fatal(err)
				}
			}

			go time.AfterFunc(time.Millisecond, cn)
			err = c.Serve(ctx, conn, uconn)

			if (err != nil) == tt.nilErr {
				t.Errorf("got: ListenAndServe() = %v, err should be nil: %v", err, tt.nilErr)
				t.Errorf("got c.Serve(ctx, tcpConn, udpConn) = %v, type: %[1]T", err)
			}
		})
	}
}

func TestListenAndServeHTTP(t *testing.T) {
	tests := []struct {
		name   string
		attr   ServerSpec
		nilErr bool
	}{
		{
			name:   "fail net.OpError",
			attr:   ServerSpec{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 80), Timeout: 5 * time.Second},
			nilErr: false,
		},
		{
			name:   "success Server Closed",
			attr:   ServerSpec{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 8080), Timeout: 5 * time.Second},
			nilErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{HTTP: tt.attr, Log: logr.Discard()}
			ctx, cancel := context.WithCancel(context.Background())
			go time.AfterFunc(time.Millisecond, cancel)
			err := c.listenAndServeHTTP(ctx)

			if (err != nil) == tt.nilErr {
				t.Errorf("got c.listenAndServeHTTP(ctx) = %v, type: %[1]T", err)
			}
		})
	}
}

func TestServeHTTP(t *testing.T) {
	tests := []struct {
		name        string
		attr        ServerSpec
		nilErr      bool
		nilListener bool
	}{
		{
			name:   "success Server Closed",
			attr:   ServerSpec{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 32456)},
			nilErr: false,
		},
		{
			name:        "fail nil conn",
			nilErr:      false,
			nilListener: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{TFTP: tt.attr, Log: logr.Discard()}
			addr := tt.attr.Addr.String()
			if (tt.attr.Addr == netip.AddrPort{}) {
				addr = netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 32456).String()
			}
			conn, err := net.Listen("tcp", addr)
			if err != nil {
				t.Fatal(err)
			}
			if tt.nilListener {
				conn = nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			go time.AfterFunc(time.Millisecond, cancel)
			err = c.serveHTTP(ctx, conn)
			if (err != nil) == tt.nilErr {
				t.Fatalf("expected error, got: %T: %[1]v", err)
			}
		})
	}
}

func TestListenAndServeTFTP(t *testing.T) {
	tests := []struct {
		name   string
		attr   ServerSpec
		nilErr bool
	}{
		{
			name:   "fail net.OpError",
			attr:   ServerSpec{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 80), Timeout: 5 * time.Second},
			nilErr: false,
		},
		{
			name:   "success Server Closed",
			attr:   ServerSpec{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 8080), Timeout: 5 * time.Second},
			nilErr: true,
		},
		{
			name:   "fail bad address",
			attr:   ServerSpec{},
			nilErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{TFTP: tt.attr, Log: logr.Discard()}
			ctx, cancel := context.WithCancel(context.Background())
			go time.AfterFunc(time.Millisecond, cancel)
			err := c.listenAndServeTFTP(ctx)
			if (err != nil) == tt.nilErr {
				t.Errorf("got c.listenAndServeTFTP(ctx) = %v, type: %[1]T", err)
			}
		})
	}
}

func TestServeTFTP(t *testing.T) {
	tests := []struct {
		name   string
		attr   ServerSpec
		nilErr bool
	}{
		{
			name:   "success Server Closed",
			attr:   ServerSpec{Addr: netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), 32456)},
			nilErr: true,
		},
		{
			name:   "fail nil conn",
			nilErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{TFTP: tt.attr, Log: logr.Discard()}
			var uconn *net.UDPConn
			if tt.nilErr {
				a, err := net.ResolveUDPAddr("udp", tt.attr.Addr.String())
				if err != nil {
					t.Fatal(err)
				}
				uconn, err = net.ListenUDP("udp", a)
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			go time.AfterFunc(time.Millisecond, cancel)
			err := c.serveTFTP(ctx, uconn)

			if (err != nil) == tt.nilErr {
				t.Errorf("got c.serveTFTP(ctx, uconn) = %v, type: %[1]T", err)
			}
		})
	}
}
