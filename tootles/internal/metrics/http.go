package metrics

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Configure configures router with a /metrics endpoint that serves prometheus metrics sourced from
// registry.
func Configure(router gin.IRouter, registry *prometheus.Registry) {
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{Registry: registry})
	router.GET("/metrics", gin.WrapH(handler))
}
