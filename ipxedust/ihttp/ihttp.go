// Package http implements an HTTP server for iPXE binaries.
package ihttp

import (
	"context"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/ipxedust/binary"
	"inet.af/netaddr"
)

// Handler is the struct that implements the http.Handler interface.
type Handler struct {
	Log logr.Logger
}

// ListenAndServe is a patterned after http.ListenAndServe.
// It listens on the TCP network address srv.Addr and then
// calls ServeHTTP to handle requests on incoming connections.
//
// ListenAndServe always returns a non-nil error. After Shutdown or Close,
// the returned error is http.ErrServerClosed.
func ListenAndServe(ctx context.Context, addr netaddr.IPPort, h *http.Server) error {
	conn, err := net.Listen("tcp", addr.String())
	if err != nil {
		return err
	}
	return Serve(ctx, conn, h)
}

// Serve is patterned after http.Serve.
// It accepts incoming connections on the Listener conn and serves them
// using the Server h.
//
// Serve always returns a non-nil error and closes conn.
// After Shutdown or Close, the returned error is http.ErrServerClosed.
func Serve(_ context.Context, conn net.Listener, h *http.Server) error {
	return h.Serve(conn)
}

func trimFirstRune(s string) string {
	_, i := utf8.DecodeRuneInString(s)
	return s[i:]
}

// Handle handles responses to HTTP requests.
func (s Handler) Handle(w http.ResponseWriter, req *http.Request) {
	s.Log.V(1).Info("handling request", "method", req.Method, "path", req.URL.Path)
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host, port, _ := net.SplitHostPort(req.RemoteAddr)
	s.Log = s.Log.WithValues("host", host, "port", port)
	m := path.Dir(req.URL.Path)
	if strings.HasPrefix(m, "/") {
		m = trimFirstRune(path.Dir(req.URL.Path))
	}
	// If a mac address is provided, log it. Mac address is optional.
	// Mac address would be useful for logging and tracing.
	mac, _ := net.ParseMAC(m)
	s.Log = s.Log.WithValues("mac", mac)

	got := filepath.Base(req.URL.Path)
	file, found := binary.Files[got]
	if !found {
		s.Log.Info("requested file not found", "file", got)
		http.NotFound(w, req)
		return
	}
	b, err := w.Write(file)
	if err != nil {
		s.Log.Error(err, "error serving file")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.Log.Info("file served", "bytes sent", b, "file size", len(file), "file", got)
}
