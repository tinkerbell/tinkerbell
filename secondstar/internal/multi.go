// https://github.com/alanshaw/multiwriter
package internal

import (
	"io"
	"sync"
)

// MultiWriter is a writer that writes to multiple other writers.
type MultiWriter struct {
	mu      sync.RWMutex
	writers []io.Writer
}

// NewMultiWriter creates a writer that duplicates its writes to all the provided writers,
// similar to the Unix tee(1) command. Writers can be added and removed
// dynamically after creation.
//
// Each write is written to each listed writer, one at a time. If a listed
// writer returns an error, that overall write operation stops and returns the
// error; it does not continue down the list.
func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	mw := &MultiWriter{
		writers: writers,
		mu:      sync.RWMutex{},
	}
	return mw
}

// Write writes some bytes to all the writers.
func (mw *MultiWriter) Write(p []byte) (int, error) {
	mw.mu.RLock()
	defer mw.mu.RUnlock()

	for _, w := range mw.writers {
		n, err := w.Write(p)
		if err != nil {
			return n, err
		}

		if n < len(p) {
			return n, io.ErrShortWrite
		}
	}

	return len(p), nil
}

// Add appends a writer to the list of writers this multiwriter writes to.
func (mw *MultiWriter) Add(w io.Writer) {
	mw.mu.Lock()
	mw.writers = append(mw.writers, w)
	mw.mu.Unlock()
}

// Remove will remove a previously added writer from the list of writers.
func (mw *MultiWriter) Remove(w io.Writer) {
	mw.mu.Lock()
	var writers []io.Writer
	for _, ew := range mw.writers {
		if ew != w {
			writers = append(writers, ew)
		}
	}
	mw.writers = writers
	mw.mu.Unlock()
}
