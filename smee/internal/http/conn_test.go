package http

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// TestBufferedConnComprehensive tests the bufferedConn implementation comprehensively
func TestBufferedConnComprehensive(t *testing.T) {
	tests := map[string]struct {
		setupConn   func() *mockConn
		testFunc    func(t *testing.T, bc *bufferedConn)
		expectError bool
	}{
		"NewBufferedConn": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("test data"))
			},
			testFunc: func(t *testing.T, bc *bufferedConn) {
				if bc == nil {
					t.Fatal("newBufferedConn returned nil")
				}
				if bc.r == nil {
					t.Error("bufferedConn.r should be initialized")
				}
			},
		},
		"PeekFirstByte_Success": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("GET / HTTP/1.1\r\n\r\n"))
			},
			testFunc: func(t *testing.T, bc *bufferedConn) {
				firstByte, err := bc.peekFirstByte()
				if err != nil {
					t.Fatalf("peekFirstByte failed: %v", err)
				}
				if firstByte != 'G' {
					t.Errorf("Expected first byte 'G', got %q", firstByte)
				}
				// Verify that peeking doesn't consume the data
				data := make([]byte, 3)
				n, err := bc.Read(data)
				if err != nil {
					t.Fatalf("Read after peek failed: %v", err)
				}
				if n != 3 || string(data) != "GET" {
					t.Errorf("Expected to read 'GET', got %q", string(data[:n]))
				}
			},
		},
		"PeekFirstByte_TLS": {
			setupConn: func() *mockConn {
				return newMockConn([]byte{0x16, 0x03, 0x01, 0x00, 0x01})
			},
			testFunc: func(t *testing.T, bc *bufferedConn) {
				firstByte, err := bc.peekFirstByte()
				if err != nil {
					t.Fatalf("peekFirstByte failed: %v", err)
				}
				if firstByte != 0x16 {
					t.Errorf("Expected first byte 0x16, got 0x%02x", firstByte)
				}
			},
		},
		"PeekFirstByte_EmptyData": {
			setupConn: func() *mockConn {
				return newMockConn([]byte{})
			},
			testFunc: func(t *testing.T, bc *bufferedConn) {
				_, err := bc.peekFirstByte()
				if err != io.EOF {
					t.Errorf("Expected EOF error, got %v", err)
				}
			},
			expectError: true,
		},
		"PeekFirstByte_ReadError": {
			setupConn: func() *mockConn {
				conn := newMockConn([]byte("test"))
				conn.SetReadError(io.ErrUnexpectedEOF)
				return conn
			},
			testFunc: func(t *testing.T, bc *bufferedConn) {
				_, err := bc.peekFirstByte()
				if err != io.ErrUnexpectedEOF {
					t.Errorf("Expected ErrUnexpectedEOF, got %v", err)
				}
			},
			expectError: true,
		},
		"Read_Success": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("Hello, World!"))
			},
			testFunc: func(t *testing.T, bc *bufferedConn) {
				testData := []byte("Hello, World!")
				buffer := make([]byte, len(testData))
				n, err := bc.Read(buffer)
				if err != nil {
					t.Fatalf("Read failed: %v", err)
				}
				if n != len(testData) {
					t.Errorf("Expected to read %d bytes, got %d", len(testData), n)
				}
				if !bytes.Equal(buffer, testData) {
					t.Errorf("Expected %q, got %q", string(testData), string(buffer))
				}
			},
		},
		"Read_AfterPeek": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("ABCDEFGH"))
			},
			testFunc: func(t *testing.T, bc *bufferedConn) {
				testData := []byte("ABCDEFGH")
				// Peek first byte
				firstByte, err := bc.peekFirstByte()
				if err != nil {
					t.Fatalf("peekFirstByte failed: %v", err)
				}
				if firstByte != 'A' {
					t.Errorf("Expected 'A', got %q", firstByte)
				}
				// Read all data
				buffer := make([]byte, len(testData))
				n, err := bc.Read(buffer)
				if err != nil {
					t.Fatalf("Read failed: %v", err)
				}
				if n != len(testData) || !bytes.Equal(buffer, testData) {
					t.Errorf("Expected %q, got %q", string(testData), string(buffer[:n]))
				}
			},
		},
		"Write_PassThrough": {
			setupConn: func() *mockConn {
				return newMockConn([]byte{})
			},
			testFunc: func(t *testing.T, bc *bufferedConn) {
				testData := []byte("test write data")
				n, err := bc.Write(testData)
				if err != nil {
					t.Fatalf("Write failed: %v", err)
				}
				if n != len(testData) {
					t.Errorf("Expected to write %d bytes, got %d", len(testData), n)
				}
				mockConn := bc.Conn.(*mockConn)
				writtenData := mockConn.GetWrittenData()
				if !bytes.Equal(writtenData, testData) {
					t.Errorf("Expected %q, got %q", string(testData), string(writtenData))
				}
			},
		},
		"SetDeadline": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("test"))
			},
			testFunc: func(t *testing.T, bc *bufferedConn) {
				deadline := time.Now().Add(5 * time.Second)
				err := bc.SetReadDeadline(deadline)
				if err != nil {
					t.Errorf("SetReadDeadline failed: %v", err)
				}
				mockConn := bc.Conn.(*mockConn)
				if !mockConn.readDeadline.Equal(deadline) {
					t.Errorf("Read deadline not set correctly")
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mockConn := tt.setupConn()
			bc := newBufferedConn(mockConn)

			if bc.Conn != mockConn {
				t.Error("bufferedConn.Conn should be the same as input conn")
			}

			tt.testFunc(t, bc)
		})
	}
}

