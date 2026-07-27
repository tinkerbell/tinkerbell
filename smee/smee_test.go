package smee

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestRunSyslogServer(t *testing.T) {
	// Grab a free UDP port, then release it so the receiver can bind to it.
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("reserve udp port: %v", err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runSyslogServer(ctx, logr.Discard(), addr)
	}()

	// Give the receiver a moment to bind, then cancel to trigger a clean stop.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("runSyslogServer() returned error on clean shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runSyslogServer() did not return after context cancellation")
	}
}

func TestRunSyslogServer_startError(t *testing.T) {
	// An unparseable bind address makes StartReceiver fail, so runSyslogServer
	// should surface the error rather than block.
	err := runSyslogServer(context.Background(), logr.Discard(), "not-a-valid-address")
	if err == nil {
		t.Error("runSyslogServer() expected error for invalid bind address, got nil")
	}
}
