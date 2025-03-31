package binary

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"
	binary "github.com/tinkerbell/tinkerbell/smee/internal/ipxe/binary/file"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// implements the TFTP read and write function handlers.
type Handler struct {
	Log   logr.Logger
	Patch []byte
}

// Handle handles GET and HEAD responses to HTTP requests.
// Serves embedded iPXE binaries.
func (h Handler) Handle(w http.ResponseWriter, req *http.Request) {
	h.Log.V(1).Info("handling request", "method", req.Method, "path", req.URL.Path)
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host, port, _ := net.SplitHostPort(req.RemoteAddr)
	log := h.Log.WithValues("host", host, "port", port)
	// If a mac address is provided (/0a:00:27:00:00:02/snp.efi), parse and log it.
	// Mac address is optional.
	optionalMac, _ := net.ParseMAC(strings.TrimPrefix(path.Dir(req.URL.Path), "/"))
	log = log.WithValues("macFromURI", optionalMac.String())
	filename := filepath.Base(req.URL.Path)
	log = log.WithValues("filename", filename)

	// clients can send traceparent over HTTP by appending the traceparent string
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

	tracer := otel.Tracer("HTTP")
	_, span := tracer.Start(ctx, fmt.Sprintf("HTTP %v", req.Method),
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("filename", filename)),
		trace.WithAttributes(attribute.String("requested-filename", longfile)),
		trace.WithAttributes(attribute.String("ip", host)),
		trace.WithAttributes(attribute.String("mac", optionalMac.String())),
	)
	defer span.End()

	file, found := binary.Files[filename]
	if !found {
		log.Info("requested file not found")
		http.NotFound(w, req)
		span.SetStatus(codes.Error, "requested file not found")

		return
	}

	file, err = binary.Patch(file, h.Patch)
	if err != nil {
		log.Error(err, "error patching file")
		w.WriteHeader(http.StatusInternalServerError)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	http.ServeContent(w, req, filename, time.Now(), bytes.NewReader(file))
	switch req.Method {
	case http.MethodGet:
		log.Info("file served", "name", filename, "fileSize", len(file))
	case http.MethodHead:
		log.Info("HEAD method requested", "fileSize", len(file))
	}
	span.SetStatus(codes.Ok, filename)
}

// extractTraceparentFromFilename takes a context and filename and checks the filename for
// a traceparent tacked onto the end of it. If there is a match, the traceparent is extracted
// and a new SpanContext is constructed and added to the context.Context that is returned.
// The filename is shortened to just the original filename so the rest of the serving process can
// carry on as usual.
func extractTraceparentFromFilename(ctx context.Context, filename string) (context.Context, string, error) {
	// traceparentRe captures 4 items, the original filename, the trace id, span id, and trace flags
	traceparentRe := regexp.MustCompile("^(.*)-[[:xdigit:]]{2}-([[:xdigit:]]{32})-([[:xdigit:]]{16})-([[:xdigit:]]{2})")
	parts := traceparentRe.FindStringSubmatch(filename)
	if len(parts) == 5 {
		traceID, err := trace.TraceIDFromHex(parts[2])
		if err != nil {
			return ctx, filename, fmt.Errorf("parsing OpenTelemetry trace id %q failed: %w", parts[2], err)
		}

		spanID, err := trace.SpanIDFromHex(parts[3])
		if err != nil {
			return ctx, filename, fmt.Errorf("parsing OpenTelemetry span id %q failed: %w", parts[3], err)
		}

		// create a span context with the parent trace id & span id
		spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     spanID,
			Remote:     true,
			TraceFlags: trace.FlagsSampled, // TODO: use the parts[4] value instead
		})

		// inject it into the context.Context and return it along with the original filename
		return trace.ContextWithSpanContext(ctx, spanCtx), parts[1], nil
	}
	// no traceparent found, return everything as it was
	return ctx, filename, nil
}