// TestSingleConnListenerComprehensive tests the singleConnListener implementation
func TestSingleConnListenerComprehensive(t *testing.T) {
	tests := map[string]struct {
		setupConn   func() *mockConn
		testFunc    func(t *testing.T, listener *singleConnListener)
		expectError bool
	}{
		"Accept_FirstCall": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("test"))
			},
			testFunc: func(t *testing.T, listener *singleConnListener) {
				conn, err := listener.Accept()
				if err != nil {
					t.Fatalf("Accept failed: %v", err)
				}
				mockConn := listener.conn.(*mockConn)
				if conn != mockConn {
					t.Error("Accept should return the same connection")
				}
			},
		},
		"Accept_SecondCall": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("test"))
			},
			testFunc: func(t *testing.T, listener *singleConnListener) {
				// First call should succeed
				mockConn := listener.conn.(*mockConn)
				conn1, err := listener.Accept()
				if err != nil {
					t.Fatalf("First Accept failed: %v", err)
				}
				if conn1 != mockConn {
					t.Error("First Accept should return the connection")
				}
				// Close before second Accept to unblock
				listener.Close()
				conn2, err := listener.Accept()
				if err == nil {
					t.Errorf("Second Accept should return error after Close, got nil")
				}
				if err != nil && err.Error() != "listener closed" {
					t.Errorf("Second Accept should return 'listener closed', got %v", err)
				}
				if conn2 != nil {
					t.Error("Second Accept should return nil connection")
				}
			},
		},
		"Accept_Concurrent": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("test"))
			},
			testFunc: func(t *testing.T, listener *singleConnListener) {
				const numGoroutines = 10
				results := make(chan net.Conn, numGoroutines)
				errors := make(chan error, numGoroutines)
				var wg sync.WaitGroup

				// Get one successful accept first
				firstConn, firstErr := listener.Accept()
				if firstErr != nil {
					t.Fatalf("First Accept failed: %v", firstErr)
				}

				// Now close the listener
				listener.Close()

				// Start remaining goroutines that should all get "listener closed" errors
				for i := 0; i < numGoroutines; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						conn, err := listener.Accept()
						results <- conn
						errors <- err
					}()
				}

				wg.Wait()
				close(results)
				close(errors)

				// Check that all goroutines got the proper error
				mockConn := listener.conn.(*mockConn)
				closedCount := 0

				for i := 0; i < numGoroutines; i++ {
					conn := <-results
					err := <-errors

					if err != nil && err.Error() == "listener closed" && conn == nil {
						closedCount++
					} else {
						t.Errorf("Unexpected result: conn=%v, err=%v", conn, err)
					}
				}

				// All goroutines should get "listener closed" errors
				if closedCount != numGoroutines {
					t.Errorf("Expected %d 'listener closed' errors, got %d", numGoroutines, closedCount)
				}

				// Verify the first connection was valid
				if firstConn != mockConn {
					t.Error("First Accept should return the connection")
				}
			},
		},
		"Close": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("test"))
			},
			testFunc: func(t *testing.T, listener *singleConnListener) {
				err := listener.Close()
				if err != nil {
					t.Errorf("Close should not return error, got %v", err)
				}
				// Close should be a no-op, connection should still work
				mockConn := listener.conn.(*mockConn)
				conn, err := listener.Accept()
				if err == nil {
					if conn != mockConn {
						t.Error("Accept after Close should still return connection if not yet accepted")
					}
				} else {
					if conn != nil {
						t.Error("Accept after Close should return nil connection on error")
					}
				}
			},
		},
		"Addr": {
			setupConn: func() *mockConn {
				return newMockConn([]byte("test"))
			},
			testFunc: func(t *testing.T, listener *singleConnListener) {
				mockConn := listener.conn.(*mockConn)
				addr := listener.Addr()
				if addr != mockConn.LocalAddr() {
					t.Errorf("Addr should return connection's LocalAddr, got %v", addr)
				}
			},
		},
		"Addr_NilConn": {
			setupConn: func() *mockConn {
				return nil
			},
			testFunc: func(t *testing.T, listener *singleConnListener) {
				addr := listener.Addr()
				if addr != nil {
					t.Errorf("Addr with nil connection should return nil, got %v", addr)
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var listener *singleConnListener
			if tt.setupConn() != nil {
				listener = newSingleConnListener(tt.setupConn())
			} else {
				listener = newSingleConnListener(nil)
			}

			tt.testFunc(t, listener)
		})
	}
}

