package binary

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/go-logr/logr"
)

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
	for _, route := range r.Routes {
		handled, err := route.TryServe(ctx, req, w)
		if handled {
			return err
		}
		r.Log.V(1).Info("route did not handle request", "route", route.Name(), "filename", req.Filename)
	}
	return fmt.Errorf("file [%v] unknown: %w", req.Base, os.ErrNotExist)
}
