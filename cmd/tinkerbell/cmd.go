package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/crd"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/pkg/build"
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
	defaultSmeeHTTPPort              = 7171
	defaultSmeeHTTPSPort             = 7272
	defaultTinkControllerMetricsPort = 8080
	defaultTinkControllerProbePort   = 8081
	defaultTinkServerPort            = 42113
	defaultTootlesPort               = 50061
	defaultUIPort                    = 8085
)

var (
	embeddedFlagSet                      *ff.FlagSet
	embeddedApiserverExecute             func(context.Context, logr.Logger) error
	embeddedEtcdExecute                  func(context.Context, int) error
	embeddedKubeControllerManagerExecute func(context.Context, string) error
)

func Execute(ctx context.Context, cancel context.CancelFunc, args []string) error { //nolint:cyclop // Will need to look into reducing the cyclomatic complexity.
	globals := &flag.GlobalConfig{
		BackendKubeConfig:    kubeConfig(),
		PublicIP:             detectPublicIPv4(),
		EnableSmee:           true,
		EnableTootles:        true,
		EnableTinkServer:     true,
		EnableTinkController: true,
		EnableRufio:          true,
		EnableSecondStar:     true,
		EnableUI:             false,
		EnableCRDMigrations:  true,
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
		DHCPIPXEBinary: flag.URLBuilder{
			Port: defaultSmeeHTTPPort,
		},
		DHCPIPXEScript: flag.URLBuilder{
			Port: defaultSmeeHTTPPort,
		},
	}

	h := &flag.TootlesConfig{
		Config:   tootles.NewConfig(tootles.Config{}, fmt.Sprintf("%s:%d", detectPublicIPv4().String(), defaultTootlesPort)),
		BindAddr: detectPublicIPv4(),
		BindPort: defaultTootlesPort,
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
		ui.WithURLPrefix("/ui"),
		ui.WithBindPort(defaultUIPort),
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

	log := getLogger(false, globals.LogLevel)
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
	s.Convert(&globals.TrustedProxies, globals.PublicIP, globals.BindAddr)
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

	// Tootles
	h.Convert(&globals.TrustedProxies, globals.BindAddr)

	// Tink Server
	ts.Convert(globals.BindAddr)
	// Configure TLS if cert and key files are provided
	if globals.TLS.CertFile != "" && globals.TLS.KeyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(globals.TLS.CertFile, globals.TLS.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS credentials for Tink Server gRPC: %w", err)
		}
		ts.Config.TLS.Cert = creds
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

	// UI
	uic.Convert(globals.BindAddr, globals.TLS.CertFile, globals.TLS.KeyFile)

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

	// Smee
	g.Go(func() error {
		if !globals.EnableSmee {
			cliLog.Info("smee service is disabled")
			return nil
		}
		ll := ternary((s.LogLevel != 0), s.LogLevel, globals.LogLevel)
		if err := s.Config.Start(ctx, getLogger(s.NoLog, ll).WithName("smee")); err != nil {
			return fmt.Errorf("failed to start smee service: %w", err)
		}
		return nil
	})

	// Tootles
	g.Go(func() error {
		if !globals.EnableTootles {
			cliLog.Info("tootles service is disabled")
			return nil
		}
		ll := ternary((h.LogLevel != 0), h.LogLevel, globals.LogLevel)
		if err := h.Config.Start(ctx, getLogger(h.NoLog, ll).WithName("tootles")); err != nil {
			return fmt.Errorf("failed to start tootles service: %w", err)
		}
		return nil
	})

	// Tink Server
	g.Go(func() error {
		if !globals.EnableTinkServer {
			cliLog.Info("tink server service is disabled")
			return nil
		}
		ll := ternary((ts.LogLevel != 0), ts.LogLevel, globals.LogLevel)
		if err := ts.Config.Start(ctx, getLogger(ts.NoLog, ll).WithName("tink-server")); err != nil {
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
		if err := tc.Config.Start(ctx, getLogger(tc.NoLog, ll).WithName("tink-controller")); err != nil {
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
		if err := rc.Config.Start(ctx, getLogger(rc.NoLog, ll).WithName("rufio")); err != nil {
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
		if err := ssc.Config.Start(ctx, getLogger(ssc.NoLog, ll).WithName("secondstar")); err != nil {
			return fmt.Errorf("failed to start secondstar service: %w", err)
		}
		return nil
	})

	// UI
	g.Go(func() error {
		if !globals.EnableUI {
			cliLog.Info("ui service is disabled")
			return nil
		}
		ll := ternary((uic.LogLevel != 0), uic.LogLevel, globals.LogLevel)
		if err := uic.Config.Start(ctx, getLogger(uic.NoLog, ll).WithName("ui")); err != nil {
			return fmt.Errorf("failed to start ui service: %w", err)
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

// getLogger returns a logger based on the configuration.
// If noLog is true, returns a logger that discards all output.
func getLogger(noLog bool, level int) logr.Logger {
	if noLog {
		return logr.Discard()
	}
	return defaultLogger(level)
}

// defaultLogger uses the slog logr implementation.
func defaultLogger(level int) logr.Logger {
	// source file and function can be long. This makes the logs less readable.
	// for improved readability, truncate source file to last 3 parts and remove the function entirely.
	customAttr := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.SourceKey {
			ss, ok := a.Value.Any().(*slog.Source)
			if !ok || ss == nil {
				return a
			}

			p := strings.Split(ss.File, "/")
			// log the file path from tinkerbell/tinkerbell to the end.
			var idx int

			for i, v := range p {
				if v == "tinkerbell" {
					if i+2 < len(p) {
						idx = i + 2
						break
					}
				}
				// This trims the source file for 3rd party packages to include
				// just enough information to identify the package. Without this,
				// the source file can be long and make the log line more cluttered
				// and hard to read.
				if v == "mod" {
					if i+1 < len(p) {
						idx = i + 1
						break
					}
				}
			}
			ss.File = filepath.Join(p[idx:]...)
			ss.File = fmt.Sprintf("%s:%d", ss.File, ss.Line)
			a.Value = slog.StringValue(ss.File)
			a.Key = "caller"

			return a
		}

		// This changes the slog.Level string representation to an integer.
		// This makes it so that the V-levels passed in to the CLI show up as is in the logs.
		if a.Key == slog.LevelKey {
			b, ok := a.Value.Any().(slog.Level)
			if !ok {
				return a
			}
			a.Value = slog.StringValue(strconv.Itoa(int(b)))
			return a
		}

		return a
	}
	opts := &slog.HandlerOptions{
		AddSource:   true,
		Level:       slog.Level(-level),
		ReplaceAttr: customAttr,
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, opts))

	return logr.FromSlogHandler(log.Handler())
}

// inCluster checks if we are running in cluster.
func inCluster() bool {
	if _, err := rest.InClusterConfig(); err == nil {
		return true
	}
	return false
}
