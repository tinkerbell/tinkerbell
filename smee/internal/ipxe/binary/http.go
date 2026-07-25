package binary

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// HTTPHandler serves the same TFTP Routes over HTTP. It is used to expose
// pxelinux.cfg and the on-disk asset directory to clients that fetch them via
// HTTP (eg. u-boot's pxe-over-http, which uses wget) instead of TFTP, using the
// exact same request path shapes.
//
// The requested URL path has PathPrefix stripped and is then handed to the
// Router as a binary.Request, so the underlying Route implementations
// (PXELinuxMACRoute, DiskAssetRoute, ...) are reused unchanged.
type HTTPHandler struct {
	Log logr.Logger
	// Router dispatches each request through its configured Routes in order.
	// The caller is responsible for constructing the Router with the routes it
	// wants to enable.
	Router Router
	// PathPrefix is the URL path prefix the handler is mounted under (eg.
	// "/tftp/"). It is stripped before the remaining path is dispatched. A
	// missing leading slash is normalized so it always matches the mount
	// prefix regardless of how it was configured (eg. "tftp" == "/tftp/").
	PathPrefix string
}

// normalizePathPrefix canonicalizes a mount prefix to a leading- and
// trailing-slashed, path.Clean'd form (eg. "tftp", "/tftp", and "/tftp//" all
// become "/tftp/"). It mirrors the normalization the HTTP server applies when
// mounting the handler, so prefix stripping always agrees with the mount point.
func normalizePathPrefix(prefix string) string {
	p := strings.TrimSpace(prefix)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

// Handle handles GET and HEAD HTTP requests by dispatching them through the
// Router. A request that no Route claims results in a 404.
func (h HTTPHandler) Handle(w http.ResponseWriter, req *http.Request) {
	h.Log.V(1).Info("handling request", "method", req.Method, "path", req.URL.Path)
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Strip the mount prefix and any leading slash so the remaining path
	// matches the shapes the Routes expect (eg. "pxelinux.cfg/01-<MAC>").
	// PathPrefix is normalized the same way the HTTP server normalizes the
	// mount point, so stripping always agrees with where the handler is
	// actually mounted, even for inputs like "tftp" or "/tftp//".
	rel := strings.TrimPrefix(req.URL.Path, normalizePathPrefix(h.PathPrefix))
	rel = strings.TrimPrefix(rel, "/")

	host, port, _ := net.SplitHostPort(req.RemoteAddr)
	// Client is kept for parity with the TFTP request; the routes enabled for
	// HTTP (pxelinux, disk asset) key on the path, not the client address.
	client := net.UDPAddr{IP: net.ParseIP(host)}
	log := h.Log.WithValues("host", host, "port", port, "path", req.URL.Path, "rel", rel)

	tracer := otel.Tracer("HTTP")
	ctx, span := tracer.Start(req.Context(), "HTTP "+req.Method,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("filename", rel)),
		trace.WithAttributes(attribute.String("ip", host)),
	)
	defer span.End()

	r := Request{Filename: rel, Base: path.Base(rel), Client: client}

	err := h.Router.Handle(ctx, r, httpReaderFrom{w: w, head: req.Method == http.MethodHead})
	switch {
	case err == nil:
		span.SetStatus(codes.Ok, rel)
	case errors.Is(err, os.ErrNotExist):
		log.Info("no route handled request")
		http.NotFound(w, req)
		span.SetStatus(codes.Error, "not found")
	default:
		// A route claimed the request but failed. If it already started
		// writing the body the status is fixed, but attempting a 500 here is
		// still correct for the pre-write failure case.
		log.Error(err, "request handling failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		span.SetStatus(codes.Error, err.Error())
	}
}

// httpReaderFrom adapts an http.ResponseWriter to io.ReaderFrom so the TFTP
// Routes (which serve their payload via ReaderFrom) can be reused over HTTP
// without modification.
type httpReaderFrom struct {
	w http.ResponseWriter
	// head is set for HEAD requests, where the headers (including
	// Content-Length) are written but the body is not streamed.
	head bool
}

func (h httpReaderFrom) ReadFrom(r io.Reader) (int64, error) {
	// Set Content-Length when the reader's size is known. Both *bytes.Reader
	// (pxelinux config) and *os.File (disk assets) implement io.Seeker, so
	// clients such as u-boot's wget get a proper length instead of chunked
	// transfer-encoding.
	if s, ok := r.(io.Seeker); ok {
		if size, err := s.Seek(0, io.SeekEnd); err == nil {
			// Only advertise the length once we can seek back to the start;
			// a failed seek-back would otherwise leave the reader at EOF and
			// io.Copy would silently write an empty body.
			if _, err := s.Seek(0, io.SeekStart); err != nil {
				return 0, err
			}
			h.w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		}
	}
	// HEAD: headers only, don't stream (and discard) the body.
	if h.head {
		return 0, nil
	}
	return io.Copy(h.w, r)
}