// TestIntegration tests the interaction between bufferedConn and singleConnListener
func TestIntegration(t *testing.T) {
	tests := map[string]struct {
		data         []byte
		expectedByte byte
		expectTLS    bool
		testFunc     func(t *testing.T, mockConn *mockConn, bc *bufferedConn, expectedByte byte)
	}{
		"BufferedConn_With_SingleConnListener": {
			data:         []byte("GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			expectedByte: 'G',
			expectTLS:    false,
			testFunc: func(t *testing.T, mockConn *mockConn, bc *bufferedConn, expectedByte byte) {
				// Should detect HTTP (not TLS)
				if expectedByte == 0x16 {
					t.Error("Should not detect TLS for HTTP request")
				}
				if expectedByte != 'G' {
					t.Errorf("Expected HTTP GET request, got byte 0x%02x", expectedByte)
				}

				// Create single connection listener
				listener := newSingleConnListener(bc)

				// Accept the connection
				acceptedConn, err := listener.Accept()
				if err != nil {
					t.Fatalf("Accept failed: %v", err)
				}

				if acceptedConn != bc {
					t.Error("Accepted connection should be the buffered connection")
				}

				// Read the request through the accepted connection
				httpRequest := []byte("GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n")
				buffer := make([]byte, len(httpRequest))
				n, err := acceptedConn.Read(buffer)
				if err != nil {
					t.Fatalf("Reading request failed: %v", err)
				}

				if n != len(httpRequest) || !bytes.Equal(buffer, httpRequest) {
					t.Errorf("Request data mismatch. Expected %q, got %q", string(httpRequest), string(buffer[:n]))
				}
			},
		},
		"TLS_Detection": {
			data:         []byte{0x16, 0x03, 0x01, 0x01, 0x00}, // TLS 1.0 Client Hello
			expectedByte: 0x16,
			expectTLS:    true,
			testFunc: func(t *testing.T, mockConn *mockConn, bc *bufferedConn, expectedByte byte) {
				if expectedByte != 0x16 {
					t.Errorf("Expected TLS handshake byte 0x16, got 0x%02x", expectedByte)
				}
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Simulate the protocol detection flow
			mockConn := newMockConn(tt.data)

			// Create buffered connection for protocol detection
			bc := newBufferedConn(mockConn)

			// Peek to detect protocol
			firstByte, err := bc.peekFirstByte()
			if err != nil {
				t.Fatalf("Protocol detection failed: %v", err)
			}

			if firstByte != tt.expectedByte {
				t.Errorf("Expected byte 0x%02x, got 0x%02x", tt.expectedByte, firstByte)
			}

			tt.testFunc(t, mockConn, bc, firstByte)
		})
	}
}

// BenchmarkBufferedConn benchmarks the bufferedConn performance
func BenchmarkBufferedConn(b *testing.B) {
	testData := bytes.Repeat([]byte("test data "), 1000)

	b.Run("PeekFirstByte", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mockConn := newMockConn(testData)
			bc := newBufferedConn(mockConn)

			_, err := bc.peekFirstByte()
			if err != nil {
				b.Fatalf("peekFirstByte failed: %v", err)
			}
		}
	})

	b.Run("Read", func(b *testing.B) {
		buffer := make([]byte, 1024)

		for i := 0; i < b.N; i++ {
			mockConn := newMockConn(testData)
			bc := newBufferedConn(mockConn)

			for {
				_, err := bc.Read(buffer)
				if err == io.EOF {
					break
				}
				if err != nil {
					b.Fatalf("Read failed: %v", err)
				}
			}
		}
	})
}

