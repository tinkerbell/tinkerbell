package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/crd"
	"github.com/tinkerbell/tinkerbell/pkg/build"
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
	"k8s.io/klog/v2"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

var (
	embeddedFlagSet                      *ff.FlagSet
	embeddedApiserverExecute             func(context.Context, logr.Logger) error
	embeddedEtcdExecute                  func(context.Context, int) error
	embeddedKubeControllerManagerExecute func(context.Context, string) error
)

func Execute(ctx context.Context, cancel context.CancelFunc, args []string) error {
	return executeWithOutput(ctx, cancel, args, os.Stdout)
}

// executeWithOutput allows command output to be captured in tests.
func executeWithOutput(ctx context.Context, cancel context.CancelFunc, args []string, stdout io.Writer) error { //nolint:cyclop // Will need to look into reducing the cyclomatic complexity.
	startTime := time.Now() // used in the HTTP healthcheck handler to report uptime.
	publicIP := detectPublicIPv4()
	publicIPv6 := detectPublicIPv6()
	globals := &flag.GlobalConfig{
		BackendKubeConfig: kubeConfig(),
		PublicIP:          publicIP,
		PublicIPv6:        publicIPv6,

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
		Config: smee.NewConfig(smee.Config{}),
	}

	h := &flag.TootlesConfig{
		Config: tootles.NewConfig(tootles.Config{}),
	}
	ts := &flag.TinkServerConfig{
		Config:   server.NewConfig(server.WithAutoDiscoveryNamespace("default")),
		BindPort: defaultTinkServerPort,
	}
	controllerOpts := []controller.Option{
		controller.WithEnableLeaderElection(false),
	}
	tc := &flag.TinkControllerConfig{
		Config: controller.NewConfig(controllerOpts...),
	}

	rufioOpts := []rufio.Option{
		rufio.WithBmcConnectTimeout(2 * time.Minute),
		rufio.WithPowerCheckInterval(30 * time.Minute),
		rufio.WithInventoryRefreshInterval(24 * time.Hour),
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
	var printVersion bool
	gfs.BoolVar(&printVersion, 0, "version", "Print the version and exit")
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
	if printVersion {
		if _, err := fmt.Fprintln(stdout, "tinkerbell", build.GitRevision()); err != nil {
			return fmt.Errorf("print version: %w", err)
		}
		return nil
	}
	if err := validatePublicAddressFamilies(globals.PublicIP, globals.PublicIPv6); err != nil {
		return err
	}
	if !globals.BindAddr.IsValid() {
		globals.BindAddr = defaultBindAddr(publicIP, publicIPv6)
	}

	log := getLogger(globals.LogLevel)

	// klog (client-go and other Kubernetes libraries) and controller-runtime
	// both log through process-global loggers for code paths that aren't handed
	// an explicit, per-instance logger. Set each once here, named after the
	// library that emits through it, so their output is attributed to the
	// correct logger instead of to whichever component called SetLogger last.
	// Per-component logs are unaffected: each manager still logs under its own
	// name via controllerruntime.Options.Logger.
	klog.SetLogger(log.WithName("klog"))
	controllerruntime.SetLogger(log.WithName("controller-runtime"))

	// Kubernetes API servers return HTTP "Warning" headers (e.g. API
	// deprecations such as v1 Endpoints) that client-go surfaces through a
	// process-global default warning handler. By default that handler logs via
	// klog, so warnings inherit whichever component's global klog logger is
	// active (misattributing them). Own the handler here so warnings are
	// attributed to a dedicated logger, independent of klog. The embedded
	// kube-apiserver/kube-controller-manager install their own NoWarnings
	// handler only from their cobra pre-run, which this binary bypasses, so this
	// handler is not overridden.
	rest.SetDefaultWarningHandler(k8sAPIWarningLogger{log: log.WithName("kube-api-warning")})

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
		"publicIPv6", globals.PublicIPv6,
		"embeddedKubeAPIServer", globals.EmbeddedGlobalConfig.EnableKubeAPIServer,
		"embeddedEtcd", globals.EmbeddedGlobalConfig.EnableETCD,
		"globalBindAddress", globals.BindAddr,
	)

	// Smee
	s.Convert(&globals.TrustedProxies, globals.PublicIP, globals.PublicIPv6, globals.BindAddr, globals.HTTPPort)
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
		// Set UseTLS so the iPXE script template emits tinkerbell_tls=true in kernel args.
		s.Config.TinkServer.UseTLS = true
	}

	// Tink Controller
	tc.Config.LeaderElectionNamespace = leaderElectionNamespace(inCluster(), tc.Config.EnableLeaderElection, tc.Config.LeaderElectionNamespace)

	// Rufio Controller
	rc.Config.LeaderElectionNamespace = leaderElectionNamespace(inCluster(), rc.Config.EnableLeaderElection, rc.Config.LeaderElectionNamespace)

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
				if err := embeddedApiserverExecute(ctx, log); err != nil {
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

			tb, err := crd.NewTinkerbell(crd.WithLogger(cliLog), crd.WithRestConfig(backendNoIndexes.ClientConfig))
			if err != nil {
				return fmt.Errorf("failed to create CRD migrator: %w", err)
			}
			if err := tb.MigrateAndReady(ctx); err != nil {
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
		h.Config.SetBackendFromFilterer(b)
		ts.Config.SetBackends(b)
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
		h.Config.SetBackendFromFilterer(b)
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

	// Initialize Smee metrics before starting goroutines so that metric
	// globals (JobsTotal, JobsInProgress, etc.) are ready before the HTTP
	// server can receive iPXE script requests that reference them.
	if globals.EnableSmee {
		s.Config.InitMetrics()
	}

	// Smee (non-HTTP services: DHCP, TFTP, syslog)
	g.Go(func() error {
		if !globals.EnableSmee {
			cliLog.Info("smee service is disabled")
			return nil
		}
		ll := ternary((s.LogLevel != 0), s.LogLevel, globals.LogLevel)
		smeeLog := getLogger(ll).WithName("smee")

		if err := s.Config.Start(ctx, smeeLog); err != nil {
			return fmt.Errorf("failed to start smee service: %w", err)
		}
		return nil
	})

	// HTTP server
	g.Go(func() error {
		return startHTTPServer(ctx, globals, s, h, uic, startTime)
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
