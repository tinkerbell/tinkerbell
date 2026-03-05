package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/build"
)

// HealthCheck returns an http.Handler that responds with JSON containing
// the git revision, uptime, and goroutine count. It encodes into a buffer first
// so that encoding errors can be reported with a proper 500 status.
func HealthCheck(log logr.Logger, startTime time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		res := struct {
			GitRev        string `json:"git_rev"`
			UptimeSeconds string `json:"uptime_seconds"`
			Goroutines    int    `json:"goroutines"`
		}{
			GitRev:        build.GitRevision(),
			UptimeSeconds: fmt.Sprintf("%.2f", time.Since(startTime).Seconds()),
			Goroutines:    runtime.NumGoroutine(),
		}
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(&res); err != nil {
			log.Error(err, "failed to encode healthcheck response")
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := buf.WriteTo(w); err != nil {
			log.Error(err, "failed to write healthcheck response")
		}
	})
}

// RedirectToHTTPS returns an http.Handler that redirects incoming HTTP requests to the corresponding HTTPS URL on the specified port.
func RedirectToHTTPS(log logr.Logger, port int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := *r.URL
		u.Scheme = "https"

		u.Host = net.JoinHostPort(parseHost(r.Host), fmt.Sprintf("%d", port))

		log.V(2).Info("redirecting to HTTPS", "host", r.Host, "httpsURL", u.String(), "httpURL", r.URL.String())
		http.Redirect(w, r, u.String(), http.StatusPermanentRedirect)
	})
}

// parseHost splits out the host from the input.
// The input is typically from url.URL.Host or http.Request.Host,
// which can be in the form of "host:port", "ip:port", "host", or "ip".
func parseHost(input string) string {
	host, _, err := net.SplitHostPort(input)
	if err != nil {
		// If SplitHostPort fails, it's usually because the port is missing.
		// In this case, the entire input is treated as the host.
		return input
	}
	return host
}
