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
	binary "github.com/tinkerbell/tinkerbell/smee/internal/ipxe/binary/file"
	ipxe "github.com/tinkerbell/tinkerbell/smee/internal/ipxe/script"
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
	Patch                []byte
	BlockSize            int
	Backend              ipxe.BackendReader
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

	content, handledByBinary := binary.Files[filepath.Base(shortfile)]
	if !handledByBinary {
		servedByHardware, errHw := tryServeAssetFromHardware(filename, client, rf, log, full, span, h)
		if servedByHardware {
			return errHw
		}

		// if AssetDir is set, stream the file directly from disk if found.
		if h.AssetDir != "" {
			servedFromDisk, errDsk := tryServeAssetFromDisk(filename, rf, h, full, log, span)
			if servedFromDisk {
				return errDsk
			}
		}

		// if still not handled, return error; file not found. otherwise proceed to patch and serve.
		err404 := fmt.Errorf("file [%v] unknown: %w", filepath.Base(shortfile), os.ErrNotExist)
		log.Error(err404, "file unknown")
		span.SetStatus(codes.Error, err404.Error())
		return err404
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

func tryServeAssetFromHardware(filename string, client net.UDPAddr, rf io.ReaderFrom, log logr.Logger, full string, span trace.Span, tftpServer TFTP) (bool, error) {
	// pxelinux has a "tell"; it will by default hit the tftp server for "pxelinux.cfg/01-<MAC-ADDRESS-DASHED-UPPER>"

	const pxelinuxFullMACPrefix = "pxelinux.cfg/01-"
	const pxelinuxFullMACPrefixLen = len(pxelinuxFullMACPrefix)
	const pxelinuxMacSuffix = "00-00-00-00-00-00"
	const pxeLinuxMacSuffixLen = len(pxelinuxMacSuffix)
	const pxeLinuxFullLen = pxelinuxFullMACPrefixLen + pxeLinuxMacSuffixLen

	inputFnLength := len(full)

	// Check if we can parse the MAC out of the rest of the string if it's exactly the expected size
	if inputFnLength == pxeLinuxFullLen && full[0:pxelinuxFullMACPrefixLen] == pxelinuxFullMACPrefix {
		log.Info("pxelinux.cfg request matches exact expected format", "full", full)
		macStr := full[pxelinuxFullMACPrefixLen:]
		didServe, errHwServe := tryServeAssetFromPXEMacFilename(filename, client, rf, log, full, span, tftpServer.Backend, macStr)
		if didServe {
			return didServe, errHwServe
		}
	}

	// If not a "pxelinux.cfg/01-<MAC-ADDRESS-DASHED-UPPER>" request, try and get HW by IP.
	// If a HW found, it might have either RPi-Netboot templates (xxxx/config.txt, xxxx/cmdline.txt), or
	// a generic path-rewrite eg 'xxxxx/' ->  'captain-armbian-rpi/'.
	hardware, errHwLookupByIP := ipxe.GetByIP(context.Background(), client.IP, tftpServer.Backend)
	if errHwLookupByIP != nil {
		log.Error(errHwLookupByIP, "failed to get hardware by IP", "client", client)
		return false, nil
	}
	log.Info("got tftp request for hardware via IP", "full", full, "hardware", hardware)

	// RPiNetboot
	if hardware.RPiNetboot.PiSerialNum == "" || hardware.RPiNetboot.AssetRewrite == "" || tftpServer.AssetDir == "" {
		log.Info("hardware does not have RPiNetboot data; skipping RPiNetboot checks", "full", full, "hardware", hardware)
		return false, nil
	}

	log.Info("hardware has RPiNetboot data; checking if request is for config.txt or cmdline.txt", "full", full)
	switch full {
	case hardware.RPiNetboot.PiSerialNum + "/config.txt":
		log.Info("request is for config.txt; serving RPiNetboot ConfigTxtTemplate", "full", full)
		return serveFromHardware(filename, log, full, hardware, rf, span, hardware.RPiNetboot.ConfigTxtTemplate)
	case hardware.RPiNetboot.PiSerialNum + "/cmdline.txt":
		log.Info("request is for cmdline.txt; serving RPiNetboot CmdlineTxtTemplate", "full", full)
		return serveFromHardware(filename, log, full, hardware, rf, span, hardware.RPiNetboot.CmdlineTxtTemplate)
	}
	log.Info("request is not for a known RPiNetboot file (config.txt or cmdline.txt); will try generic serve and asset dir next", "full", full)

	// rewrite the "full uri" by replacing the PiSerialNum prefix with the AssetRewrite value
	rewrittenFull := hardware.RPiNetboot.AssetRewrite + full[len(hardware.RPiNetboot.PiSerialNum):]
	log.Info("rewritten filename for asset dir lookup", "full", full, "rewrittenFull", rewrittenFull)
	servedFromDisk, errDsk := tryServeAssetFromDisk(filename, rf, tftpServer, rewrittenFull, log, span)
	if servedFromDisk {
		return true, errDsk
	}

	return false, nil
}

func tryServeAssetFromPXEMacFilename(filename string, client net.UDPAddr, rf io.ReaderFrom, log logr.Logger, full string, span trace.Span, backend ipxe.BackendReader, macStr string) (bool, error) {
	log.Info("parsed MAC string from pxelinux.cfg request", "macStr", macStr)
	hardwareAddr, errMacParse := net.ParseMAC(macStr)
	if errMacParse != nil {
		log.Error(errMacParse, "failed to parse MAC from pxelinux.cfg request", "macStr", macStr)
	} else {
		log.Info("parsed MAC address from pxelinux.cfg request; looking up in Backend...", "hardwareAddr", hardwareAddr)
		hardware, errHwLookupByMac := ipxe.GetByMac(context.Background(), hardwareAddr, backend)
		if errHwLookupByMac != nil {
			log.Error(errHwLookupByMac, "failed to get hardware by MAC", "client", client, "full", full, "macStr", macStr, "hardwareAddr", hardwareAddr)
		} else {
			didServe, errHwServe := serveFromHardware(filename, log, full, hardware, rf, span, hardware.PXELINUX.Template)
			if didServe {
				return didServe, errHwServe
			}
		}
	}
	return false, nil
}

func serveFromHardware(filename string, log logr.Logger, full string, hardware ipxe.Info, rf io.ReaderFrom, span trace.Span, template string) (bool, error) {
	// got a hardware, serve it  from there
	log.Info("got tftp request for hardware", "full", full, "hardware", hardware)
	if template == "" {
		log.Info("no template found in hardware; cannot serve", "full", full, "hardware", hardware)
		return false, nil // Not served, empty template, "next!"
	}
	// do actual templating here one day
	bytesSent, err := rf.ReadFrom(bytes.NewReader([]byte(template)))
	if err != nil {
		log.Error(err, "serving from template in hardware failed", "bytesSent", bytesSent, "contentSize", len([]byte(template)))
		return true, err // tried & failed to serve
	}
	log.Info("file served from hardware", "bytesSent", bytesSent)
	span.SetStatus(codes.Ok, filename)
	return true, nil // tried & served OK
}

func tryServeAssetFromDisk(filename string, rf io.ReaderFrom, h TFTP, full string, log logr.Logger, span trace.Span) (bool, error) {
	// Join the h.AssetDir with the full requested path ("full") in a secure way; prevent path traversal
	assetPath := filepath.Join(h.AssetDir, full)
	log.Info("attempting to load file from asset dir", "assetPath", assetPath, "assetDir", h.AssetDir)

	file, err := os.Open(assetPath)
	if err != nil {
		log.Error(err, "failed to read file from asset dir", "assetPath", assetPath)
		return false, nil
	}

	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Error(cerr, "failed to close file", "assetPath", assetPath)
		}
	}()

	log.Info("streaming file directly from asset dir", "assetPath", assetPath)

	bytesSent, err := rf.ReadFrom(file)
	if err != nil {
		log.Error(err, "file serve failed", "bytesSent", bytesSent)
		span.SetStatus(codes.Error, err.Error())
		return true, err
	}

	log.Info("file served from disk", "bytesSent", bytesSent)
	span.SetStatus(codes.Ok, filename)
	return true, nil
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
