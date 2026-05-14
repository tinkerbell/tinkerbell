package containerd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// jsonLogEntry is the per-line shape consumed by `nerdctl logs` (and
// Docker's json-file driver). nerdctl preserves the trailing "\n" inside
// the Log field.
//
// Mirror of nerdctl's pkg/logging/jsonfile.Entry:
// https://github.com/containerd/nerdctl/blob/main/pkg/logging/jsonfile/jsonfile.go
type jsonLogEntry struct {
	Log    string    `json:"log,omitempty"`
	Stream string    `json:"stream,omitempty"`
	Time   time.Time `json:"time"`
}

// jsonLogWriter is an io.Writer that splits its input on newlines and emits
// one JSON-Lines record per complete line. The trailing newline of each
// line is preserved inside the "log" field, matching Docker's json-file
// format.
//
// Multiple jsonLogWriters can share a single mutex/encoder/file handle so
// that interleaved stdout+stderr writes still produce well-formed JSONL.
type jsonLogWriter struct {
	mu     *sync.Mutex
	enc    *json.Encoder
	stream string

	// buf accumulates partial lines (input that did not end in '\n') so
	// that they are emitted as a single record once the rest arrives.
	// Guarded by mu (cross-stream serialization is fine; each writer only
	// touches its own buf, but we share the lock with the encoder anyway).
	buf bytes.Buffer
}

// Write implements io.Writer. It always returns len(p) on success so it
// can be wrapped in io.MultiWriter alongside os.Stdout/os.Stderr.
func (w *jsonLogWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	// Accumulate, then flush every complete line.
	w.buf.Write(p)
	for {
		data := w.buf.Bytes()
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		// Emit data[:i+1] (include the newline, matching nerdctl).
		line := make([]byte, i+1)
		copy(line, data[:i+1])
		// Advance past this line BEFORE attempting to encode, so that a
		// failing encode does not leave the bad bytes in w.buf where the
		// next Write would re-encode them and repeat the error
		// indefinitely. The failing record is dropped (logging is
		// best-effort here; the same content is also tee'd to the agent's
		// own stdout via io.MultiWriter).
		w.buf.Next(i + 1)
		if err := w.encodeLocked(string(line)); err != nil {
			// All of p has already been absorbed into w.buf (and any
			// earlier complete lines were encoded successfully). Report
			// the bytes as consumed so io.MultiWriter does not raise a
			// spurious "short write" on top of the real encode error.
			return len(p), err
		}
	}
	return len(p), nil
}

// flush emits any buffered partial line as a final record. Callers must
// invoke this once writes are done (e.g. before closing the file) so the
// last line of output without a trailing newline is not lost.
func (w *jsonLogWriter) flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf.Len() == 0 {
		return nil
	}
	line := w.buf.String()
	w.buf.Reset()
	return w.encodeLocked(line)
}

// encodeLocked writes a single JSONL record. Must be called with w.mu held.
func (w *jsonLogWriter) encodeLocked(line string) error {
	return w.enc.Encode(jsonLogEntry{
		Log:    line,
		Stream: w.stream,
		Time:   time.Now().UTC(),
	})
}

// jsonLogPair is the pair of writers (one per stream) plus a Close that
// flushes any buffered partial lines and closes the underlying file.
type jsonLogPair struct {
	Stdout io.Writer
	Stderr io.Writer

	flushers []*jsonLogWriter
	file     *os.File
}

// Close flushes any buffered partial lines and closes the file. It is safe
// to call multiple times. The underlying file is always closed even if a
// flush returns an error, so a partial-line encode failure does not leak
// the OS file descriptor; all errors are joined via errors.Join.
func (p *jsonLogPair) Close() error {
	if p == nil || p.file == nil {
		return nil
	}
	var errs []error
	for _, w := range p.flushers {
		if err := w.flush(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := p.file.Close(); err != nil {
		errs = append(errs, err)
	}
	p.file = nil
	return errors.Join(errs...)
}

// newJSONLogPair opens (or creates) the json-file at path and returns
// stdout/stderr writers that share a single file handle, mutex and
// json.Encoder so JSONL records stay well-formed across both streams.
func newJSONLogPair(path string) (*jsonLogPair, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	mu := &sync.Mutex{}
	enc := json.NewEncoder(f)
	stdoutW := &jsonLogWriter{mu: mu, enc: enc, stream: "stdout"}
	stderrW := &jsonLogWriter{mu: mu, enc: enc, stream: "stderr"}
	return &jsonLogPair{
		Stdout:   stdoutW,
		Stderr:   stderrW,
		flushers: []*jsonLogWriter{stdoutW, stderrW},
		file:     f,
	}, nil
}
