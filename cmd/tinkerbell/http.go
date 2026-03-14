package main

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/pkg/http/handler"
	"github.com/tinkerbell/tinkerbell/pkg/http/middleware"
	httpserver "github.com/tinkerbell/tinkerbell/pkg/http/server"
	"github.com/tinkerbell/tinkerbell/smee"
	"github.com/tinkerbell/tinkerbell/tink/server"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	routeMetrics           = "/metrics"
	routeHealthcheck       = "/healthcheck"
	routeHealthz           = "/healthz"
	routeReadyz            = "/readyz"
	routeSmeeMetrics       = "/smee/metrics"
	routeTinkServerMetrics = "/tink-server/metrics"
	routeControllerMetrics = "/controllers/metrics"
	routeHTTPMetrics       = "/http/metrics"
	routeEC2Metadata       = "/2009-04-04/"
	routeTootles           = "/tootles/"
	routeHackMetadata      = "/metadata"
	routeISO               = smee.ISOURI
	routeIPXEBinary        = smee.IPXEBinaryURI
	routeIPXEScript        = smee.IPXEScriptURI
)

// startHTTPServer registers all HTTP/HTTPS routes, applies middleware, and
// starts the consolidated HTTP server. It blocks until ctx is cancelled.
func startHTTPServer(ctx context.Context, globals *flag.GlobalConfig, s *flag.SmeeConfig, h *flag.TootlesConfig, uic *flag.UIConfig, startTime time.Time) error {
	httpLog := getLogger(globals.LogLevel).WithName("http")
	routeList := &httpserver.Routes{}
	tlsEnabled := len(s.Config.TLS.Certs) > 0

	// Smee HTTP handlers
	if globals.EnableSmee {
		ll := ternary((s.LogLevel != 0), s.LogLevel, globals.LogLevel)
		smeeLog := getLogger(ll).WithName("smee")

		if bh := s.Config.BinaryHandler(smeeLog); bh != nil {
			routeList.Register(routeIPXEBinary,
				middleware.WithLogLevel(middleware.LogLevelAlways, bh),
				"smee iPXE binary handler",
			)
		}
		if sh := s.Config.ScriptHandler(smeeLog); sh != nil {
			routeList.Register(routeIPXEScript,
				middleware.WithLogLevel(middleware.LogLevelAlways, sh),
				"smee iPXE script handler",
			)
		}
		if isoH, err := s.Config.ISOHandler(smeeLog); err == nil && isoH != nil {
			routeList.Register(routeISO,
				middleware.WithLogLevel(middleware.LogLevelNever, isoH),
				"smee ISO handler",
				httpserver.WithHTTPSEnabled(tlsEnabled),
			)
		} else if err != nil {
			return fmt.Errorf("failed to create smee iso handler: %w", err)
		}
	}

	// Tootles HTTP handlers
	if globals.EnableTootles {
		ec2H := middleware.WithLogLevel(middleware.LogLevelAlways, h.Config.EC2MetadataHandler())
		routeList.Register(routeEC2Metadata,
			ec2H,
			"EC2 metadata handler",
			httpserver.WithHTTPSEnabled(tlsEnabled),
			httpserver.WithRewriteHTTPToHTTPS(tlsEnabled),
		)
		if h.Config.InstanceEndpoint {
			routeList.Register(routeTootles,
				ec2H,
				"EC2 instance endpoint handler",
				httpserver.WithHTTPSEnabled(tlsEnabled),
				httpserver.WithRewriteHTTPToHTTPS(tlsEnabled),
			)
		}
		routeList.Register(routeHackMetadata,
			middleware.WithLogLevel(middleware.LogLevelAlways, h.Config.HackMetadataHandler()),
			"Hack metadata handler",
			httpserver.WithHTTPSEnabled(tlsEnabled),
			httpserver.WithRewriteHTTPToHTTPS(tlsEnabled),
		)
	}

	// UI HTTP handler
	if globals.EnableUI {
		ll := ternary((uic.LogLevel != 0), uic.LogLevel, globals.LogLevel)
		uiLog := getLogger(ll).WithName("ui")

		uiHandler, err := uic.Config.Handler(uiLog)
		if err != nil {
			return fmt.Errorf("failed to create ui handler: %w", err)
		}
		if uiHandler != nil {
			uiLog.Info("UI handler enabled", "urlPrefix", uic.Config.URLPrefix)
			routeUI := normalizeURLPrefix(uic.Config.URLPrefix)
			routeList.Register(routeUI,
				middleware.WithLogLevel(middleware.LogLevelDebug, uiHandler),
				"UI handler",
				httpserver.WithHTTPSEnabled(tlsEnabled),
				httpserver.WithRewriteHTTPToHTTPS(tlsEnabled),
			)
		}
	}

	// Per-service and combined metrics endpoints.
	gatherers := prometheus.Gatherers{prometheus.DefaultGatherer}

	if globals.EnableSmee {
		gatherers = append(gatherers, smee.MetricsRegistry())
		routeList.Register(routeSmeeMetrics,
			middleware.WithLogLevel(middleware.LogLevelNever, promhttp.HandlerFor(smee.MetricsRegistry(), promhttp.HandlerOpts{})),
			"Smee metrics handler",
		)
	}
	if globals.EnableTinkServer {
		gatherers = append(gatherers, server.Registry)
		routeList.Register(routeTinkServerMetrics,
			middleware.WithLogLevel(middleware.LogLevelNever, promhttp.HandlerFor(server.Registry, promhttp.HandlerOpts{})),
			"Tink server metrics handler",
		)
	}
	if globals.EnableTinkController || globals.EnableRufio {
		// controller-runtime's registry registers its own GoCollector and
		// ProcessCollector. Those duplicate the collectors already present
		// in prometheus.DefaultGatherer, so strip them for the combined
		// endpoint to avoid "collected before with the same name" errors.
		gatherers = append(gatherers, filteredGatherer{
			gatherer: crmetrics.Registry,
			excluded: []string{"go_", "process_"},
		})
		routeList.Register(routeControllerMetrics,
			middleware.WithLogLevel(middleware.LogLevelNever, promhttp.HandlerFor(crmetrics.Registry, promhttp.HandlerOpts{})),
			"Controller-runtime metrics handler (tink-controller + rufio)",
		)
	}
	gatherers = append(gatherers, middleware.Registry)
	routeList.Register(routeHTTPMetrics,
		middleware.WithLogLevel(middleware.LogLevelNever, promhttp.HandlerFor(middleware.Registry, promhttp.HandlerOpts{})),
		"HTTP middleware metrics handler",
	)

	routeList.Register(routeMetrics,
		middleware.WithLogLevel(middleware.LogLevelNever, promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{})),
		"Combined Prometheus metrics handler",
	)

	routeList.Register(routeHealthcheck, middleware.WithLogLevel(middleware.LogLevelNever, handler.HealthCheck(httpLog, startTime)), "Healthcheck handler")
	routeList.Register(routeHealthz, middleware.WithLogLevel(middleware.LogLevelNever, handler.Healthz()), "Liveness probe handler")
	routeList.Register(routeReadyz, middleware.WithLogLevel(middleware.LogLevelNever, handler.Readyz()), "Readiness probe handler")

	httpMux, httpsMux := routeList.Muxes(httpLog, globals.HTTPSPort, !globals.TLS.DisableHTTPToHTTPSRedirect && tlsEnabled)

	// Only wrap and pass the HTTPS handler when there are HTTPS routes
	// (which implies TLS is configured) — otherwise skip the HTTPS server.
	var httpsArg http.Handler
	if routeList.HasHTTPSRoutes() {
		httpsArg = httpsMux
	}

	httpHandler, httpsHandler, err := addMiddleware(httpLog, globals.TrustedProxies, httpMux, httpsArg)
	if err != nil {
		return fmt.Errorf("failed to add middleware: %w", err)
	}

	opts := []httpserver.Option{
		func(c *httpserver.Config) {
			c.BindAddr = globals.BindAddr.String()
			c.BindPort = globals.HTTPPort
			c.HTTPSPort = globals.HTTPSPort
			c.TLSCerts = s.Config.TLS.Certs
		},
	}
	srv := httpserver.NewConfig(opts...)

	kvs := []any{
		"addr", fmt.Sprintf("%s:%d", globals.BindAddr.String(), globals.HTTPPort),
		"enabledSchemes", func() []string {
			schemes := []string{"http"}
			if httpsHandler != nil {
				schemes = append(schemes, "https")
			}
			return schemes
		}(),
		"registeredRoutes", routeList,
	}
	httpLog.Info("starting HTTP server", kvs...)
	return srv.Serve(ctx, httpLog, httpHandler, httpsHandler)
}

