package binary

import (
	"bytes"
	"context"
	"io"
	"path"
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
//   - Matches the inline files on the cleaned path, so the doubled-slash
//     form the bootloader also asks for ("<SerialNum>//config.txt") is
//     answered as well as the single-slash one.
//
// Returns handled=false when there's no Hardware match, netboot is not
// allowed for it, no RPI config on the Hardware, no AssetDir, the path
// doesn't have the serial prefix, or the rewritten on-disk file does not
// exist.
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
		// An IP with no matching Hardware is an expected fall-through (this
		// route is tried for every client), not a fatal error; log quietly so
		// routine misses don't spam error logs or trip alerting.
		log.V(1).Info("failed to get hardware by IP; skipping", "err", err)
		return false, nil
	}

	// Netboot has to be allowed, the same gate the iPXE script handler
	// applies. AllowNetboot is what the Hardware's netboot.allowPXE becomes
	// once dhcp.Convert* has translated it into hardware.Info.
	//
	// Without this the route serves boot files forever: a machine that has
	// just been provisioned reboots, the Workflow controller clears allowPXE
	// (bootOptions.toggleAllowNetboot), and this route hands it the OSIE again
	// instead of letting it fall through its BOOT_ORDER to the disk it was
	// just installed to. It never boots the installed OS.
	if !hw.AllowNetboot {
		log.V(1).Info("hardware does not allow netboot; skipping")
		return false, nil
	}

	rpi := hw.RPI
	if rpi.SerialNum == "" || rpi.FirmwarePath == "" {
		// Expected fall-through: this route is tried for every client and most
		// Hardware won't set RPI. Keep it quiet so it doesn't spam logs.
		log.V(1).Info("hardware does not have RPI data; skipping")
		return false, nil
	}

	if !strings.HasPrefix(req.Filename, rpi.SerialNum+"/") {
		log.V(1).Info("request path does not begin with SerialNum; skipping", "serialNum", rpi.SerialNum)
		return false, nil
	}

	suffix := req.Filename[len(rpi.SerialNum):]

	// Compared cleaned, because the Raspberry Pi 5 bootloader asks for each of
	// these TWICE: once as "<serial>/config.txt" and again as
	// "<serial>//config.txt", with a doubled slash. Both are the same request.
	// An exact match on req.Filename answers only the first, and it is the
	// second that the firmware acts on -- when it misses, the Pi discards the
	// config it was just served, probes the default kernel names
	// (kernel_2712.img, kernel8.img, kernel8_rt.img), finds nothing and loops
	// forever.
	//
	// This is observed behaviour of the Pi 5 bootloader specifically, seen on a
	// Pi 5 booting a custom OSIE. It has not been observed on other models, and
	// no such behaviour is documented by Raspberry Pi. Cleaning the comparison
	// is a no-op for a client that only ever sends the single-slash form, so
	// the route behaves identically for models that do not do this.
	//
	// The miss is close to invisible: this route simply returns handled=false,
	// the router falls through, and the client gets a 404 for a file it has
	// already been told exists. What it looks like from outside is a Pi that
	// fetched a few files and then went quiet.
	//
	// Only the comparison is normalised. The rewritten path below keeps the raw
	// suffix, for a reason worth spelling out: os.OpenRoot already resolves the
	// doubled slash for real files, so cleaning buys nothing there -- but it
	// costs something. OpenRoot refuses a path that escapes the root, and
	// "<FirmwarePath>/../../../etc/passwd" does. Cleaning the suffix first
	// rewrites that request to "/etc/passwd", making the rewritten path
	// "<FirmwarePath>/etc/passwd", which is inside the root and served if it
	// happens to exist. Normalising here would convert a refusal into a hit.
	switch path.Clean(suffix) {
	case "/config.txt":
		log.Info("serving RPI ConfigTxt")
		return serveTemplate(w, log, span, req.Filename, rpi.ConfigTxt)
	case "/cmdline.txt":
		cmdline := strings.Join(hw.OSIE.KernelParams, " ")
		log.Info("serving cmdline.txt from OSIE.KernelParams", "params", hw.OSIE.KernelParams)
		return serveTemplate(w, log, span, req.Filename, cmdline)
	}

	// The rewritten path mixes hardware-supplied FirmwarePath with the
	// client-supplied suffix; openAsset confines it to AssetDir via os.OpenRoot,
	// so neither can escape the directory. Note OpenRoot refuses a path that
	// leaves the root, not every ".." -- one that resolves back inside is served
	// -- which is the distinction the comment above depends on.
	rewritten := rpi.FirmwarePath + suffix
	log.V(1).Info("attempting to load rewritten file from asset dir", "rewritten", rewritten, "assetDir", r.AssetDir)

	file, err := openAsset(r.AssetDir, rewritten)
	if err != nil {
		// Expected fall-through (404), like DiskAssetRoute; log at V(1).
		log.V(1).Info("rewritten asset not found on disk; skipping", "rewritten", rewritten, "err", err)
		return false, nil
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Error(cerr, "failed to close file", "assetPath", file.Name())
		}
	}()

	bytesSent, err := w.ReadFrom(file)
	if err != nil {
		log.Error(err, "serving rewritten asset failed", "assetPath", file.Name(), "bytesSent", bytesSent)
		span.SetStatus(codes.Error, err.Error())
		return true, err
	}
	log.Info("rewritten asset served from disk", "assetPath", file.Name(), "bytesSent", bytesSent)
	span.SetStatus(codes.Ok, req.Filename)
	return true, nil
}

// serveTemplate writes a hardware-supplied template to the TFTP writer.
// Returns handled=false when the template is empty (so the Router can try
// the next route), and handled=true otherwise.
func serveTemplate(w io.ReaderFrom, log logr.Logger, span trace.Span, filename, template string) (bool, error) {
	if template == "" {
		// Expected fall-through (handled=false): an unset config.txt/cmdline.txt
		// is common and can be high-volume during boot, so keep it at V(1) like
		// the other fall-through paths in this route.
		log.V(1).Info("template is empty; skipping")
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
