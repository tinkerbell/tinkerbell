package script

import (
	"bytes"

	"github.com/go-logr/logr"
)

// logWriter turns a script's raw output stream into one log entry per line. Writes arrive in
// arbitrary chunks that do not respect line boundaries, so partial lines are buffered until their
// newline shows up. Flush emits any trailing content the script left unterminated.
type logWriter struct {
	log    logr.Logger
	action string
	stream string
	buf    bytes.Buffer
}

func newLogWriter(log logr.Logger, action, stream string) *logWriter {
	return &logWriter{log: log, action: action, stream: stream}
}

// Write implements io.Writer. It never reports an error: losing an Action's output to the log is
// not a reason to fail the Action itself.
func (w *logWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	// Emit every complete line and leave the rest buffered until its newline arrives.
	for {
		unread := w.buf.Bytes()
		i := bytes.IndexByte(unread, '\n')
		if i < 0 {
			break
		}
		w.emit(string(unread[:i]))
		w.buf.Next(i + 1)
	}

	return len(p), nil
}

// Flush emits any buffered partial line. Call it once the script has exited.
func (w *logWriter) Flush() {
	if w.buf.Len() == 0 {
		return
	}
	w.emit(w.buf.String())
	w.buf.Reset()
}

func (w *logWriter) emit(line string) {
	// Strip a trailing CR so scripts writing CRLF don't leave a stray character in the log.
	w.log.Info(string(bytes.TrimSuffix([]byte(line), []byte("\r"))), "action", w.action, "stream", w.stream)
}
