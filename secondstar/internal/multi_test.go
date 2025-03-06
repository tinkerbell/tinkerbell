package internal

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
)

// TestMultiWriterCreation tests basic creation of MultiWriter.
func TestMultiWriterCreation(t *testing.T) {
	tests := []struct {
		name    string
		writers []io.Writer
		wantLen int
	}{
		{
			name:    "empty",
			wantLen: 0,
		},
		{
			name:    "single writer",
			writers: []io.Writer{bytes.NewBuffer(nil)},
			wantLen: 1,
		},
		{
			name:    "multiple writers",
			writers: []io.Writer{bytes.NewBuffer(nil), bytes.NewBuffer(nil)},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := NewMultiWriter(tt.writers...)
			if len(mw.writers) != tt.wantLen {
				t.Errorf("expected %d writers, got %d", tt.wantLen, len(mw.writers))
			}
		})
	}
}

// TestWriteSuccess tests successful writing to multiple writers.
func TestWriteSuccess(t *testing.T) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}
	mw := NewMultiWriter(buf1, buf2)
	data := []byte("test data")

	n, err := mw.Write(data)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("wrong number of bytes written: %d, expected %d", n, len(data))
	}
	if buf1.String() != "test data" || buf2.String() != "test data" {
		t.Errorf("data mismatch: buf1=%q, buf2=%q", buf1.String(), buf2.String())
	}
}

// TestWriteError tests error handling during writing.
func TestWriteError(t *testing.T) {
	failingWriter := &errorWriter{err: io.ErrUnexpectedEOF}
	buf := &bytes.Buffer{}
	mw := NewMultiWriter(failingWriter, buf)
	data := []byte("test data")

	n, err := mw.Write(data)
	if err == nil {
		t.Errorf("expected error but got none")
	}
	if n != 0 {
		t.Errorf("expected 0 bytes written on error, got %d", n)
	}
}

// TestAddRemove tests adding and removing writers.
func TestAddRemove(t *testing.T) {
	mw := NewMultiWriter()
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	// Add first writer
	mw.Add(buf1)
	if len(mw.writers) != 1 {
		t.Errorf("expected 1 writer after add, got %d", len(mw.writers))
	}

	// Add second writer
	mw.Add(buf2)
	if len(mw.writers) != 2 {
		t.Errorf("expected 2 writers after second add, got %d", len(mw.writers))
	}

	// Remove first writer
	mw.Remove(buf1)
	if len(mw.writers) != 1 {
		t.Errorf("expected 1 writer after remove, got %d", len(mw.writers))
	}

	// Try to remove non-existent writer
	mw.Remove(&bytes.Buffer{})
	if len(mw.writers) != 1 {
		t.Errorf("removing non-existent writer changed length")
	}
}

// TestConcurrentAccess tests thread safety.
func TestConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	mw := NewMultiWriter(bytes.NewBuffer(nil))
	numOps := 100

	// Simulate concurrent adds and removes
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				mw.Add(bytes.NewBuffer(nil))
			} else {
				mw.Remove(bytes.NewBuffer(nil))
			}
		}(i)
	}

	wg.Wait()
	// read the writers slice
	mw.mu.RLock()
	defer mw.mu.RUnlock()
	if len(mw.writers) < 1 {
		t.Errorf("expected at least one writer, got %d", len(mw.writers))
	}
}

// errorWriter implements io.Writer and always returns an error.
type errorWriter struct {
	err error
}

func (ew *errorWriter) Write([]byte) (n int, err error) {
	return 0, ew.err
}

// TestShortWrite tests handling of short writes.
func TestShortWrite(t *testing.T) {
	shortWriter := &shortWriter{}
	mw := NewMultiWriter(shortWriter)
	data := []byte("test data")

	n, err := mw.Write(data)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("expected io.ErrShortWrite, got %v", err)
	}
	if n != len(data)-1 {
		t.Errorf("expected %d bytes written on short write, got %d", len(data)-1, n)
	}
}

// shortWriter implements io.Writer and always performs a short write.
type shortWriter struct{}

func (sw *shortWriter) Write(p []byte) (n int, err error) {
	return len(p) - 1, nil // Always write one byte less than requested
}
