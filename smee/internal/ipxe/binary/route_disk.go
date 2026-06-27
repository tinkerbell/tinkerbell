package binary

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// DiskAssetRoute streams files from a local directory. The requested
// path is joined with Dir via filepath.Join, which collapses ".." and
// other traversal attempts.
//
// Returns handled=false when Dir is unset or the file does not exist,
// so the Router can continue to a later route or fall through to the
// 404 response.
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

	assetPath := filepath.Join(r.Dir, req.Filename)
	file, err := os.Open(assetPath)
	if err != nil {
		log.Info("asset not found on disk; skipping", "assetPath", assetPath, "err", err)
		return false, nil
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Error(cerr, "failed to close file", "assetPath", assetPath)
		}
	}()

	bytesSent, err := w.ReadFrom(file)
	if err != nil {
		log.Error(err, "file serve failed", "assetPath", assetPath, "bytesSent", bytesSent)
		span.SetStatus(codes.Error, err.Error())
		return true, err
	}
	log.Info("file served from disk", "assetPath", assetPath, "bytesSent", bytesSent)
	span.SetStatus(codes.Ok, req.Filename)
	return true, nil
}
