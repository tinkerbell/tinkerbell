package ipxe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"inet.af/netaddr"
)

func TestServe(t *testing.T) {
	tests := []struct {
		name    string
		tftp    Attribute
		http    Attribute
		wantErr []error
	}{
		{
			name: "success",
			tftp: Attribute{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 6969), Timeout: 5 * time.Second},
			wantErr: []error{&net.OpError{
				Op:   "listen",
				Net:  "udp",
				Addr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 69},
				Err:  fmt.Errorf("bind: permission denied"),
			}, http.ErrServerClosed},
		},
		{
			name: "fail",
			tftp: Attribute{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 69), Timeout: 5 * time.Second},
			wantErr: []error{http.ErrServerClosed, &net.OpError{
				Op:   "listen",
				Net:  "udp",
				Addr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 69},
				Err:  fmt.Errorf("bind: permission denied"),
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := &Config{
				TFTP: tt.tftp,
			}
			ctx, cn := context.WithCancel(context.Background())

			var err error
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				err = got.ListenAndServe(ctx)
				t.Log("goroutine", err)
				wg.Done()
			}()
			cn()
			wg.Wait()

			if err == nil {
				t.Fatal("expected error, got: nil")
			}
			var atLeastOne bool
			var diff string
			for _, e := range tt.wantErr {
				if d := cmp.Diff(err.Error(), e.Error()); d != "" {
					diff = d
				}
				atLeastOne = true
			}
			if !atLeastOne {
				t.Fatal(diff)
			}

		})
	}
}

func TestServeHTTP(t *testing.T) {
	tests := []struct {
		name    string
		attr    Attribute
		wantErr error
	}{
		{
			name: "fail net.OpError",
			attr: Attribute{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 80), Timeout: 5 * time.Second},
			wantErr: &net.OpError{
				Op:   "listen",
				Net:  "tcp",
				Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80},
				Err:  fmt.Errorf("bind: permission denied"),
			},
		},
		{
			name:    "success Server Closed",
			attr:    Attribute{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 8080), Timeout: 5 * time.Second},
			wantErr: http.ErrServerClosed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{HTTP: tt.attr, Log: logr.Discard()}
			ctx, cancel := context.WithCancel(context.Background())
			errChan := make(chan error, 1)
			go func() {
				errChan <- c.serveHTTP(ctx)
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
		attr    Attribute
		wantErr error
	}{
		{
			name: "fail net.OpError",
			attr: Attribute{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 80), Timeout: 5 * time.Second},
			wantErr: &net.OpError{
				Op:   "listen",
				Net:  "udp",
				Addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80},
				Err:  fmt.Errorf("bind: permission denied"),
			},
		},
		{
			name:    "success Server Closed",
			attr:    Attribute{Addr: netaddr.IPPortFrom(netaddr.IPv4(127, 0, 0, 1), 8080), Timeout: 5 * time.Second},
			wantErr: nil,
		},
		{
			name: "fail bad address",
			attr: Attribute{},
			wantErr: &net.AddrError{
				Err:  "missing port in address",
				Addr: "invalid IPPort",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{TFTP: tt.attr, Log: logr.Discard()}
			ctx, cancel := context.WithCancel(context.Background())
			errChan := make(chan error, 1)
			go func() {
				errChan <- c.serveTFTP(ctx)
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
