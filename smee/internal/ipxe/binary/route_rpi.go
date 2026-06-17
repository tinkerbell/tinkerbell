package binary

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/smee/internal/hardware"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RPiNetbootRoute handles RaspberryPi EEPROM netboot, which addresses by
// serial number rather than MAC: requests arrive as "<SerialNum>/<file>"
// from the client identified by IP.
//
// The route:
//   - Looks up Hardware by req.Client.IP.
//   - If the Hardware has RPI.SerialNum and FirmwarePath set and AssetDir
//     is configured, and req.Filename starts with "<SerialNum>/", either
//     serves an inline value (RPI.ConfigTxt for config.txt; the joined
//     OSIE.KernelParams for cmdline.txt) or rewrites the path's serial
//     prefix to FirmwarePath and streams the file from AssetDir.
//
// Returns handled=false when there's no Hardware match, no RPI config on
// the Hardware, no AssetDir, the path doesn't have the serial prefix, or
// the rewritten on-disk file does not exist.
type RPiNetbootRoute struct {
	Log      logr.Logger
	Resolver hardware.Resolver
	AssetDir string
}

func (r RPiNetbootRoute) Name() string { return "rpi-netboot" }

func (r RPiNetbootRoute) TryServe(ctx context.Context, req Request, w io.ReaderFrom) (bool, error) {
	if r.AssetDir == "" {
		return false, nil
	}
	log := r.Log.WithValues("route", r.Name(), "filename", req.Filename, "client", req.Client)
	span := trace.SpanFromContext(ctx)

	hw, err := r.Resolver.ByIP(ctx, req.Client.IP)
	if err != nil {
		log.Error(err, "failed to get hardware by IP")
		return false, nil
	}

	rpi := hw.RPI
	if rpi.SerialNum == "" || rpi.FirmwarePath == "" {
		log.Info("hardware does not have RPI data; skipping")
		return false, nil
	}

	if !strings.HasPrefix(req.Filename, rpi.SerialNum+"/") {
		log.Info("request path does not begin with SerialNum; skipping", "serialNum", rpi.SerialNum)
		return false, nil
	}

	switch req.Filename {
	case rpi.SerialNum + "/config.txt":
		log.Info("serving RPI ConfigTxt")
		return serveTemplate(w, log, span, req.Filename, rpi.ConfigTxt)
	case rpi.SerialNum + "/cmdline.txt":
		cmdline := strings.Join(hw.OSIE.KernelParams, " ")
		log.Info("serving cmdline.txt from OSIE.KernelParams", "params", hw.OSIE.KernelParams)
		return serveTemplate(w, log, span, req.Filename, cmdline)
	}

	rewritten := rpi.FirmwarePath + req.Filename[len(rpi.SerialNum):]
	assetPath := filepath.Join(r.AssetDir, rewritten)
	log.Info("attempting to load rewritten file from asset dir", "rewritten", rewritten, "assetPath", assetPath)

	file, err := os.Open(assetPath)
	if err != nil {
		log.Info("rewritten asset not found on disk; skipping", "assetPath", assetPath, "err", err)
		return false, nil
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Error(cerr, "failed to close file", "assetPath", assetPath)
		}
	}()

	bytesSent, err := w.ReadFrom(file)
	if err != nil {
		log.Error(err, "serving rewritten asset failed", "assetPath", assetPath, "bytesSent", bytesSent)
		span.SetStatus(codes.Error, err.Error())
		return true, err
	}
	log.Info("rewritten asset served from disk", "assetPath", assetPath, "bytesSent", bytesSent)
	span.SetStatus(codes.Ok, req.Filename)
	return true, nil
}

// serveTemplate writes a hardware-supplied template to the TFTP writer.
// Returns handled=false when the template is empty (so the Router can try
// the next route), and handled=true otherwise.
func serveTemplate(w io.ReaderFrom, log logr.Logger, span trace.Span, filename, template string) (bool, error) {
	if template == "" {
		log.Info("template is empty; skipping")
		return false, nil
	}
	bytesSent, err := w.ReadFrom(bytes.NewReader([]byte(template)))
	if err != nil {
		log.Error(err, "serving template failed", "bytesSent", bytesSent)
		span.SetStatus(codes.Error, err.Error())
		return true, err
	}
	log.Info("template served", "bytesSent", bytesSent, "templateSize", len(template))
	span.SetStatus(codes.Ok, filename)
	return true, nil
}
