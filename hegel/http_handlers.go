package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/packethost/hegel/metrics"
)

func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write(gitRevJSON)
	if err != nil {
		logger.Error(err, " Failed to write gitRevJSON")
	}
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	isCacherAvailableMu.RLock()
	isCacherAvailableTemp := isCacherAvailable
	isCacherAvailableMu.RUnlock()

	res := struct {
		GitRev          string  `json:"git_rev"`
		Uptime          float64 `json:"uptime"`
		Goroutines      int     `json:"goroutines"`
		CacherAvailable bool    `json:"cacher_status"`
	}{
		GitRev:          gitRev,
		Uptime:          time.Since(StartTime).Seconds(),
		Goroutines:      runtime.NumGoroutine(),
		CacherAvailable: isCacherAvailableTemp,
	}
	b, err := json.Marshal(&res)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	if !isCacherAvailableTemp {
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(b)
	if err != nil {
		logger.Error(err, " Failed to write for healthChecker")
	}
}

func getMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		logger.Debug("Calling getMetadata ")
		userIP := getIPFromRequest(r)
		if userIP != "" {
			metrics.MetadataRequests.Inc()
			logger.With("userIP", userIP).Info("Actual IP is : ")
			ehw, err := getByIP(context.Background(), hegelServer, userIP)
			if err != nil {
				metrics.Errors.WithLabelValues("metadata", "lookup").Inc()
				logger.Info("Error in finding or exporting hardware ", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			_, err = w.Write(ehw)
			if err != nil {
				logger.Error(err, "failed to write Metadata")
			}
		}
	}
}

func getIPFromRequest(r *http.Request) string {
	IPAddress := r.RemoteAddr
	if strings.ContainsRune(IPAddress, ':') {
		IPAddress, _, _ = net.SplitHostPort(IPAddress)
	}
	return IPAddress
}
