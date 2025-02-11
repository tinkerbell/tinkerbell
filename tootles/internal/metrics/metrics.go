package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	routeLabel      = "route"
	methodLabel     = "method"
	statusCodeLabel = "status_code"
)

// InstrumentRequestCount adds a CounterVec to registrar and returns a handler that increments
// the count with every request.
func InstrumentRequestCount(registrar prometheus.Registerer) gin.HandlerFunc {
	m := promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_server_requests_total",
			Help: "Count of HTTP requests",
		},
		[]string{methodLabel, statusCodeLabel},
	)

	registrar.MustRegister(m)

	return func(ctx *gin.Context) {
		ctx.Next()
		m.WithLabelValues(
			ctx.Request.Method,
			strconv.Itoa(ctx.Writer.Status()),
		).Inc()
	}
}

// InstrumentReuqestDuration adds a HistogramVec to registrar and returns a handler that records
// request durations with every request.
func InstrumentRequestDuration(registrar prometheus.Registerer) gin.HandlerFunc {
	m := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_server_request_duration_seconds",
			Help:    "Histogram of response time for HTTP requests in seconds",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{routeLabel, methodLabel, statusCodeLabel},
	)

	registrar.MustRegister(m)

	return func(ctx *gin.Context) {
		start := time.Now()
		ctx.Next()
		m.WithLabelValues(
			ctx.FullPath(),
			ctx.Request.Method,
			strconv.Itoa(ctx.Writer.Status()),
		).Observe(time.Since(start).Seconds())
	}
}
