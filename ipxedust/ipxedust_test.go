package ipxedust

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"inet.af/netaddr"
)

func TestListenAndServe(t *testing.T) {
	tests := []struct {
		name    string
		tftp    ServerSpec
		http    ServerSpec
		wantErr error
	}{
		{
			name:    "success",
			tftp:    ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 6969), Timeout: 5 * time.Second},
			wantErr: nil,
		},
		{
			name: "fail",
			tftp: ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 69), Timeout: 5 * time.Second},
			wantErr: &net.OpError{
				Op:   "listen",
				Net:  "udp",
				Addr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 69},
				Err:  fmt.Errorf("bind: permission denied"),
			},
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

			if !errors.Is(err, tt.wantErr) && !errors.As(err, &tt.wantErr) {
				t.Errorf("got c.ListenAndServe(ctx) = %v, type: %[1]T", err)
			}
		})
	}
}

func TestServe(t *testing.T) {
	tests := []struct {
		name       string
		tftp       ServerSpec
		http       ServerSpec
		wantErr    error
		wantTCPErr bool
		wantUDPErr bool
	}{
		{
			name:    "success",
			tftp:    ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 6868), Timeout: 5 * time.Second},
			wantErr: http.ErrServerClosed,
		},
		{
			name:       "fail tcp listener",
			tftp:       ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 69), Timeout: 5 * time.Second},
			wantErr:    fmt.Errorf("tcp listener must not be nil"),
			wantTCPErr: true,
		},
		{
			name:       "fail udp listener",
			tftp:       ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 69), Timeout: 5 * time.Second},
			wantErr:    fmt.Errorf("udp conn must not be nil"),
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

			if !errors.Is(err, tt.wantErr) && !errors.As(err, &tt.wantErr) {
				t.Errorf("got c.Serve(ctx, tcpConn, udpConn) = %v, type: %[1]T", err)
			}
		})
	}
}

func TestListenAndServeHTTP(t *testing.T) {
	tests := []struct {
		name    string
		attr    ServerSpec
		wantErr error
	}{
		{
			name: "fail net.OpError",
			attr: ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 80), Timeout: 5 * time.Second},
			wantErr: &net.OpError{
				Op:   "listen",
				Net:  "tcp",
				Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80},
				Err:  fmt.Errorf("bind: permission denied"),
			},
		},
		{
			name:    "success Server Closed",
			attr:    ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 8080), Timeout: 5 * time.Second},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{HTTP: tt.attr, Log: logr.Discard()}
			ctx, cancel := context.WithCancel(context.Background())
			go time.AfterFunc(time.Millisecond, cancel)
			err := c.listenAndServeHTTP(ctx)

			if !errors.Is(err, tt.wantErr) && !errors.As(err, &tt.wantErr) {
				t.Errorf("got c.listenAndServeHTTP(ctx) = %v, type: %[1]T", err)
			}
		})
	}
}

func TestServeHTTP(t *testing.T) {
	tests := []struct {
		name        string
		attr        ServerSpec
		wantErr     error
		nilListener bool
	}{
		{
			name:    "success Server Closed",
			attr:    ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 9090)},
			wantErr: http.ErrServerClosed,
		},
		{
			name:        "fail nil conn",
			wantErr:     errNilListener,
			nilListener: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{TFTP: tt.attr, Log: logr.Discard()}
			addr := tt.attr.Addr.String()
			if tt.attr.Addr.IsZero() {
				addr = netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 9090).String()
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
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error, got: %T: %[1]v", err)
			}
		})
	}
}

func TestListenAndServeTFTP(t *testing.T) {
	tests := []struct {
		name    string
		attr    ServerSpec
		wantErr error
	}{
		{
			name: "fail net.OpError",
			attr: ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 80), Timeout: 5 * time.Second},
			wantErr: &net.OpError{
				Op:   "listen",
				Net:  "udp",
				Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80},
				Err:  fmt.Errorf("bind: permission denied"),
			},
		},
		{
			name:    "success Server Closed",
			attr:    ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 8080), Timeout: 5 * time.Second},
			wantErr: nil,
		},
		{
			name: "fail bad address",
			attr: ServerSpec{},
			wantErr: &net.AddrError{
				Err:  "missing port in address",
				Addr: "invalid IPPort",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{TFTP: tt.attr, Log: logr.Discard()}
			ctx, cancel := context.WithCancel(context.Background())
			go time.AfterFunc(time.Millisecond, cancel)
			err := c.listenAndServeTFTP(ctx)
			if !errors.Is(err, tt.wantErr) && !errors.As(err, &tt.wantErr) {
				t.Errorf("got c.listenAndServeTFTP(ctx) = %v, type: %[1]T", err)
			}
		})
	}
}

func TestServeTFTP(t *testing.T) {
	tests := []struct {
		name    string
		attr    ServerSpec
		wantErr error
	}{
		{
			name:    "success Server Closed",
			attr:    ServerSpec{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 9090)},
			wantErr: nil,
		},
		{
			name:    "fail nil conn",
			wantErr: fmt.Errorf("conn must not be nil"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{TFTP: tt.attr, Log: logr.Discard()}
			var uconn *net.UDPConn
			if tt.wantErr == nil {
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

			if !errors.Is(err, tt.wantErr) && !errors.As(err, &tt.wantErr) {
				t.Errorf("got c.serveTFTP(ctx, uconn) = %v, type: %[1]T", err)
			}
		})
	}
}
