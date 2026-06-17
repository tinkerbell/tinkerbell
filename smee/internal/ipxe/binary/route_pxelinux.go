package binary

import (
	"bytes"
	"context"
	"io"
	"net"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/smee/internal/hardware"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// PXELinuxMACRoute handles PXELinux requests of the exact form
// "pxelinux.cfg/01-AA-BB-CC-DD-EE-FF", where the trailing token is a
// dash-separated MAC address (parsed via net.ParseMAC, so either case is
// accepted). The MAC is extracted from the path, used to look up Hardware,
// and the Hardware's PXELINUX.Config is served.
//
// The route returns handled=false when the path doesn't have that exact
// shape, when MAC parsing fails, when the Hardware lookup fails, or when
// the matched Hardware has no PXELINUX.Config. In all of those cases
// the next Route in the Router gets a chance.
type PXELinuxMACRoute struct {
	Log      logr.Logger
	Resolver hardware.Resolver
}

const (
	pxelinuxFullMACPrefix    = "pxelinux.cfg/01-"
	pxelinuxFullMACPrefixLen = len(pxelinuxFullMACPrefix)
	pxelinuxMACDashedLen     = len("00-00-00-00-00-00")
	pxeLinuxFullLen          = pxelinuxFullMACPrefixLen + pxelinuxMACDashedLen
)

func (r PXELinuxMACRoute) Name() string { return "pxelinux-mac" }

func (r PXELinuxMACRoute) TryServe(ctx context.Context, req Request, w io.ReaderFrom) (bool, error) {
	if len(req.Filename) != pxeLinuxFullLen || req.Filename[:pxelinuxFullMACPrefixLen] != pxelinuxFullMACPrefix {
		return false, nil
	}

	log := r.Log.WithValues("route", r.Name(), "filename", req.Filename)
	span := trace.SpanFromContext(ctx)

	macStr := req.Filename[pxelinuxFullMACPrefixLen:]
	mac, err := net.ParseMAC(macStr)
	if err != nil {
		log.Error(err, "failed to parse MAC from pxelinux.cfg request", "macStr", macStr)
		return false, nil
	}

	hw, err := r.Resolver.ByMAC(ctx, mac)
	if err != nil {
		log.Error(err, "failed to get hardware by MAC", "mac", mac.String())
		return false, nil
	}

	if hw.PXELINUX.Config == "" {
		log.Info("no PXELINUX config in hardware; skipping", "mac", mac.String())
		return false, nil
	}

	bytesSent, err := w.ReadFrom(bytes.NewReader([]byte(hw.PXELINUX.Config)))
	if err != nil {
		log.Error(err, "serving PXELINUX config failed", "bytesSent", bytesSent)
		span.SetStatus(codes.Error, err.Error())
		return true, err
	}

	log.Info("PXELINUX config served", "bytesSent", bytesSent)
	span.SetStatus(codes.Ok, req.Filename)
	return true, nil
}
