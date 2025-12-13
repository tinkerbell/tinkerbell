// package http handles serving HTTP(s) requests, HTTP middleware, and defines common handlers.
package http

import (
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/xff"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	healthCheckURI = "/healthcheck"
	metricsURI     = "/metrics"
)

// HandlerMapping is a map of routes to http.HandlerFuncs.
type HandlerMapping map[string]http.HandlerFunc

func createHandler(log logr.Logger, otelOperation string, trustedProxies []string, handlers HandlerMapping) (http.Handler, error) {
	mux := http.NewServeMux()
	for pattern, handler := range handlers {
		// we don't log healthcheck or metrics requests because they are generally called very frequently.
		if pattern == healthCheckURI || pattern == metricsURI {
			mux.Handle(pattern, handler)
			continue
		}
		// otelhttp.WithRouteTag takes a route and an http handler function, wraps the function
		// with otelhttp, and returns the route again and http.Handler all set for mux.Handle().
		mux.Handle(pattern, LogRequest(handler, log))
	}

	// wrap the mux with an OpenTelemetry interceptor
	otelHandler := otelhttp.NewHandler(mux, otelOperation)

	// add X-Forwarded-For support if trusted proxies are configured
	var xffHandler http.Handler
	if len(trustedProxies) > 0 {
		xffmw, err := xff.NewXFF(xff.Options{
			AllowedSubnets: trustedProxies,
		})
		if err != nil {
			log.Error(err, "failed to create new xff object")
			return nil, fmt.Errorf("failed to create new xff object: %w", err)
		}

		xffHandler = xffmw.Handler(otelHandler)
	} else {
		xffHandler = otelHandler
	}
	return xffHandler, nil
}
