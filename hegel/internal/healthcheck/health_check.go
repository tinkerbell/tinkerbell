package healthcheck

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tinkerbell/tinkerbell/hegel/internal/build"
)

// Client defines health check behavior for a service.
type Client interface {
	// IsHealthy returns true if the backend is healthy, else false.
	IsHealthy(context.Context) bool
}

// NewHandler returns a gin.HandlerFunc that provides a health check endpoint behavior. On each
// request it queries client.IsHealthy and returns a 200 if the backend is healthy, else a 500.
func NewHandler(client Client) gin.HandlerFunc {
	start := time.Now()
	return func(ctx *gin.Context) {
		isHealthy := client.IsHealthy(ctx)

		res := struct {
			GitRev                  string  `json:"git_rev"`
			Uptime                  float64 `json:"uptime"`
			Goroutines              int     `json:"goroutines"`
			HardwareClientAvailable bool    `json:"hardware_client_status"`
		}{
			GitRev:                  build.GetGitRevision(),
			Uptime:                  time.Since(start).Seconds(),
			Goroutines:              runtime.NumGoroutine(),
			HardwareClientAvailable: isHealthy,
		}

		status := http.StatusOK
		if !isHealthy {
			status = http.StatusInternalServerError
		}

		ctx.JSON(status, res)
	}
}