// addMiddleware applies the shared middleware stack to both the HTTP and
// HTTPS handlers. The middleware executes in the following order on the
// request path:
//
//	Request  → SourceIP → XFF → RequestMetrics → Recovery → Logging → OTel → mux
//	Response ← SourceIP ← XFF ← RequestMetrics ← Recovery ← Logging ← OTel ← mux
func addMiddleware(log logr.Logger, trustedProxies []netip.Prefix, httpHandler, httpsHandler http.Handler) (http.Handler, http.Handler, error) {
	// Convert trusted proxies once for both handlers.
	var proxies []string
	for _, p := range trustedProxies {
		proxies = append(proxies, p.String())
	}

	// RequestMetrics uses sync.Once internally, so calling it twice is
	// safe — metrics are registered only on the first call.
	metrics := middleware.RequestMetrics()

	wrap := func(h http.Handler, otelName string) (http.Handler, error) {
		h = middleware.OTel(otelName)(h)
		h = middleware.Logging(log)(h)
		h = middleware.Recovery(log)(h)
		h = metrics(h)
		if len(proxies) > 0 {
			xff, err := middleware.XFF(proxies)
			if err != nil {
				return nil, fmt.Errorf("failed to create XFF middleware: %w", err)
			}
			h = xff(h)
		}
		h = middleware.SourceIP()(h)
		return h, nil
	}

	var err error
	httpHandler, err = wrap(httpHandler, "tinkerbell-http")
	if err != nil {
		return nil, nil, err
	}
	if httpsHandler != nil {
		httpsHandler, err = wrap(httpsHandler, "tinkerbell-https")
		if err != nil {
			return nil, nil, err
		}
	}

	return httpHandler, httpsHandler, nil
}

// filteredGatherer wraps a prometheus.Gatherer and drops any MetricFamily
// whose name starts with one of the excluded prefixes.
type filteredGatherer struct {
	gatherer prometheus.Gatherer
	excluded []string
}

func (f filteredGatherer) Gather() ([]*dto.MetricFamily, error) {
	mfs, err := f.gatherer.Gather()
	filtered := mfs[:0]
	for _, mf := range mfs {
		skip := false
		for _, prefix := range f.excluded {
			if strings.HasPrefix(mf.GetName(), prefix) {
				skip = true
				break
			}
		}
		if !skip {
			filtered = append(filtered, mf)
		}
	}
	return filtered, err
}
