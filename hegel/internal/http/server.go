package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/packethost/pkg/log"
)

// Serve is a blocking call that begins serving the provided handler on port. When ctx is cancelled
// it will attempt to gracefully shutdown. If graceful shutdown fails, it will force shutdown
// and return an error.
func Serve(ctx context.Context, logger log.Logger, address string, handler http.Handler) error {
	server := http.Server{
		Addr:    address,
		Handler: handler,

		// Mitigate Slowloris attacks. 30 seconds is based on Apache's recommended 20-40
		// recommendation. Hegel doesn't really have many headers so 20s should be plenty of time.
		// https://en.wikipedia.org/wiki/Slowloris_(computer_security)
		ReadHeaderTimeout: 20 * time.Second,
	}

	go func() {
		logger.Info(fmt.Sprintf("Listening on %s", address))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Info(err.Error())
		}
	}()

	// Wait until we're told to shutdown.
	<-ctx.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt a graceful shutdown with timeout.
	if err := server.Shutdown(ctx); err != nil {
		server.Close()

		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("timed out waiting for graceful shutdown")
		}

		return err
	}

	return nil
}
