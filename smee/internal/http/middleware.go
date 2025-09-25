package http

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

func LogRequest(next http.Handler, logger logr.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			start  = time.Now()
			method = r.Method
			uri    = r.RequestURI
			client = clientIP(r.RemoteAddr)
			scheme = func() string {
				if r.TLS != nil {
					return "https"
				}
				return "http"
			}()
		)

		res := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(res, r) // process the request

		// The "X-Global-Logging" header allows all registered HTTP handlers to disable this global logging
		// by setting the header to any non empty string. This is useful for handlers that handle partial content of
		// larger file. The ISO handler, for example.
		if res.Header().Get("X-Global-Logging") == "" {
			logger.Info("response", "scheme", scheme, "method", method, "uri", uri, "client", client, "duration", time.Since(start), "status", res.statusCode)
		}
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = 200
	}
	n, err := w.ResponseWriter.Write(b)
	if err != nil {
		return 0, fmt.Errorf("failed writing response: %w", err)
	}

	return n, nil
}

func (w *responseWriter) WriteHeader(code int) {
	if w.statusCode == 0 {
		w.statusCode = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func clientIP(str string) string {
	host, _, err := net.SplitHostPort(str)
	if err != nil {
		return "?"
	}

	return host
}