// BenchmarkSingleConnListener benchmarks the singleConnListener performance
func BenchmarkSingleConnListener(b *testing.B) {
	b.Run("Accept", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			mockConn := newMockConn([]byte("test"))
			listener := newSingleConnListener(mockConn)

			_, err := listener.Accept()
			if err != nil {
				b.Fatalf("Accept failed: %v", err)
			}
		}
	})
}

// mockConn implements net.Conn for testing
type mockConn struct {
	readData     []byte
	readIndex    int
	writeData    *bytes.Buffer
	closed       bool
	readDeadline time.Time
	localAddr    net.Addr
	remoteAddr   net.Addr
	readError    error
	writeError   error
	mu           sync.Mutex
}

func newMockConn(data []byte) *mockConn {
	return &mockConn{
		readData:   data,
		writeData:  &bytes.Buffer{},
		localAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080},
		remoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345},
	}
}

func (mc *mockConn) Read(b []byte) (int, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return 0, io.EOF
	}

	if mc.readError != nil {
		return 0, mc.readError
	}

	if mc.readIndex >= len(mc.readData) {
		return 0, io.EOF
	}

	n := copy(b, mc.readData[mc.readIndex:])
	mc.readIndex += n
	return n, nil
}

func (mc *mockConn) Write(b []byte) (int, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return 0, io.ErrClosedPipe
	}

	if mc.writeError != nil {
		return 0, mc.writeError
	}

	return mc.writeData.Write(b)
}

func (mc *mockConn) Close() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.closed = true
	return nil
}

func (mc *mockConn) LocalAddr() net.Addr  { return mc.localAddr }
func (mc *mockConn) RemoteAddr() net.Addr { return mc.remoteAddr }

func (mc *mockConn) SetDeadline(t time.Time) error {
	return mc.SetReadDeadline(t)
}

func (mc *mockConn) SetReadDeadline(t time.Time) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.readDeadline = t
	return nil
}

func (mc *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (mc *mockConn) GetWrittenData() []byte {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.writeData.Bytes()
}

func (mc *mockConn) SetReadError(err error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.readError = err
}

func (mc *mockConn) SetWriteError(err error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.writeError = err
}
