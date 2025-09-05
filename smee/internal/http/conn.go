package http

import (
	"bufio"
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

// bufferedConn wraps a connection to allow peeking at bytes without consuming them
// Based on cmux buffer implementation
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func newBufferedConn(conn net.Conn) *bufferedConn {
	return &bufferedConn{
		Conn: conn,
		r:    bufio.NewReader(conn),
	}
}

func (bc *bufferedConn) peekFirstByte() (byte, error) {
	// Set a read deadline for protocol detection
	bc.Conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	bytes, err := bc.r.Peek(1)
	if err != nil {
		return 0, err
	}
	// Reset the read deadline
	bc.Conn.SetReadDeadline(time.Time{})

	return bytes[0], nil
}

func (bc *bufferedConn) Read(b []byte) (int, error) {
	return bc.r.Read(b)
}

// singleConnListener implements net.Listener for a single connection
// Minimal implementation matching standard patterns
type singleConnListener struct {
	conn      net.Conn
	done      chan struct{}
	accepted  bool
	ctx       context.Context // Added context for cancellation
	closeOnce sync.Once       // Ensure Close() is only called once
}

// newSingleConnListener creates a new single connection listener
// Now takes a context parameter for cancellation
func newSingleConnListener(ctx context.Context, conn net.Conn) *singleConnListener {
	return &singleConnListener{
		conn: conn,
		done: make(chan struct{}),
		ctx:  ctx, // Store the context
	}
}

/*
When the HTTP/HTTPS handler calls server.Serve(listener):

The server calls listener.Accept() to get the connection
The first call returns the actual connection
The server processes this connection in a goroutine
The server loops and calls Accept() again
On subsequent calls to Accept():

The code reaches select {} with no cases
This goroutine blocks indefinitely
This is intentional and doesn't cause problems
The goroutine running this Accept() call will only terminate if:

The entire program terminates
The parent goroutine (server) is canceled via context
The OS thread is terminated
*/
// Accept now properly handles single connection lifecycle
func (s *singleConnListener) Accept() (net.Conn, error) {
	if s.accepted {
		// After the first connection, wait for either:
		// 1. The listener to be explicitly closed via Close()
		// 2. The context to be cancelled (when connection handler finishes)
		select {
		case <-s.done:
			return nil, errors.New("listener closed")
		case <-s.ctx.Done():
			return nil, errors.New("context cancelled")
		}
	}
	s.accepted = true

	// Wrap the connection to automatically close the listener when the connection closes
	return &connWrapper{
		Conn:     s.conn,
		listener: s,
	}, nil
}

// connWrapper wraps a connection to notify the listener when it closes
type connWrapper struct {
	net.Conn
	listener *singleConnListener
	closed   bool
}

func (c *connWrapper) Close() error {
	if !c.closed {
		c.closed = true
		// Close the original connection
		err := c.Conn.Close()
		// Signal the listener to stop accepting more connections
		c.listener.Close()
		return err
	}
	return nil
}

func (s *singleConnListener) Close() error {
	// No-op since we don't own the connection lifecycle.
	// Use sync.Once to ensure the channel is only closed once
	s.closeOnce.Do(func() {
		close(s.done)
	})
	return nil
}

func (s *singleConnListener) Addr() net.Addr {
	if s.conn != nil {
		return s.conn.LocalAddr()
	}
	return nil
}
