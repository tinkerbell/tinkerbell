package logger

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
)

// Middleware creates a gin middleware that logs requests. It includes client_ip, method,
// status_code, path and latency.
func Middleware(logger logr.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Process the request recording how long it took.
		start := time.Now()
		c.Next()
		end := time.Now()

		// Build the path including query and fragment portions.
		var b strings.Builder
		b.WriteString(c.Request.URL.Path)
		if c.Request.URL.RawQuery != "" {
			b.WriteString("?")
			b.WriteString(c.Request.URL.RawQuery)
		}
		if c.Request.URL.RawFragment != "" {
			b.WriteString("#")
			b.WriteString(c.Request.URL.RawFragment)
		}
		path := b.String()

		// Build an event with all the values we want to include.
		event := logger.WithValues(
			"client_ip", c.ClientIP(),
			"method", c.Request.Method,
			"status_code", c.Writer.Status(),
			"path", path,
			"latency", end.Sub(start),
		)

		// If we received a non-error status code Info else error it.
		if c.Writer.Status() < 500 {
			event.Info("")
		} else {
			msg := "No error message specified"
			errs := strings.Join(c.Errors.Errors(), "; ")
			if len(c.Errors.Errors()) > 0 {
				msg = c.Errors.Errors()[0]
			}

			event.Error(errors.New(msg), "all_errors", errs)
		}
	}
}
