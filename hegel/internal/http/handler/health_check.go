package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/packethost/pkg/log"
	"github.com/tinkerbell/hegel/internal/build"
)

// HealthChecker provide health checking behavior for services.
type healthChecker interface {
	IsHealthy(context.Context) bool
}

// HealthCheckHandler provides an http handler that exposes health check information to consumers.
// The data is exposed as a json payload containing git_rev, uptim, goroutines and hardware_client_status.
func healthCheckHandler(logger log.Logger, client healthChecker, start time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIsHealthy := client.IsHealthy(r.Context())

		res := struct {
			GitRev                  string  `json:"git_rev"`
			Uptime                  float64 `json:"uptime"`
			Goroutines              int     `json:"goroutines"`
			HardwareClientAvailable bool    `json:"hardware_client_status"`
		}{
			GitRev:                  build.GetGitRevision(),
			Uptime:                  time.Since(start).Seconds(),
			Goroutines:              runtime.NumGoroutine(),
			HardwareClientAvailable: clientIsHealthy,
		}

		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)

		if err := encoder.Encode(&res); err != nil {
			logger.Error(err, "Failed to write for healthChecker")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !clientIsHealthy {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})
}
