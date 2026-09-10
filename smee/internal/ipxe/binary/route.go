package binary

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/go-logr/logr"
)

// ErrNetbootNotAllowed marks a request that was declined solely because the
// Hardware matching the client has netboot disabled (netboot.allowPXE=false in
// the Hardware, which dhcp.Convert* turns into hardware.Info.AllowNetboot).
//
// It exists to keep that case distinguishable from "this file does not exist".
// In the L3 scenarios (external DHCP, static IPs, DHCP relay) Smee's DHCP
// handler never runs, so the operator never sees the "/netboot-not-allowed"
// boot file name it would otherwise hand out; the first and only signal is
// whatever TFTP/HTTP answers. A plain 404 (or TFTP "file not found") for that
// is indistinguishable from a missing file or a typo'd path and sends people
// hunting through the asset directory instead of at the Hardware record.
//
// Routes return it alongside handled=false so the Router can keep trying later
// routes; if none of them serve the request, the Router returns this instead of
// os.ErrNotExist, and the transports turn it into 403 Forbidden (HTTP) or a
// TFTP error whose message names the cause.
var ErrNetbootNotAllowed = errors.New("netboot is not allowed for this hardware: netboot.allowPXE is false in the Hardware record")

// netbootNotAllowedForMAC/IP wrap ErrNetbootNotAllowed with the identity the
// route matched on, so the message the client is shown (TFTP puts it in the
// ERROR packet, HTTP in the 403 body) names the Hardware to go look at.
func netbootNotAllowedForMAC(mac net.HardwareAddr) error {
	return fmt.Errorf("%w (mac: %v)", ErrNetbootNotAllowed, mac)
}

func netbootNotAllowedForIP(ip net.IP) error {
	return fmt.Errorf("%w (ip: %v)", ErrNetbootNotAllowed, ip)
}

// Request is the parsed TFTP read request handed to each Route.
// Filename is the raw path as received from the client; Base is its basename,
// precomputed so routes that key on the leaf (eg. embedded iPXE) don't all
// repeat the same filepath.Base call.
type Request struct {
	Filename string
	Base     string
	Client   net.UDPAddr
}

// Route is one step in the TFTP read-dispatch chain.
// Returning handled=true means the route owns this request (whether it
// succeeded or returned an error); the Router will not consult later routes.
// Returning handled=false means the request didn't match this route and
// the Router should continue.
//
// A route may return handled=false with a non-nil error to explain why it
// declined. The Router remembers the explanation and returns it if no later
// route serves the request, so a decline with a known cause (currently
// ErrNetbootNotAllowed) surfaces to the client instead of a generic 404.
type Route interface {
	Name() string
	TryServe(ctx context.Context, req Request, w io.ReaderFrom) (handled bool, err error)
}

// Router walks its Routes in order and returns the first handled result,
// or a 404-style error if no route claims the request.
type Router struct {
	Log    logr.Logger
	Routes []Route
}

func (r Router) Handle(ctx context.Context, req Request, w io.ReaderFrom) error {
	// Kept across the loop: a route that declined because netboot is disabled
	// still lets later routes try (the disk asset route may hold a real file by
	// that name), but if the request ends up unserved this is the useful answer,
	// not "not found". The first such reason wins; routes are ordered most
	// specific first, so it's the closest match to what the client asked for.
	var declined error
	for _, route := range r.Routes {
		handled, err := route.TryServe(ctx, req, w)
		if handled {
			return err
		}
		if err != nil {
			if declined == nil && errors.Is(err, ErrNetbootNotAllowed) {
				declined = err
			} else {
				r.Log.V(1).Info("route declined request with an error", "route", route.Name(), "filename", req.Filename, "err", err)
			}
		}
		r.Log.V(1).Info("route did not handle request", "route", route.Name(), "filename", req.Filename)
	}
	if declined != nil {
		return declined
	}
	return fmt.Errorf("file [%v] unknown: %w", req.Base, os.ErrNotExist)
}
