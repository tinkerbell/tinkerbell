package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/go-logr/logr"
)

type HealthCheck struct {
	StartTime time.Time
	GitRev    string
}

func (h HealthCheck) HandlerFunc(log logr.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		res := struct {
			GitRev        string `json:"git_rev"`
			UptimeSeconds string `json:"uptime_seconds"`
			Goroutines    int    `json:"goroutines"`
		}{
			GitRev:        h.GitRev,
			UptimeSeconds: fmt.Sprintf("%.2f", time.Since(h.StartTime).Seconds()),
			Goroutines:    runtime.NumGoroutine(),
		}
		if err := json.NewEncoder(w).Encode(&res); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Error(err, "marshaling healthcheck json")
		}
	}
}
