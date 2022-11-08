package zpages

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tinkerbell/hegel/internal/healthcheck"
)

// Configure configures router with Hegel specific z-page endpoints.
func Configure(router gin.IRouter, backend healthcheck.Client) {
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	router.GET("/healthz", healthcheck.NewHandler(backend, time.Now()))
}
