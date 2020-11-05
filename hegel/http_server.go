package main

import (
	"fmt"
	"net/http"

	"github.com/packethost/hegel/xff"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func ServeHTTP() {
	mux := &http.ServeMux{}
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/_packet/healthcheck", healthCheckHandler)
	mux.HandleFunc("/_packet/version", versionHandler)
	mux.HandleFunc("/2009-04-04", ec2Handler) // workaround for making trailing slash optional
	mux.HandleFunc("/2009-04-04/", ec2Handler)

	buildSubscriberHandlers(hegelServer)

	err := registerCustomEndpoints(mux)
	if err != nil {
		logger.Fatal(err, "could not register custom endpoints")
	}

	trustedProxies := xff.ParseTrustedProxies()
	http.Handle("/", xff.HTTPHandler(logger, mux, trustedProxies))

	logger.With("port", *metricsPort).Info("Starting http server")
	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", *metricsPort), nil)
		if err != nil {
			logger.Fatal(err, "failed to serve http")
		}
	}()
}
