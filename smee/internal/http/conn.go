package http

import (
	"bufio"
	"errors"
	"net"
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
	conn     net.Conn
	done     chan struct{}
	accepted bool
}

// newSingleConnListener creates a new single connection listener
func newSingleConnListener(conn net.Conn) *singleConnListener {
	return &singleConnListener{
		conn: conn,
		done: make(chan struct{}),
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
func (s *singleConnListener) Accept() (net.Conn, error) {
	if s.accepted {
		<-s.done // Block until closed
		return nil, errors.New("listener closed")
	}
	s.accepted = true
	return s.conn, nil
}

func (s *singleConnListener) Close() error {
	// No-op since we don't own the connection lifecycle.
	close(s.done)
	return nil
}

func (s *singleConnListener) Addr() net.Addr {
	if s.conn != nil {
		return s.conn.LocalAddr()
	}
	return nil
}
