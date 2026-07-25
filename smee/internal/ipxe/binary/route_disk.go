package binary

import (
	"context"
	"io"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// DiskAssetRoute streams files from a local directory. The requested path is
// opened relative to Dir via openAsset, which confines access to Dir and
// rejects directory traversal (absolute paths, ".." segments, escaping
// symlinks).
//
// Returns handled=false when Dir is unset or the file does not exist (or the
// path escaped Dir), so the Router can continue to a later route or fall
// through to the 404 response.
type DiskAssetRoute struct {
	Log logr.Logger
	Dir string
}

func (r DiskAssetRoute) Name() string { return "disk-asset" }

func (r DiskAssetRoute) TryServe(ctx context.Context, req Request, w io.ReaderFrom) (bool, error) {
	if r.Dir == "" {
		return false, nil
	}
	log := r.Log.WithValues("route", r.Name(), "filename", req.Filename)
	span := trace.SpanFromContext(ctx)

	file, err := openAsset(r.Dir, req.Filename)
	if err != nil {
		// As the usual final fall-through route, a missing asset (or a rejected
		// traversal attempt) is an expected, potentially frequent 404; log at
		// V(1) so routine misses don't spam logs. The Router's own V(1) "did
		// not handle" line covers the fall-through when needed.
		log.V(1).Info("asset not found on disk; skipping", "dir", r.Dir, "err", err)
		return false, nil
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Error(cerr, "failed to close file", "assetPath", file.Name())
		}
	}()

	bytesSent, err := w.ReadFrom(file)
	if err != nil {
		log.Error(err, "file serve failed", "assetPath", file.Name(), "bytesSent", bytesSent)
		span.SetStatus(codes.Error, err.Error())
		return true, err
	}
	log.Info("file served from disk", "assetPath", file.Name(), "bytesSent", bytesSent)
	span.SetStatus(codes.Ok, req.Filename)
	return true, nil
}
