package server

import (
	"net/http"

	"github.com/go-logr/logr"
	"github.com/tinkerbell/tinkerbell/pkg/http/handler"
)

// Route represents an HTTP route with its pattern, description, and handler.
type Route struct {
	Pattern            string       `json:"pattern"`
	Description        string       `json:"description"`
	RewriteHTTPToHTTPS bool         `json:"rewriteHttpToHttps"`
	HTTPEnabled        bool         `json:"httpEnabled"`
	HTTPSEnabled       bool         `json:"httpsEnabled"`
	Handler            http.Handler `json:"-"`
}

// Routes is a collection of Route objects that can be registered with an HTTP server.
type Routes []Route

func WithHTTPEnabled(enable bool) func(*Route) {
	return func(r *Route) {
		r.HTTPEnabled = enable
	}
}

func WithHTTPSEnabled(enable bool) func(*Route) {
	return func(r *Route) {
		r.HTTPSEnabled = enable
	}
}

func WithRewriteHTTPToHTTPS(enable bool) func(*Route) {
	return func(r *Route) {
		r.RewriteHTTPToHTTPS = enable
	}
}

// Register adds a new route to the Routes collection for later registration with an HTTP server.
// This allows tracking endpoint patterns and their handlers for use with http.ServeMux.Handle.
// Useful for logging information about registered routes and their descriptions.
// HTTP is enabled by default, overridable with options.
func (rs *Routes) Register(pattern string, hh http.Handler, desc string, options ...func(*Route)) {
	if desc == "" {
		desc = "No description provided"
	}

	rt := Route{
		Pattern:     pattern,
		Description: desc,
		Handler:     hh,
		HTTPEnabled: true,
	}
	for _, opt := range options {
		opt(&rt)
	}

	*rs = append(*rs, rt)
}

// Muxes builds and returns separate HTTP and HTTPS [http.ServeMux] instances
// from the registered routes. When tlsEnabled is false, routes marked with
// RewriteHTTPToHTTPS are served normally over HTTP instead of redirecting,
// so endpoints remain reachable in no-TLS deployments.
func (rs *Routes) Muxes(log logr.Logger, httpsPort int, tlsEnabled bool) (*http.ServeMux, *http.ServeMux) {
	httpMux := http.NewServeMux()
	httpsMux := http.NewServeMux()
	for _, route := range *rs {
		if route.HTTPEnabled && route.RewriteHTTPToHTTPS && tlsEnabled {
			httpMux.Handle(route.Pattern, handler.RedirectToHTTPS(log, httpsPort))
		} else if route.HTTPEnabled {
			httpMux.Handle(route.Pattern, route.Handler)
		}
		if route.HTTPSEnabled {
			httpsMux.Handle(route.Pattern, route.Handler)
		}
	}

	return httpMux, httpsMux
}
