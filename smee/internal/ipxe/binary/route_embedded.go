package binary

import (
	"bytes"
	"context"
	"io"

	"github.com/go-logr/logr"
	binaryfile "github.com/tinkerbell/tinkerbell/smee/internal/ipxe/binary/file"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// EmbeddedIPXERoute serves iPXE binaries from the embedded file map
// (smee/internal/ipxe/binary/file). Hits the route on filename basename match;
// if Patch is set, the binary is patched before serving.
type EmbeddedIPXERoute struct {
	Log   logr.Logger
	Patch []byte
}

func (r EmbeddedIPXERoute) Name() string { return "embedded-ipxe" }

func (r EmbeddedIPXERoute) TryServe(ctx context.Context, req Request, w io.ReaderFrom) (bool, error) {
	span := trace.SpanFromContext(ctx)
	log := r.Log.WithValues("route", r.Name(), "filename", req.Filename, "base", req.Base)

	content, ok := binaryfile.Files[req.Base]
	if !ok {
		return false, nil
	}

	patched, err := binaryfile.Patch(content, r.Patch)
	if err != nil {
		log.Error(err, "failed to patch binary")
		span.SetStatus(codes.Error, err.Error())
		return true, err
	}

	bytesSent, err := w.ReadFrom(bytes.NewReader(patched))
	if err != nil {
		log.Error(err, "file serve failed", "bytesSent", bytesSent, "contentSize", len(patched))
		span.SetStatus(codes.Error, err.Error())
		return true, err
	}

	log.Info("file served", "bytesSent", bytesSent, "contentSize", len(patched))
	span.SetStatus(codes.Ok, req.Base)
	return true, nil
}
