package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"path"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/crd"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/pkg/build"
	"github.com/tinkerbell/tinkerbell/pkg/http/handler"
	"github.com/tinkerbell/tinkerbell/pkg/http/middleware"
	httpserver "github.com/tinkerbell/tinkerbell/pkg/http/server"
	"github.com/tinkerbell/tinkerbell/pkg/otel"
	"github.com/tinkerbell/tinkerbell/rufio"
	"github.com/tinkerbell/tinkerbell/secondstar"
	"github.com/tinkerbell/tinkerbell/smee"
	"github.com/tinkerbell/tinkerbell/tink/controller"
	"github.com/tinkerbell/tinkerbell/tink/server"
	"github.com/tinkerbell/tinkerbell/tootles"
	"github.com/tinkerbell/tinkerbell/ui"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/credentials"
	"k8s.io/client-go/rest"
)

const (
	defaultRufioMetricsPort          = 8082
	defaultRufioProbePort            = 8083
	defaultSecondStarPort            = 2222
	defaultHTTPPort                  = 7171
	defaultHTTPSPort                 = 7272
	defaultTinkControllerMetricsPort = 8080
	defaultTinkControllerProbePort   = 8081
	defaultTinkServerPort            = 42113
	routeMetrics                     = "/metrics"
	routeHealthcheck                 = "/healthcheck"
	routeEC2Metadata                 = "/2009-04-04/"
	routeTootles                     = "/tootles/"
	routeHackMetadata                = "/metadata"
	routeISO                         = smee.ISOURI
	routeIPXEBinary                  = smee.IPXEBinaryURI
	routeIPXEScript                  = smee.IPXEScriptURI
)

var (
	embeddedFlagSet                      *ff.FlagSet
	embeddedApiserverExecute             func(context.Context, logr.Logger) error
	embeddedEtcdExecute                  func(context.Context, int) error
	embeddedKubeControllerManagerExecute func(context.Context, string) error
)

