package binary

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path"
	"time"

	"github.com/go-logr/logr"
	"github.com/pin/tftp/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TFTP config settings.
type TFTP struct {
	Log                  logr.Logger
	EnableTFTPSinglePort bool
	Addr                 netip.AddrPort
	Timeout              time.Duration
	BlockSize            int
	// Router dispatches each TFTP read request through its configured
	// Routes in order. The caller is responsible for constructing the
	// Router with the routes it wants to enable.
	Router Router
}

// ListenAndServe will listen and serve iPXE binaries over TFTP.
func (h *TFTP) ListenAndServe(ctx context.Context) error {
	a, err := net.ResolveUDPAddr("udp", h.Addr.String())
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", a)
	if err != nil {
		return err
	}

	h.Log.Info("starting TFTP server",
		"addr", h.Addr.String(), "singlePort", h.EnableTFTPSinglePort,
		"blockSize", h.BlockSize, "timeout", h.Timeout.String())

	ts := tftp.NewServer(h.HandleRead, h.HandleWrite)
	ts.SetTimeout(h.Timeout)
	ts.SetBlockSize(h.BlockSize)
	if h.EnableTFTPSinglePort {
		ts.EnableSinglePort()
	}

	go func() {
		<-ctx.Done()
		if err := conn.Close(); err != nil {
			h.Log.Error(err, "failed to close connection")
		}
		ts.Shutdown()
	}()

	return ts.Serve(conn)
}

// HandleRead handlers TFTP GET requests. The function signature satisfies the tftp.Server.readHandler parameter type.
func (h TFTP) HandleRead(filename string, rf io.ReaderFrom) error {
	client := net.UDPAddr{}
	if rpi, ok := rf.(tftp.OutgoingTransfer); ok {
		client = rpi.RemoteAddr()
	}

	full := filename

	// Clients can send traceparent over TFTP by appending the traceparent string
	// to the end of the filename they really want. Strip it from the full path
	// (not just the basename) so routes that match on the request path still
	// match when a traceparent is appended.
	ctx, shortfull, err := extractTraceparentFromFilename(context.Background(), full)
	if err != nil {
		// Include request context (the raw requested path and client) so a
		// malformed traceparent can actually be traced back in production.
		h.Log.Error(err, "failed to extract traceparent from filename", "filename", full, "client", client)
	}

	filename = path.Base(shortfull)
	log := h.Log.WithValues("event", "get", "filename", filename, "uri", full, "client", client)
	if shortfull != full {
		// Log the traceparent-stripped full path (not the basename, which is
		// already logged as "filename") so path-matching routes are debuggable.
		log = log.WithValues("shortfile", shortfull)
		log.Info("traceparent found in filename", "filenameWithTraceparent", full)
	}
	// If a mac address is provided (0a:00:27:00:00:02/snp.efi), parse and log it.
	// Mac address is optional.
	optionalMac, _ := net.ParseMAC(path.Dir(shortfull))
	log = log.WithValues("macFromURI", optionalMac.String())

	tracer := otel.Tracer("TFTP")
	_, span := tracer.Start(ctx, "TFTP get",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("filename", filename)),
		trace.WithAttributes(attribute.String("requested-filename", full)),
		trace.WithAttributes(attribute.String("ip", client.IP.String())),
		trace.WithAttributes(attribute.String("mac", optionalMac.String())),
	)
	defer span.End()

	req := Request{Filename: shortfull, Base: filename, Client: client}

	err = h.Router.Handle(ctx, req, rf)
	switch {
	case err == nil:
		span.SetStatus(codes.Ok, filename)
	case errors.Is(err, os.ErrNotExist):
		// Expected fall-through: no route claimed the request. The Router wraps
		// os.ErrNotExist for this case; log it quietly rather than as an error.
		log.V(1).Info("no route handled request", "err", err)
		span.SetStatus(codes.Error, "not found")
	default:
		// A route claimed the request but failed to serve it (eg patch/serve error).
		log.Error(err, "request handling failed")
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// HandleWrite handles TFTP PUT requests. It will always return an error. This library does not support PUT.
func (h TFTP) HandleWrite(filename string, wt io.WriterTo) error {
	err := fmt.Errorf("access_violation: %w", os.ErrPermission)
	client := net.UDPAddr{}
	if rpi, ok := wt.(tftp.OutgoingTransfer); ok {
		client = rpi.RemoteAddr()
	}
	h.Log.Error(err, "client", client, "event", "put", "filename", filename)

	return err
}
