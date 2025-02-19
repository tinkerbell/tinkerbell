package middleware

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
)

// Logging creates a gin middleware that logs requests. It includes client_ip, method,
// status_code, path and latency.
func Logging(logger logr.Logger) gin.HandlerFunc {
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

		// Build an log with all the values we want to include.
		log := logger.WithValues(
			"clientIpXff", c.ClientIP(),
			"clientIpPort", c.Request.RemoteAddr,
			"method", c.Request.Method,
			"statusCode", c.Writer.Status(),
			"path", path,
			"latencyNanoseconds", end.Sub(start),
			"latencyHuman", end.Sub(start).String(),
		)

		// If we received a non-error status code Info else error it.
		if c.Writer.Status() < 500 {
			if c.Errors.Errors() != nil {
				log.Error(errors.New("errors occurred"), "all errors", "errs", strings.Join(c.Errors.Errors(), "; "))
			}
			log.Info("request received")
		} else {
			msg := "no errors occurred"
			errs := strings.Join(c.Errors.Errors(), "; ")
			if len(c.Errors.Errors()) > 0 {
				msg = "errors occurred"
			}

			log.Error(errors.New(msg), "all errors", "errs", errs)
		}
	}
}
