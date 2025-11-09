package tftp

import (
	"io"
	"time"

	"github.com/go-logr/logr"
	"github.com/pin/tftp/v3"
)

// tftpLoggingMiddleware implements tftp.Hook interface for logging TFTP transfer statistics.
type tftpLoggingMiddleware struct {
	log logr.Logger
}

func LogRequest(next Handler, logger logr.Logger) Handler {
	return HandlerFunc(func(filename string, rf io.ReaderFrom) error {
		var (
			start  = time.Now()
			method = "GET" // TFTP only supports GET and PUT; this is a read operation
			uri    = filename
			client = "" // TFTP does not provide client IP in this context
			scheme = "tftp"
		)

		err := next.ServeTFTP(filename, rf) // process the request
		if err == nil {
			logger.Info("response", "scheme", scheme, "method", method, "uri", uri, "client", client, "duration", time.Since(start), "status", "success")
		} else {
			logger.Error(err, "response", "scheme", scheme, "method", method, "uri", uri, "client", client, "duration", time.Since(start), "status", "failure")
		}
		return err
	})
}

// OnSuccess logs successful TFTP transfers.
func (h *tftpLoggingMiddleware) OnSuccess(stats tftp.TransferStats) {
	h.log.Info("tftp transfer successful",
		"filename", stats.Filename,
		"remoteAddr", stats.RemoteAddr.String(),
		"duration", stats.Duration.String(),
		"datagramsSent", stats.DatagramsSent,
		"datagramsAcked", stats.DatagramsAcked,
		"mode", stats.Mode,
		"tid", stats.Tid,
	)
}

// OnFailure logs failed TFTP transfers.
func (h *tftpLoggingMiddleware) OnFailure(stats tftp.TransferStats, err error) {
	h.log.Error(err, "tftp transfer failed",
		"filename", stats.Filename,
		"remoteAddr", stats.RemoteAddr.String(),
		"duration", stats.Duration.String(),
		"datagramsSent", stats.DatagramsSent,
		"datagramsAcked", stats.DatagramsAcked,
		"mode", stats.Mode,
		"tid", stats.Tid,
	)
}
