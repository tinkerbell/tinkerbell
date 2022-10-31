/*
Package handler defines the HTTP handler for Hegel.
*/
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/packethost/pkg/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tinkerbell/hegel/internal/hardware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// API is an API identifier.
type API int

const (
	EC2 API = iota
	Hegel
)

// New creates a new handler with routes for configured for the desired API.
func New(logger log.Logger, api API, backend hardware.Client) (http.Handler, error) {
	var mux http.ServeMux
	var handler http.Handler

	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/healthz", healthCheckHandler(logger, backend, time.Now()))
	mux.Handle("/versionz", versionHandler(logger))

	if api == EC2 {
		ec2MetadataHandler := otelhttp.WithRouteTag("/2009-04-04", ec2MetadataHandler(logger, backend))
		mux.Handle("/2009-04-04/", ec2MetadataHandler)
		mux.Handle("/2009-04-04", ec2MetadataHandler)

		metadataHandler := otelhttp.WithRouteTag("/metadata", getMetadataHandler(logger, backend, ".metadata.instance", backend.GetDataModel()))
		mux.Handle("/metadata", metadataHandler)

		handler = &mux
	} else {
		router := gin.Default()
		router.RedirectTrailingSlash = true

		v0 := router.Group("/v0")
		v0HegelMetadataHandler(logger, backend, v0)

		handler = router
	}

	return handler, nil
}