func Execute(ctx context.Context, cancel context.CancelFunc, args []string) error { //nolint:cyclop,gocognit // Will need to look into reducing the cyclomatic complexity.
	startTime := time.Now()
	globals := &flag.GlobalConfig{
		BackendKubeConfig:    kubeConfig(),
		PublicIP:             detectPublicIPv4(),
		EnableSmee:           true,
		EnableTootles:        true,
		EnableTinkServer:     true,
		EnableTinkController: true,
		EnableRufio:          true,
		EnableSecondStar:     true,
		EnableUI:             true,
		EnableCRDMigrations:  true,
		HTTPPort:             defaultHTTPPort,
		HTTPSPort:            defaultHTTPSPort,
		BindAddr:             detectPublicIPv4(),
		EmbeddedGlobalConfig: flag.EmbeddedGlobalConfig{
			EnableKubeAPIServer: (embeddedApiserverExecute != nil),
			EnableETCD:          (embeddedEtcdExecute != nil),
		},
		BackendKubeOptions: flag.BackendKubeOptions{
			QPS:   defaultQPS,   // Default QPS value. A negative value disables client-side ratelimiting.
			Burst: defaultBurst, // Default burst value.
		},
	}

	s := &flag.SmeeConfig{
		Config: smee.NewConfig(smee.Config{}, detectPublicIPv4()),
	}

	h := &flag.TootlesConfig{
		Config: tootles.NewConfig(tootles.Config{}),
	}
	ts := &flag.TinkServerConfig{
		Config:   server.NewConfig(server.WithAutoDiscoveryNamespace("default")),
		BindAddr: detectPublicIPv4(),
		BindPort: defaultTinkServerPort,
	}
	controllerOpts := []controller.Option{
		controller.WithMetricsAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), defaultTinkControllerMetricsPort))),
		controller.WithProbeAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), defaultTinkControllerProbePort))),
		controller.WithEnableLeaderElection(false),
	}
	tc := &flag.TinkControllerConfig{
		Config: controller.NewConfig(controllerOpts...),
	}

	rufioOpts := []rufio.Option{
		rufio.WithMetricsAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), defaultRufioMetricsPort))),
		rufio.WithProbeAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), defaultRufioProbePort))),
		rufio.WithBmcConnectTimeout(2 * time.Minute),
		rufio.WithPowerCheckInterval(30 * time.Minute),
		rufio.WithEnableLeaderElection(false),
	}
	rc := &flag.RufioConfig{
		Config: rufio.NewConfig(rufioOpts...),
	}

	ssc := &flag.SecondStarConfig{
		Config: &secondstar.Config{
			SSHPort:      defaultSecondStarPort,
			IPMITOOLPath: "/usr/sbin/ipmitool",
			IdleTimeout:  15 * time.Minute,
		},
	}

	uiOpts := []ui.Option{
		ui.WithURLPrefix("/"),
	}
	uic := &flag.UIConfig{
		Config: ui.NewConfig(uiOpts...),
	}

	// order here determines the help output.
	top := ff.NewFlagSet("smee - DHCP and iPXE service")
	if embeddedFlagSet != nil {
		top = ff.NewFlagSet("smee - DHCP and iPXE service").SetParent(embeddedFlagSet)
	}
	sfs := ff.NewFlagSet("smee - DHCP and iPXE service").SetParent(top)
	hfs := ff.NewFlagSet("tootles - Metadata service").SetParent(sfs)
	tfs := ff.NewFlagSet("tink server - Workflow service").SetParent(hfs)
	cfs := ff.NewFlagSet("tink controller - Workflow controller").SetParent(tfs)
	rfs := ff.NewFlagSet("rufio - BMC controller").SetParent(cfs)
	ssfs := ff.NewFlagSet("secondstar - SSH over serial service").SetParent(rfs)
	uifs := ff.NewFlagSet("ui - UI service").SetParent(ssfs)
	gfs := ff.NewFlagSet("globals").SetParent(uifs)
	flag.RegisterSmeeFlags(&flag.Set{FlagSet: sfs}, s)
	flag.RegisterTootlesFlags(&flag.Set{FlagSet: hfs}, h)
	flag.RegisterTinkServerFlags(&flag.Set{FlagSet: tfs}, ts)
	flag.RegisterTinkControllerFlags(&flag.Set{FlagSet: cfs}, tc)
	flag.RegisterRufioFlags(&flag.Set{FlagSet: rfs}, rc)
	flag.RegisterSecondStarFlags(&flag.Set{FlagSet: ssfs}, ssc)
	flag.RegisterUIFlags(&flag.Set{FlagSet: uifs}, uic)
	flag.RegisterGlobal(&flag.Set{FlagSet: gfs}, globals)
	if embeddedApiserverExecute != nil && embeddedFlagSet != nil {
		// This way the embedded flags only show up when the embedded services have been compiled in.
		flag.RegisterEmbeddedGlobals(&flag.Set{FlagSet: gfs}, globals)
	}

	cli := &ff.Command{
		Name:     "tinkerbell",
		Usage:    "tinkerbell [flags]",
		LongHelp: "Tinkerbell stack.",
		Flags:    gfs,
	}

	if err := cli.Parse(args, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		e := errors.New(ffhelp.Command(cli).String())
		if !errors.Is(err, ff.ErrHelp) {
			e = fmt.Errorf("%w\n%s", e, err)
		}

		return e
	}

	log := getLogger(globals.LogLevel)
	cliLog := log.WithName("cli")
	cliLog.Info("starting tinkerbell",
		"version", build.GitRevision(),
		"smeeEnabled", globals.EnableSmee,
		"tootlesEnabled", globals.EnableTootles,
		"tinkServerEnabled", globals.EnableTinkServer,
		"tinkControllerEnabled", globals.EnableTinkController,
		"rufioEnabled", globals.EnableRufio,
		"secondStarEnabled", globals.EnableSecondStar,
		"uiEnabled", globals.EnableUI,
		"publicIP", globals.PublicIP,
		"embeddedKubeAPIServer", globals.EmbeddedGlobalConfig.EnableKubeAPIServer,
		"embeddedEtcd", globals.EmbeddedGlobalConfig.EnableETCD,
		"globalBindAddress", globals.BindAddr,
	)

	// Smee
	s.Convert(&globals.TrustedProxies, globals.PublicIP, globals.BindAddr, globals.HTTPPort)
	if s.DHCPIPXEBinary.Port == 0 {
		s.DHCPIPXEBinary.Port = globals.HTTPPort
	}
	if s.DHCPIPXEScript.Port == 0 {
		s.DHCPIPXEScript.Port = globals.HTTPPort
	}
	s.Config.OTEL.Endpoint = globals.OTELEndpoint
	s.Config.OTEL.InsecureEndpoint = globals.OTELInsecure
	// Configure TLS if cert and key files are provided
	if globals.TLS.CertFile != "" && globals.TLS.KeyFile != "" {
		// Load the certificates with extensive logging
		// This key must be of type RSA. iPXE does not support ECDSA keys.
		cert, err := tls.LoadX509KeyPair(globals.TLS.CertFile, globals.TLS.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS certificates for Smee HTTP: %w", err)
		}
		// iPXE only supports using RSA keys for TLS. https://github.com/ipxe/ipxe/issues/1179
		if _, ok := cert.PrivateKey.(*rsa.PrivateKey); !ok {
			log.Info("WARNING: iPXE only supports RSA certificates. HTTPS for Smee's iPXE binaries and scripts might not work", "certType", fmt.Sprintf("%T", cert.PrivateKey))
		}
		s.Config.TLS.Certs = []tls.Certificate{cert}
	}

	// Tink Server
	ts.Convert(globals.BindAddr)
	// Configure TLS if cert and key files are provided
	if globals.TLS.CertFile != "" && globals.TLS.KeyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(globals.TLS.CertFile, globals.TLS.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS credentials for Tink Server gRPC: %w", err)
		}
		ts.Config.TLS.Cert = creds
		// When using TLS with the Tink Server, the Agent needs to know that TLS is enabled.
		// This is done in the Smee config.
		// First check if the extra kernel parameter already exists.
		updated := false
		for i, arg := range s.Config.IPXE.HTTPScriptServer.ExtraKernelArgs {
			if strings.HasPrefix(arg, "tinkerbell_tls") {
				s.Config.IPXE.HTTPScriptServer.ExtraKernelArgs[i] = "tinkerbell_tls=true"
				updated = true
				break
			}
		}
		if !updated {
			s.Config.IPXE.HTTPScriptServer.ExtraKernelArgs = append(s.Config.IPXE.HTTPScriptServer.ExtraKernelArgs, "tinkerbell_tls=true")
		}
	}

	// Tink Controller
	tc.Config.LeaderElectionNamespace = leaderElectionNamespace(inCluster(), tc.Config.EnableLeaderElection, tc.Config.LeaderElectionNamespace)
	if globals.BindAddr.IsValid() {
		tc.Config.MetricsAddr = netip.AddrPortFrom(globals.BindAddr, tc.Config.MetricsAddr.Port())
		tc.Config.ProbeAddr = netip.AddrPortFrom(globals.BindAddr, tc.Config.ProbeAddr.Port())
	}

	// Rufio Controller
	rc.Config.LeaderElectionNamespace = leaderElectionNamespace(inCluster(), rc.Config.EnableLeaderElection, rc.Config.LeaderElectionNamespace)
	if globals.BindAddr.IsValid() {
		rc.Config.MetricsAddr = netip.AddrPortFrom(globals.BindAddr, rc.Config.MetricsAddr.Port())
		rc.Config.ProbeAddr = netip.AddrPortFrom(globals.BindAddr, rc.Config.ProbeAddr.Port())
	}

	// Second star
	if err := ssc.Convert(); err != nil {
		return fmt.Errorf("failed to convert secondstar config: %w", err)
	}
	if globals.BindAddr.IsValid() {
		ssc.Config.BindAddr = globals.BindAddr
	}

	// Initialize OTel before starting goroutines so the provider outlives
	// all goroutines (Smee non-HTTP, consolidated HTTP server, etc.).
	// otel.Init is a no-op when globals.OTELEndpoint is empty.
	otelCtx, otelShutdown, err := otel.Init(ctx, otel.Config{
		Servicename: "tinkerbell",
		Endpoint:    globals.OTELEndpoint,
		Insecure:    globals.OTELInsecure,
		Logger:      log.WithName("otel"),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
	}
	ctx = otelCtx
	defer otelShutdown()

	g, ctx := errgroup.WithContext(ctx)
	// Etcd server
	g.Go(func() error {
		if !globals.EmbeddedGlobalConfig.EnableETCD {
			cliLog.Info("embedded etcd is disabled")
			return nil
		}
		if embeddedEtcdExecute != nil {
			if err := retry.Do(func() error {
				if err := embeddedEtcdExecute(ctx, globals.LogLevel); err != nil {
					return fmt.Errorf("etcd server error: %w", err)
				}
				return nil
			}, retry.Attempts(10), retry.Delay(2*time.Second), retry.Context(ctx)); err != nil {
				return err
			}
		}
		return nil
	})

	// API Server
	g.Go(func() error {
		if !globals.EmbeddedGlobalConfig.EnableKubeAPIServer {
			cliLog.Info("embedded kube-apiserver is disabled")
			return nil
		}
		if embeddedApiserverExecute != nil {
			if err := retry.Do(func() error {
				if err := embeddedApiserverExecute(ctx, log.WithName("kube-apiserver")); err != nil {
					return fmt.Errorf("API server error: %w", err)
				}
				return nil
			}, retry.Attempts(10), retry.Delay(2*time.Second), retry.Context(ctx)); err != nil {
				return err
			}
		}
		return nil
	})

	if numEnabled(globals) == 0 {
		globals.Backend = "pass"
	}
	switch globals.Backend {
	case "kube":
		if globals.EnableCRDMigrations {
			backendNoIndexes, err := newKubeBackend(ctx, globals.BackendKubeConfig, "", globals.BackendKubeNamespace, nil, WithQPS(globals.BackendKubeOptions.QPS), WithBurst(globals.BackendKubeOptions.Burst))
			if err != nil {
				return fmt.Errorf("failed to create kube backend with no indexes: %w", err)
			}
			// Wait for the API server to be healthy and ready.
			if err := backendNoIndexes.WaitForAPIServer(ctx, cliLog, 20*time.Second, 5*time.Second, nil); err != nil {
				return fmt.Errorf("failed to wait for API server health: %w", err)
			}

			if err := crd.NewTinkerbell(crd.WithRestConfig(backendNoIndexes.ClientConfig)).MigrateAndReady(ctx); err != nil {
				cancel()
				gerr := g.Wait()
				return fmt.Errorf("CRD migrations failed: %w", errors.Join(err, gerr))
			}
			cliLog.Info("CRD migrations completed")
		}

		b, err := newKubeBackend(ctx, globals.BackendKubeConfig, "", globals.BackendKubeNamespace, enabledIndexes(globals.EnableSmee, globals.EnableTootles, globals.EnableTinkServer, globals.EnableSecondStar), WithQPS(globals.BackendKubeOptions.QPS), WithBurst(globals.BackendKubeOptions.Burst))
		if err != nil {
			return fmt.Errorf("failed to create kube backend: %w", err)
		}
		s.Config.Backend = b
		h.Config.BackendEc2 = b
		h.Config.BackendHack = b
		ts.Config.Backend = b
		ts.Config.Auto.Enrollment.Backend = b
		ts.Config.Auto.Discovery.Backend = b
		tc.Config.Client = b.ClientConfig
		tc.Config.DynamicClient = b
		rc.Config.Client = b.ClientConfig
		ssc.Config.Backend = b
		if uic.Config.EnableAutoLogin {
			uic.Config.AutoLoginRestConfig = b.ClientConfig
			uic.Config.AutoLoginNamespace = globals.BackendKubeNamespace
		}
	case "file":
		b, err := newFileBackend(ctx, log, globals.BackendFilePath)
		if err != nil {
			return fmt.Errorf("failed to create file backend: %w", err)
		}
		s.Config.Backend = b
	case "none":
		b := newNoopBackend()
		s.Config.Backend = b
		h.Config.BackendEc2 = b
		h.Config.BackendHack = b
	case "pass":
	default:
		return fmt.Errorf("unknown backend %q", globals.Backend)
	}

	// Kube Controller Manager
	g.Go(func() error {
		if !globals.EmbeddedGlobalConfig.EnableKubeAPIServer {
			cliLog.Info("embedded kube-controller-manager is disabled")
			return nil
		}
		if err := embeddedKubeControllerManagerExecute(ctx, globals.BackendKubeConfig); err != nil {
			return fmt.Errorf("kube-controller-manager error: %w", err)
		}
		return nil
	})

	// Smee (non-HTTP services: DHCP, TFTP, syslog)
	g.Go(func() error {
		if !globals.EnableSmee {
			cliLog.Info("smee service is disabled")
			return nil
		}
		ll := ternary((s.LogLevel != 0), s.LogLevel, globals.LogLevel)
		smeeLog := getLogger(ll).WithName("smee")

		// Register Smee-specific Prometheus metrics (DHCP counters, etc.).
		// OTel is already initialized globally above, so we only init metrics
		// here to avoid overwriting the global tracer provider.
		s.Config.InitMetrics()

		if err := s.Config.Start(ctx, smeeLog); err != nil {
			return fmt.Errorf("failed to start smee service: %w", err)
		}
		return nil
	})

	// HTTP server
	g.Go(func() error {
		httpLog := getLogger(globals.LogLevel).WithName("http")
		routeList := &httpserver.Routes{}

		// Smee HTTP handlers
		if globals.EnableSmee {
			ll := ternary((s.LogLevel != 0), s.LogLevel, globals.LogLevel)
			smeeLog := getLogger(ll).WithName("smee")

			if h := s.Config.BinaryHandler(smeeLog); h != nil {
				routeList.Register(routeIPXEBinary,
					middleware.WithLogLevel(middleware.LogLevelAlways, h),
					"smee iPXE binary handler",
				)
			}
			if h := s.Config.ScriptHandler(smeeLog); h != nil {
				routeList.Register(routeIPXEScript,
					middleware.WithLogLevel(middleware.LogLevelAlways, h),
					"smee iPXE script handler",
				)
			}
			isoH, err := s.Config.ISOHandler(smeeLog)
			if err != nil {
				return fmt.Errorf("failed to create smee iso handler: %w", err)
			}
			if isoH != nil {
				routeList.Register(routeISO,
					middleware.WithLogLevel(middleware.LogLevelNever, isoH),
					"smee ISO handler",
					httpserver.WithHTTPSEnabled(true),
				)
			}
		}

		// Tootles HTTP handlers
		if globals.EnableTootles {
			ec2H := middleware.WithLogLevel(middleware.LogLevelAlways, h.Config.EC2MetadataHandler())
			routeList.Register(routeEC2Metadata,
				ec2H,
				"EC2 metadata handler",
				httpserver.WithHTTPSEnabled(true),
				httpserver.WithRewriteHTTPToHTTPS(true),
			)
			if h.Config.InstanceEndpoint {
				routeList.Register(routeTootles,
					ec2H,
					"EC2 instance endpoint handler",
					httpserver.WithHTTPSEnabled(true),
					httpserver.WithRewriteHTTPToHTTPS(true),
				)
			}
			routeList.Register(routeHackMetadata,
				middleware.WithLogLevel(middleware.LogLevelAlways, h.Config.HackMetadataHandler()),
				"Hack metadata handler",
				httpserver.WithHTTPSEnabled(true),
				httpserver.WithRewriteHTTPToHTTPS(true),
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
					httpserver.WithHTTPSEnabled(true),
					httpserver.WithRewriteHTTPToHTTPS(true),
				)
			}
		}

		// Shared metrics and healthcheck.
		// Use the default Prometheus registry so that Smee's promauto-registered
		// metrics (DHCP counters, discover/job histograms, etc.) and the HTTP
		// middleware metrics all appear on the same /metrics endpoint.
		// The default registry already includes GoCollector and ProcessCollector.
		routeList.Register(routeMetrics, middleware.WithLogLevel(middleware.LogLevelNever, promhttp.Handler()), "Prometheus metrics handler")
		routeList.Register(routeHealthcheck, middleware.WithLogLevel(middleware.LogLevelNever, handler.HealthCheck(httpLog, startTime)), "Healthcheck handler")

		httpMux, httpsMux := routeList.Muxes(httpLog, globals.HTTPSPort, len(s.Config.TLS.Certs) > 0)

		httpHandler, httpsHandler, err := addMiddleware(httpLog, globals.TrustedProxies, httpMux, httpsMux)
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
			"schemes", func() []string {
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
	})

	// Tink Server
	g.Go(func() error {
		if !globals.EnableTinkServer {
			cliLog.Info("tink server service is disabled")
			return nil
		}
		ll := ternary((ts.LogLevel != 0), ts.LogLevel, globals.LogLevel)
		if err := ts.Config.Start(ctx, getLogger(ll).WithName("tink-server")); err != nil {
			return fmt.Errorf("failed to start tink server service: %w", err)
		}
		return nil
	})

	// Tink Controller
	g.Go(func() error {
		if !globals.EnableTinkController {
			cliLog.Info("tink controller service is disabled")
			return nil
		}
		ll := ternary((tc.LogLevel != 0), tc.LogLevel, globals.LogLevel)
		if err := tc.Config.Start(ctx, getLogger(ll).WithName("tink-controller")); err != nil {
			return fmt.Errorf("failed to start tink controller service: %w", err)
		}
		return nil
	})

	// Rufio Controller
	g.Go(func() error {
		if !globals.EnableRufio {
			cliLog.Info("rufio service is disabled")
			return nil
		}
		ll := ternary((rc.LogLevel != 0), rc.LogLevel, globals.LogLevel)
		if err := rc.Config.Start(ctx, getLogger(ll).WithName("rufio")); err != nil {
			return fmt.Errorf("failed to start rufio service: %w", err)
		}
		return nil
	})

	// SecondStar
	g.Go(func() error {
		if !globals.EnableSecondStar {
			cliLog.Info("secondstar service is disabled")
			return nil
		}
		ll := ternary((ssc.LogLevel != 0), ssc.LogLevel, globals.LogLevel)
		if err := ssc.Config.Start(ctx, getLogger(ll).WithName("secondstar")); err != nil {
			return fmt.Errorf("failed to start secondstar service: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func ternary[T any](condition bool, valueIfTrue, valueIfFalse T) T {
	if condition {
		return valueIfTrue
	}
	return valueIfFalse
}

func numEnabled(globals *flag.GlobalConfig) int {
	n := 0
	if globals.EnableSmee {
		n++
	}
	if globals.EnableTootles {
		n++
	}
	if globals.EnableTinkServer {
		n++
	}
	if globals.EnableTinkController {
		n++
	}
	if globals.EnableRufio {
		n++
	}
	if globals.EnableSecondStar {
		n++
	}
	if globals.EnableUI {
		n++
	}
	return n
}

func enabledIndexes(smeeEnabled, tootlesEnabled, tinkServerEnabled, secondStarEnabled bool) map[kube.IndexType]kube.Index {
	idxs := make(map[kube.IndexType]kube.Index, 0)

	if smeeEnabled {
		idxs = flag.KubeIndexesSmee
	}
	if tootlesEnabled {
		for k, v := range flag.KubeIndexesTootles {
			idxs[k] = v
		}
	}
	if tinkServerEnabled {
		for k, v := range flag.KubeIndexesTinkServer {
			idxs[k] = v
		}
	}
	if secondStarEnabled {
		for k, v := range flag.KubeIndexesSecondStar {
			idxs[k] = v
		}
	}

	return idxs
}

// normalizeURLPrefix ensures a URL prefix is valid for use with http.ServeMux.
// It trims whitespace, ensures a leading "/", cleans the path (collapsing repeated
// slashes, resolving ".." etc.), and ensures a trailing "/" so the mux matches all
// sub-paths.
func normalizeURLPrefix(prefix string) string {
	pattern := strings.TrimSpace(prefix)
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	pattern = path.Clean(pattern)
	if !strings.HasSuffix(pattern, "/") {
		pattern += "/"
	}
	return pattern
}

// inCluster checks if we are running in cluster.
func inCluster() bool {
	if _, err := rest.InClusterConfig(); err == nil {
		return true
	}
	return false
}

// addMiddleware is a helper to apply a middleware functions to http.Handlers.
func addMiddleware(log logr.Logger, trustedProxies []netip.Prefix, httpHandler, httpsHandler http.Handler) (http.Handler, http.Handler, error) {
	// Apply middleware chain. Each wrap adds an outer layer, so the last
	// applied middleware runs first on the request path and last on the
	// response path:
	//   Request  → SourceIP → XFF → RequestMetrics → Recovery → Logging → OTel → mux
	//   Response ← SourceIP ← XFF ← RequestMetrics ← Recovery ← Logging ← OTel ← mux
	httpHandler = middleware.OTel("tinkerbell-http")(httpHandler)
	httpHandler = middleware.Logging(log)(httpHandler)
	httpHandler = middleware.Recovery(log)(httpHandler)
	httpHandler = middleware.RequestMetrics()(httpHandler)
	if len(trustedProxies) > 0 {
		proxies := make([]string, 0, len(trustedProxies))
		for _, p := range trustedProxies {
			proxies = append(proxies, p.String())
		}
		h, err := middleware.XFF(proxies)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create XFF middleware: %w", err)
		}
		httpHandler = h(httpHandler)
	}
	httpHandler = middleware.SourceIP()(httpHandler)

	httpsHandler = middleware.OTel("tinkerbell-https")(httpsHandler)
	httpsHandler = middleware.Logging(log)(httpsHandler)
	httpsHandler = middleware.Recovery(log)(httpsHandler)
	httpsHandler = middleware.RequestMetrics()(httpsHandler)
	if len(trustedProxies) > 0 {
		proxies := make([]string, 0, len(trustedProxies))
		for _, p := range trustedProxies {
			proxies = append(proxies, p.String())
		}
		h, err := middleware.XFF(proxies)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create XFF middleware: %w", err)
		}
		httpsHandler = h(httpsHandler)
	}
	httpsHandler = middleware.SourceIP()(httpsHandler)

	return httpHandler, httpsHandler, nil
}
