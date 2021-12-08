package ipxe

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"inet.af/netaddr"
)

func TestServe(t *testing.T) {
	want := Config{
		TFTP: Server{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 69), Timeout: 5 * time.Second},
		HTTP: Server{Addr: netaddr.IPPortFrom(netaddr.IPv4(0, 0, 0, 0), 8080), Timeout: 5 * time.Second},
		Log:  logr.Discard(),
	}
	got := &Config{}
	err := got.Serve(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if diff := cmp.Diff(*got, want, cmpopts.IgnoreUnexported(netaddr.IPPort{}, logr.Logger{})); diff != "" {
		t.Fatal(diff)
	}

}
