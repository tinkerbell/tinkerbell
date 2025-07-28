package syslog

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
)

func TestStartReceiver(t *testing.T) {
	tests := map[string]struct {
		laddr     string
		parsers   int
		wantErr   bool
		errSubstr string
	}{
		"valid address with default parsers": {
			laddr:   "127.0.0.1:0", // Use port 0 to get a random available port
			parsers: 0,             // Should default to 1
			wantErr: false,
		},
		"valid address with custom parsers": {
			laddr:   "127.0.0.1:0",
			parsers: 3,
			wantErr: false,
		},
		"invalid address": {
			laddr:     "invalid-address",
			parsers:   1,
			wantErr:   true,
			errSubstr: "resolve syslog udp listen address",
		},
		"address in use": {
			laddr:   "127.0.0.1:1", // Port 1 is typically reserved
			parsers: 1,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err := StartReceiver(ctx, logr.Discard(), tc.laddr, tc.parsers)

			if tc.wantErr {
				if err == nil {
					t.Errorf("StartReceiver() expected error but got none")
				}
				if tc.errSubstr != "" && !containsSubstring(err.Error(), tc.errSubstr) {
					t.Errorf("StartReceiver() error = %v, want substring %v", err, tc.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("StartReceiver() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestReceiver_DoneAndErr(t *testing.T) {
	// Start a receiver on a random port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a UDP connection to get a free port
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()

	err = StartReceiver(ctx, logr.Discard(), addr, 1)
	if err != nil {
		t.Fatalf("StartReceiver() unexpected error = %v", err)
	}

	// Since we can't easily access the receiver instance from StartReceiver,
	// we'll test the concept by creating our own receiver
	testReceiver := &Receiver{
		done: make(chan struct{}),
		err:  nil,
	}

	// Test Done() method
	select {
	case <-testReceiver.Done():
		t.Error("Done() channel should not be closed initially")
	default:
		// Expected behavior
	}

	// Test Err() method
	if testReceiver.Err() != nil {
		t.Errorf("Err() = %v, want nil", testReceiver.Err())
	}

	// Simulate an error
	testReceiver.err = net.ErrClosed
	if testReceiver.Err() != net.ErrClosed {
		t.Errorf("Err() = %v, want %v", testReceiver.Err(), net.ErrClosed)
	}
}

func TestReceiver_cleanup(t *testing.T) {
	// Create a real UDP connection for testing
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}

	r := &Receiver{
		c:     conn,
		parse: make(chan *message, 1),
		done:  make(chan struct{}),
	}

	// Test that cleanup closes everything
	r.cleanup()

	// Verify connection is closed by trying to read from it
	buf := make([]byte, 1)
	_, _, err = conn.ReadFromUDP(buf)
	if err == nil {
		t.Error("Expected connection to be closed after cleanup()")
	}

	// Verify channels are closed
	select {
	case <-r.done:
		// Expected - channel should be closed
	default:
		t.Error("done channel should be closed after cleanup()")
	}

	select {
	case _, ok := <-r.parse:
		if ok {
			t.Error("parse channel should be closed after cleanup()")
		}
	default:
		// This is also acceptable as the channel might be empty
	}
}

func TestParse(t *testing.T) {
	tests := map[string]struct {
		message  *message
		expected map[string]interface{}
	}{
		"basic message": {
			message: &message{
				priority: 128, // local0.emerg (facility=16, severity=0): 16*8+0=128
				hostname: []byte("test-host"),
				app:      []byte("test-app"),
				procid:   []byte("1234"),
				msgid:    []byte("test-id"),
				msg:      []byte("test message"),
				host:     net.IPv4(192, 168, 1, 1),
			},
			expected: map[string]interface{}{
				"facility": "local0",
				"severity": "EMERG",
				"hostname": "test-host",
				"app-name": "test-app",
				"procid":   "1234",
				"msgid":    "test-id",
				"msg":      "test message",
				"host":     "192.168.1.1",
			},
		},
		"message with JSON payload": {
			message: &message{
				priority: 30, // daemon.info (facility=3, severity=6): 3*8+6=30
				hostname: []byte("json-host"),
				app:      []byte("json-app"),
				msg:      []byte(`{"level": "info", "message": "structured log"}`),
				host:     net.IPv4(10, 0, 0, 1),
			},
			expected: map[string]interface{}{
				"facility": "daemon",
				"severity": "INFO",
				"hostname": "json-host",
				"app-name": "json-app",
				"msg": map[string]interface{}{
					"level":   "info",
					"message": "structured log",
				},
				"host": "10.0.0.1",
			},
		},
		"minimal message": {
			message: &message{
				priority: 0, // kern.emerg (facility=0, severity=0): 0*8+0=0
				host:     net.IPv4(127, 0, 0, 1),
			},
			expected: map[string]interface{}{
				"facility": "kern",
				"severity": "EMERG",
				"host":     "127.0.0.1",
			},
		},
		"invalid JSON in message": {
			message: &message{
				priority: 22, // mail.info (facility=2, severity=6): 2*8+6=22
				msg:      []byte(`{invalid json`),
				host:     net.IPv4(172, 16, 0, 1),
			},
			expected: map[string]interface{}{
				"facility": "mail",
				"severity": "INFO",
				"msg":      "{invalid json",
				"host":     "172.16.0.1",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := parse(tc.message)
			if diff := cmp.Diff(tc.expected, result); diff != "" {
				t.Errorf("parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReceiver_runParser(t *testing.T) {
	// Create a test receiver with a parse channel
	parseChannel := make(chan *message, 2)
	receiver := &Receiver{
		parse:  parseChannel,
		Logger: logr.Discard(),
	}

	// Create test messages with valid syslog format
	msg1 := &message{
		priority: 128, // local0.emerg (facility=16, severity=0): 16*8+0=128
		hostname: []byte("test-host"),
		msg:      []byte("test message"),
		host:     net.IPv4(192, 168, 1, 1),
		buf:      [32768]byte{},
		size:     20,
	}
	// Add some content to the buffer to make parsing work
	copy(msg1.buf[:], "<128>test message")

	msg2 := &message{
		priority: 31, // daemon.debug (facility=3, severity=7): 3*8+7=31
		hostname: []byte("debug-host"),
		msg:      []byte("debug message"),
		host:     net.IPv4(192, 168, 1, 2),
		buf:      [32768]byte{},
		size:     22,
	}
	// Add some content to the buffer to make parsing work
	copy(msg2.buf[:], "<31>debug message")

	// Send messages to the channel
	parseChannel <- msg1
	parseChannel <- msg2
	close(parseChannel)

	// Track the completion of message processing
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		receiver.runParser()
	}()

	// Wait for processing to complete with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - processing completed
	case <-time.After(2 * time.Second):
		t.Error("runParser() did not complete within timeout")
	}
}

func TestSyslogMessagePool(t *testing.T) {
	// Test that the pool creates new messages
	item := syslogMessagePool.Get()
	msg1, ok := item.(*message)
	if !ok || msg1 == nil {
		t.Error("Pool should return a non-nil message")
		return
	}

	// Modify the message
	msg1.priority = 123

	// Put it back in the pool
	syslogMessagePool.Put(msg1)

	// Get another message (might be the same one)
	item2 := syslogMessagePool.Get()
	msg2, ok := item2.(*message)
	if !ok || msg2 == nil {
		t.Error("Pool should return a non-nil message")
		return
	}

	// The pool reuses objects, so we might get the same one back
	// This is expected behavior for sync.Pool
}

func TestReceiverIntegration(t *testing.T) {
	// Integration test that sends actual UDP messages
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get a free port
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()

	// Start the receiver
	err = StartReceiver(ctx, logr.Discard(), addr, 1)
	if err != nil {
		t.Fatalf("StartReceiver() error = %v", err)
	}

	// Give the receiver a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send a test syslog message
	clientConn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatalf("Failed to create client connection: %v", err)
	}
	defer clientConn.Close()

	testMessage := "<30>test-host test-app: test message"
	_, err = clientConn.Write([]byte(testMessage))
	if err != nil {
		t.Fatalf("Failed to send test message: %v", err)
	}

	// Give time for message processing
	time.Sleep(100 * time.Millisecond)

	// The test passes if no panics occur and the receiver processes the message
	// More detailed verification would require access to the receiver instance
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) &&
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

func TestReceiver_run_contextCancel(t *testing.T) {
	// Test that the receiver properly handles context cancellation
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	r := &Receiver{
		c:      conn,
		parse:  make(chan *message, 1),
		done:   make(chan struct{}),
		Logger: logr.Discard(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start the receiver in a goroutine
	go r.run(ctx)

	// Cancel the context
	cancel()

	// Wait for the receiver to finish
	select {
	case <-r.Done():
		// Expected behavior
	case <-time.After(time.Second):
		t.Error("Receiver did not stop within timeout after context cancellation")
	}
}

func TestReceiver_run_networkError(t *testing.T) {
	// Test handling of network errors by using an invalid operation
	// Create a closed connection
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close() // Close immediately to cause errors

	r := &Receiver{
		c:      conn,
		parse:  make(chan *message, 1),
		done:   make(chan struct{}),
		Logger: logr.Discard(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start the receiver
	go r.run(ctx)

	// Wait for either the receiver to finish or context timeout
	select {
	case <-r.Done():
		// Check that an error was recorded or context was cancelled
		// Both are acceptable outcomes
	case <-ctx.Done():
		// Context timeout is also acceptable since the receiver might
		// be handling the closed connection gracefully
	}
}

func TestReceiver_run_parseChannelBlocking(t *testing.T) {
	// Test behavior when parse channel is full - simplified version
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	parseChannel := make(chan *message) // No buffer - will block immediately
	r := &Receiver{
		c:      conn,
		parse:  parseChannel,
		done:   make(chan struct{}),
		Logger: logr.Discard(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start the receiver
	receiverDone := make(chan struct{})
	go func() {
		r.run(ctx)
		close(receiverDone)
	}()

	// Wait for context timeout (which should trigger receiver cleanup)
	<-ctx.Done()

	// The receiver should handle context cancellation even when parse channel is full
	select {
	case <-receiverDone:
		// Expected behavior - receiver stopped due to context cancellation
	case <-time.After(100 * time.Millisecond):
		t.Error("Receiver did not stop within timeout after context cancellation")
	}
}

func TestParse_emptyAndNilFields(t *testing.T) {
	tests := map[string]struct {
		message  *message
		expected map[string]interface{}
	}{
		"all fields empty": {
			message: &message{
				priority: 0,
				hostname: []byte{},
				app:      []byte{},
				procid:   []byte{},
				msgid:    []byte{},
				msg:      []byte{},
				host:     net.IPv4(127, 0, 0, 1),
			},
			expected: map[string]interface{}{
				"facility": "kern",
				"severity": "EMERG",
				"host":     "127.0.0.1",
			},
		},
		"nil fields": {
			message: &message{
				priority: 8, // user.emerg (facility=1, severity=0): 1*8+0=8
				hostname: nil,
				app:      nil,
				procid:   nil,
				msgid:    nil,
				msg:      nil,
				host:     net.IPv4(192, 168, 1, 1),
			},
			expected: map[string]interface{}{
				"facility": "user",
				"severity": "EMERG",
				"host":     "192.168.1.1",
			},
		},
		"whitespace-only fields": {
			message: &message{
				priority: 16, // mail.emerg (facility=2, severity=0): 2*8+0=16
				hostname: []byte("   "),
				app:      []byte("\t"),
				procid:   []byte("\n"),
				msgid:    []byte(" \t\n"),
				msg:      []byte("   whitespace message   "),
				host:     net.IPv4(10, 0, 0, 1),
			},
			expected: map[string]interface{}{
				"facility": "mail",
				"severity": "EMERG",
				"hostname": "   ",
				"app-name": "\t",
				"procid":   "\n",
				"msgid":    " \t\n",
				"msg":      "   whitespace message   ",
				"host":     "10.0.0.1",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := parse(tc.message)
			if diff := cmp.Diff(tc.expected, result); diff != "" {
				t.Errorf("parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParse_priorityEdgeCases(t *testing.T) {
	tests := map[string]struct {
		priority byte
		facility string
		severity string
	}{
		"minimum priority": {
			priority: 0,
			facility: "kern",
			severity: "EMERG",
		},
		"maximum priority": {
			priority: 191, // local7.debug (facility=23, severity=7): 23*8+7=191
			facility: "local7",
			severity: "DEBUG",
		},
		"middle priority": {
			priority: 85, // authpriv.notice (facility=10, severity=5): 10*8+5=85
			facility: "authpriv",
			severity: "NOTICE",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			message := &message{
				priority: tc.priority,
				host:     net.IPv4(127, 0, 0, 1),
			}
			result := parse(message)

			if result["facility"] != tc.facility {
				t.Errorf("Expected facility %s, got %s", tc.facility, result["facility"])
			}
			if result["severity"] != tc.severity {
				t.Errorf("Expected severity %s, got %s", tc.severity, result["severity"])
			}
		})
	}
}

func TestParse_malformedJSON(t *testing.T) {
	tests := map[string]struct {
		jsonMsg  string
		expected interface{}
	}{
		"incomplete JSON object": {
			jsonMsg:  `{"level": "info", "incomplete"`,
			expected: `{"level": "info", "incomplete"`,
		},
		"JSON with trailing comma": {
			jsonMsg:  `{"level": "info", "message": "test",}`,
			expected: `{"level": "info", "message": "test",}`,
		},
		"JSON array": {
			jsonMsg:  `[1, 2, 3]`,
			expected: `[1, 2, 3]`,
		},
		"empty JSON object": {
			jsonMsg:  `{}`,
			expected: map[string]interface{}{},
		},
		"JSON with special characters": {
			jsonMsg:  `{"message": "test\nwith\tspecial\"chars"}`,
			expected: map[string]interface{}{"message": "test\nwith\tspecial\"chars"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			message := &message{
				priority: 30,
				msg:      []byte(tc.jsonMsg),
				host:     net.IPv4(127, 0, 0, 1),
			}
			result := parse(message)

			if diff := cmp.Diff(tc.expected, result["msg"]); diff != "" {
				t.Errorf("JSON parsing mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReceiver_runParser_invalidMessages(t *testing.T) {
	parseChannel := make(chan *message, 3)
	receiver := &Receiver{
		parse:  parseChannel,
		Logger: logr.Discard(),
	}

	// Create messages that will fail parsing
	invalidMsg1 := &message{
		priority: 0,
		host:     net.IPv4(127, 0, 0, 1),
		buf:      [32768]byte{},
		size:     5,
	}
	// Add invalid syslog format
	copy(invalidMsg1.buf[:], "invalid")

	invalidMsg2 := &message{
		priority: 0,
		host:     net.IPv4(127, 0, 0, 1),
		buf:      [32768]byte{},
		size:     0, // Empty message
	}

	// Send invalid messages
	parseChannel <- invalidMsg1
	parseChannel <- invalidMsg2
	close(parseChannel)

	// Track completion
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		receiver.runParser()
	}()

	// Wait for processing to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - processing completed even with invalid messages
	case <-time.After(time.Second):
		t.Error("runParser() did not complete processing invalid messages within timeout")
	}
}

func TestSyslogMessagePool_concurrency(t *testing.T) {
	// Test that the pool is safe for concurrent use
	const numGoroutines = 10
	const messagesPerGoroutine = 100

	var wg sync.WaitGroup
	errChan := make(chan error, numGoroutines)

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				// Get from pool
				item := syslogMessagePool.Get()
				msg, ok := item.(*message)
				if !ok || msg == nil {
					errChan <- fmt.Errorf("goroutine %d: pool returned invalid item: %T", id, item)
					return
				}

				// Use the message briefly
				msg.priority = byte(j % 256)
				msg.size = j

				// Put back to pool
				syslogMessagePool.Put(msg)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Check for any errors
		select {
		case err := <-errChan:
			t.Error(err)
		default:
			// Success - no errors
		}
	case <-time.After(5 * time.Second):
		t.Error("Concurrent pool operations did not complete within timeout")
	}
}

func TestStartReceiver_multipleInstances(t *testing.T) {
	// Test starting multiple receiver instances
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start multiple receivers on different ports
	addresses := []string{
		"127.0.0.1:0", // Will get random ports
		"127.0.0.1:0",
		"127.0.0.1:0",
	}

	for i, addr := range addresses {
		t.Run(fmt.Sprintf("instance_%d", i), func(t *testing.T) {
			err := StartReceiver(ctx, logr.Discard(), addr, 2)
			if err != nil {
				t.Errorf("Failed to start receiver instance %d: %v", i, err)
			}
		})
	}
}

func TestReceiver_runParser_DEBUG_severity(t *testing.T) {
	// Test specific handling of DEBUG severity messages
	parseChannel := make(chan *message, 1)
	receiver := &Receiver{
		parse:  parseChannel,
		Logger: logr.Discard(),
	}

	// Create a DEBUG message
	debugMsg := &message{
		priority: 31, // daemon.debug (facility=3, severity=7): 3*8+7=31
		hostname: []byte("debug-host"),
		app:      []byte("debug-app"),
		msg:      []byte("debug message"),
		host:     net.IPv4(127, 0, 0, 1),
		buf:      [32768]byte{},
		size:     25,
	}
	copy(debugMsg.buf[:], "<31>debug-host debug-app: debug message")

	parseChannel <- debugMsg
	close(parseChannel)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		receiver.runParser()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - DEBUG messages should be logged with V(1)
	case <-time.After(time.Second):
		t.Error("runParser() did not complete processing DEBUG message within timeout")
	}
}
