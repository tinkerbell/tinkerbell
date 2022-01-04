package ipxedust

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
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
			got := &Server{
				TFTP:                 tt.tftp,
				HTTP:                 tt.http,
				EnableTFTPSinglePort: true,
			}
			ctx, cn := context.WithCancel(context.Background())

			var err error
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				err = got.ListenAndServe(ctx)

				wg.Done()
			}()
			cn()
			wg.Wait()

			switch {
			case tt.wantErr == nil && err != nil:
				t.Errorf("expected nil error, got: %T", err)
			case tt.wantErr != nil && err == nil:
				t.Errorf("expected error, got: nil")
			case tt.wantErr != nil && err != nil:
				if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
					t.Fatal(diff)
				}
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
			wantErr: nil,
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
			got := &Server{
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

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				err = got.Serve(ctx, conn, uconn)
				wg.Done()
			}()
			cn()
			wg.Wait()

			switch {
			case tt.wantErr == nil && err != nil:
				t.Errorf("expected nil error, got: %T", err)
			case tt.wantErr != nil && err == nil:
				t.Errorf("expected error, got: nil")
			case tt.wantErr != nil && err != nil:
				if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
					t.Fatal(diff)
				}
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
			errChan := make(chan error, 1)
			go func() {
				errChan <- c.listenAndServeHTTP(ctx)
			}()
			cancel()
			err := <-errChan

			switch {
			case tt.wantErr == nil && err != nil:
				t.Errorf("expected nil error, got: %T", err)
			case tt.wantErr != nil && err == nil:
				t.Errorf("expected error, got: nil")
			case tt.wantErr != nil && err != nil:
				if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}

func TestServeHTTP(t *testing.T) {
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
			wantErr: fmt.Errorf("listener must not be nil"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Server{TFTP: tt.attr, Log: logr.Discard()}
			var conn net.Listener
			var err error
			if tt.wantErr == nil {
				conn, err = net.Listen("tcp", tt.attr.Addr.String())
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			errChan := make(chan error, 1)
			go func() {
				errChan <- c.serveHTTP(ctx, conn)
			}()
			cancel()
			err = <-errChan

			switch {
			case tt.wantErr == nil && err != nil:
				t.Errorf("expected nil error, got: %T", err)
			case tt.wantErr != nil && err == nil:
				t.Errorf("expected error, got: nil")
			case tt.wantErr != nil && err != nil:
				if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
					t.Fatal(diff)
				}
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
			errChan := make(chan error, 1)
			go func() {
				errChan <- c.listenAndServeTFTP(ctx)
			}()
			cancel()
			err := <-errChan

			switch {
			case tt.wantErr == nil && err != nil:
				t.Errorf("expected nil error, got: %T", err)
			case tt.wantErr != nil && err == nil:
				t.Errorf("expected error, got: nil")
			case tt.wantErr != nil && err != nil:
				if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
					t.Fatal(diff)
				}
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
			errChan := make(chan error, 1)
			go func() {
				errChan <- c.serveTFTP(ctx, uconn)
			}()
			cancel()
			err := <-errChan

			switch {
			case tt.wantErr == nil && err != nil:
				t.Errorf("expected nil error, got: %T", err)
			case tt.wantErr != nil && err == nil:
				t.Errorf("expected error, got: nil")
			case tt.wantErr != nil && err != nil:
				if diff := cmp.Diff(err.Error(), tt.wantErr.Error()); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}
