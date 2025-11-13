package binary

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/go-logr/logr"
	binary "github.com/tinkerbell/tinkerbell/smee/internal/ipxe/binary/file"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// HandleTFTP implements the TFTP read function handler.
func (h Handler) HandleTFTP(filename string, rf io.ReaderFrom) error {
	log := h.Log.WithValues("event", "ipxe_binary", "filename", filename)
	log.Info("handling iPXE binary file request")

	// Create tracing context
	tracer := otel.Tracer("TFTP-Binary")
	_, span := tracer.Start(context.Background(), "TFTP binary serve",
		trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	// Check if binary exists in embedded files
	content, ok := binary.Files[filename]
	if !ok {
		log.Info("iPXE binary not found", "filename", filename)
		span.SetStatus(codes.Error, "file not found")
		return ErrNotFound
	}

	// Apply patch if configured
	if len(h.Patch) > 0 {
		var err error
		content, err = binary.Patch(content, h.Patch)
		if err != nil {
			log.Error(err, "failed to patch binary")
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}

	log.Info("successfully loaded iPXE binary", "size", len(content))

	// Serve the content
	return serveContent(content, rf, log, span, filename)
}

func serveContent(content []byte, rf io.ReaderFrom, log logr.Logger, span trace.Span, filename string) error {
	if transfer, ok := rf.(interface{ SetSize(int64) }); ok {
		transfer.SetSize(int64(len(content)))
	}

	reader := bytes.NewReader(content)
	bytesRead, err := rf.ReadFrom(reader)
	if err != nil {
		log.Error(err, "file serve failed", "bytesRead", bytesRead, "contentSize", len(content))
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	log.Info("file served", "bytesSent", bytesRead, "contentSize", len(content))
	span.SetStatus(codes.Ok, filename)
	return nil
}

// ErrNotFound represents a file not found error
var ErrNotFound = errors.New("file not found")
