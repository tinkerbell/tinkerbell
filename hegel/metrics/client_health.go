package metrics

import (
	"context"
	"errors"
	"time"

	"github.com/packethost/pkg/log"
)

const DefaultTrackClientHealthPollInterval = 15 * time.Second

// HealthChecker checks the health of a construct.
type HealthChecker interface {
	IsHealthy(context.Context) bool
}

// TrackClientHealth tracks client's health status. It updates CacherConnected and CacherHEalthcheck metrics.
func TrackClientHealth(ctx context.Context, logger log.Logger, interval time.Duration, client HealthChecker) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			isHealthy := client.IsHealthy(ctx)
			lgr := logger.With("status", isHealthy)

			if isHealthy {
				CacherConnected.Set(1)
				CacherHealthcheck.WithLabelValues("true").Inc()
				lgr.Debug("tick")
			} else {
				CacherConnected.Set(0)
				CacherHealthcheck.WithLabelValues("false").Inc()
				Errors.WithLabelValues("cacher", "healthcheck").Inc()
				lgr.Error(errors.New("client reported unhealthy"))
			}
		case <-ctx.Done():
			return
		}
	}
}
