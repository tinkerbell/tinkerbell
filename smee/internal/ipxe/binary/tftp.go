package binary

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/pin/tftp/v3"
	"github.com/tinkerbell/tinkerbell/pkg/data"
	binary "github.com/tinkerbell/tinkerbell/smee/internal/ipxe/binary/file"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// BackendReader is the interface for getting data from a backend.
//
// Backends implement this interface to provide DHCP and Netboot data to the handlers.
type BackendReader interface {
	GetByIP(context.Context, net.IP) (data.Hardware, error)
}

// TFTP config settings.
type TFTP struct {
	Log                  logr.Logger
	EnableTFTPSinglePort bool
	Addr                 netip.AddrPort
	Timeout              time.Duration
	Patch                []byte
	BlockSize            int
	Backend              BackendReader
	AssetDir             string
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
		conn.Close()
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

	content, ok := binary.Files[filepath.Base(shortfile)]
	if !ok {
		hardware, errHw := h.Backend.GetByIP(context.Background(), client.IP)
		if errHw != nil {
			log.Error(errHw, "failed to get hardware by IP")
		} else {
			log.Info("got tftp request for hardware", "assetDir", h.AssetDir, "hardware", hardware)
		}

		err := fmt.Errorf("file [%v] unknown: %w", filepath.Base(shortfile), os.ErrNotExist)
		log.Error(err, "file unknown")
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	content, err = binary.Patch(content, h.Patch)
	if err != nil {
		log.Error(err, "failed to patch binary")
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	ct := bytes.NewReader(content)
	b, err := rf.ReadFrom(ct)
	if err != nil {
		log.Error(err, "file serve failed", "b", b, "contentSize", len(content))
		span.SetStatus(codes.Error, err.Error())

		return err
	}
	log.Info("file served", "bytesSent", b, "contentSize", len(content))
	span.SetStatus(codes.Ok, filename)

	return nil
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
