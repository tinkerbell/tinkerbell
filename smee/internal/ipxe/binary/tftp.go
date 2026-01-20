package binary

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"path"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/pin/tftp/v3"
	binary "github.com/tinkerbell/tinkerbell/smee/internal/ipxe/binary/file"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// HandleTFTP implements the TFTP read function handler.
func (h Handler) HandleTFTP(filename string, rf io.ReaderFrom) error {
	client := net.UDPAddr{}
	if rpi, ok := rf.(tftp.OutgoingTransfer); ok {
		client = rpi.RemoteAddr()
	}

	full := filename
	filename = path.Base(filename)
	log := h.Log.WithValues("event", "get", "filename", filename, "uri", full, "client", client)

	// clients can send traceparent over TFTP by appending the traceparent string
	// to the end of the filename they really want
	longfile := filename // hang onto this to report in traces
	ctx, shortfile, err := extractTraceparentFromFilename(context.Background(), filename)
	if err != nil {
		log.Error(err, "failed to extract traceparent from filename")
	}
	if shortfile != filename {
		log = log.WithValues("shortfile", shortfile)
		log.Info("traceparent found in filename", "filenameWithTraceparent", longfile)
		filename = shortfile
	}
	// If a mac address is provided (0a:00:27:00:00:02/snp.efi), parse and log it.
	// Mac address is optional.
	optionalMac, _ := net.ParseMAC(path.Dir(full))
	log = log.WithValues("macFromURI", optionalMac.String())

	tracer := otel.Tracer("TFTP")
	_, span := tracer.Start(ctx, "TFTP get",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("filename", filename)),
		trace.WithAttributes(attribute.String("requested-filename", longfile)),
		trace.WithAttributes(attribute.String("ip", client.IP.String())),
		trace.WithAttributes(attribute.String("mac", optionalMac.String())),
	)
	defer span.End()

	// Check if binary exists in embedded files
	content, ok := binary.Files[filepath.Base(shortfile)]
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
