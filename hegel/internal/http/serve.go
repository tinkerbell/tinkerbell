package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

// Serve is a blocking call that begins serving the provided handler on port. When ctx is cancelled
// it will attempt to gracefully shutdown. If graceful shutdown fails, it will force shutdown
// and return an error.
func Serve(ctx context.Context, logger logr.Logger, address string, handler http.Handler) error {
	server := http.Server{
		Addr:    address,
		Handler: handler,

		// Mitigate Slowloris attacks. 20 seconds is based on Apache's recommended 20-40
		// recommendation. Hegel doesn't really have many headers so 20s should be plenty of time.
		// https://en.wikipedia.org/wiki/Slowloris_(computer_security)
		ReadHeaderTimeout: 20 * time.Second,
	}

	errChan := make(chan error, 1)
	go func() {
		logger.Info("starting http server", "bindAddr", address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	// Wait until we're told to shutdown.
	select {
	case <-ctx.Done():
	case e := <-errChan:
		return e
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt a graceful shutdown with timeout.
	//nolint:contextcheck // We can't derive from the original context as it's already done.
	if err := server.Shutdown(ctx); err != nil {
		server.Close()

		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("timed out waiting for graceful shutdown")
		}

		return err
	}

	return nil
}
