package main

import (
	"context"
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
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/migrate"
	"github.com/tinkerbell/tinkerbell/crd"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/pkg/build"
	"github.com/tinkerbell/tinkerbell/rufio"
	"github.com/tinkerbell/tinkerbell/secondstar"
	"github.com/tinkerbell/tinkerbell/smee"
	"github.com/tinkerbell/tinkerbell/tink/controller"
	"github.com/tinkerbell/tinkerbell/tink/server"
	"github.com/tinkerbell/tinkerbell/tootles"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/rest"
)

var (
	embeddedFlagSet                      *ff.FlagSet
	embeddedApiserverExecute             func(context.Context, logr.Logger) error
	embeddedEtcdExecute                  func(context.Context, int) error
	embeddedKubeControllerManagerExecute func(context.Context, string) error
)

type Config struct {
	GlobalConfig         *flag.GlobalConfig
	SmeeConfig           *flag.SmeeConfig
	TootlesConfig        *flag.TootlesConfig
	TinkServerConfig     *flag.TinkServerConfig
	TinkControllerConfig *flag.TinkControllerConfig
	RufioConfig          *flag.RufioConfig
	SecondStarConfig     *flag.SecondStarConfig
}

func Execute(ctx context.Context, cancel context.CancelFunc, args []string) error {
	c := flags()
	gfs := rootFlagSet(c)
	cli := &ff.Command{
		Name:     "tinkerbell",
		Usage:    "tinkerbell [flags]",
		LongHelp: "Tinkerbell stack.",
		Flags:    gfs,
		Subcommands: []*ff.Command{
			migrate.NewCommand(),
		},
		Exec: func(ctx context.Context, _ []string) error {
			return rootCommand(ctx, cancel, c)
		},
	}

	if err := cli.ParseAndRun(ctx, args, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		e := errors.New(ffhelp.Command(cli).String())
		if !errors.Is(err, ff.ErrHelp) {
			e = fmt.Errorf("%w\n%s", e, err)
		}

		return e
	}

	return nil
}

func rootFlagSet(c Config) *ff.FlagSet {
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
	gfs := ff.NewFlagSet("globals").SetParent(ssfs)
	flag.RegisterSmeeFlags(&flag.Set{FlagSet: sfs}, c.SmeeConfig)
	flag.RegisterTootlesFlags(&flag.Set{FlagSet: hfs}, c.TootlesConfig)
	flag.RegisterTinkServerFlags(&flag.Set{FlagSet: tfs}, c.TinkServerConfig)
	flag.RegisterTinkControllerFlags(&flag.Set{FlagSet: cfs}, c.TinkControllerConfig)
	flag.RegisterRufioFlags(&flag.Set{FlagSet: rfs}, c.RufioConfig)
	flag.RegisterSecondStarFlags(&flag.Set{FlagSet: ssfs}, c.SecondStarConfig)
	flag.RegisterGlobal(&flag.Set{FlagSet: gfs}, c.GlobalConfig)
	if embeddedApiserverExecute != nil && embeddedFlagSet != nil {
		// This way the embedded flags only show up when the embedded services have been compiled in.
		flag.RegisterEmbeddedGlobals(&flag.Set{FlagSet: gfs}, c.GlobalConfig)
	}
	return gfs
}

func flags() Config {
	globals := &flag.GlobalConfig{
		BackendKubeConfig:    kubeConfig(),
		PublicIP:             detectPublicIPv4(),
		EnableSmee:           true,
		EnableTootles:        true,
		EnableTinkServer:     true,
		EnableTinkController: true,
		EnableRufio:          true,
		EnableSecondStar:     true,
		EnableCRDMigrations:  true,
		EmbeddedGlobalConfig: flag.EmbeddedGlobalConfig{
			EnableKubeAPIServer: (embeddedApiserverExecute != nil),
			EnableETCD:          (embeddedEtcdExecute != nil),
		},
	}

	s := &flag.SmeeConfig{
		Config: smee.NewConfig(smee.Config{}, detectPublicIPv4()),
		DHCPIPXEBinary: flag.URLBuilder{
			Port: smee.DefaultHTTPPort,
		},
		DHCPIPXEScript: flag.URLBuilder{
			Port: smee.DefaultHTTPPort,
		},
	}

	h := &flag.TootlesConfig{
		Config:   tootles.NewConfig(tootles.Config{}, fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 50061)),
		BindAddr: detectPublicIPv4(),
		BindPort: 50061,
	}
	ts := &flag.TinkServerConfig{
		Config:   server.NewConfig(server.WithAutoDiscoveryNamespace("default")),
		BindAddr: detectPublicIPv4(),
		BindPort: 42113,
	}
	controllerOpts := []controller.Option{
		controller.WithMetricsAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8080))),
		controller.WithProbeAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8081))),
		controller.WithEnableLeaderElection(false),
	}
	tc := &flag.TinkControllerConfig{
		Config: controller.NewConfig(controllerOpts...),
	}

	rufioOpts := []rufio.Option{
		rufio.WithMetricsAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8082))),
		rufio.WithProbeAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8083))),
		rufio.WithBmcConnectTimeout(2 * time.Minute),
		rufio.WithPowerCheckInterval(30 * time.Minute),
		rufio.WithEnableLeaderElection(false),
	}
	rc := &flag.RufioConfig{
		Config: rufio.NewConfig(rufioOpts...),
	}

	ssc := &flag.SecondStarConfig{
		Config: &secondstar.Config{
			SSHPort:      2222,
			IPMITOOLPath: "/usr/sbin/ipmitool",
			IdleTimeout:  15 * time.Minute,
		},
	}

	// Smee
	s.Convert(&globals.TrustedProxies, globals.PublicIP)
	s.Config.OTEL.Endpoint = globals.OTELEndpoint
	s.Config.OTEL.InsecureEndpoint = globals.OTELInsecure

	// Tootles
	h.Convert(&globals.TrustedProxies)

	// Tink Server
	ts.Convert()

	// Tink Controller
	tc.Config.LeaderElectionNamespace = leaderElectionNamespace(inCluster(), tc.Config.EnableLeaderElection, tc.Config.LeaderElectionNamespace)

	// Rufio Controller
	rc.Config.LeaderElectionNamespace = leaderElectionNamespace(inCluster(), rc.Config.EnableLeaderElection, rc.Config.LeaderElectionNamespace)

	// Second star
	if err := ssc.Convert(); err != nil {
		// return fmt.Errorf("failed to convert secondstar config: %w", err)
		return Config{}
	}

	return Config{
		GlobalConfig:         globals,
		SmeeConfig:           s,
		TootlesConfig:        h,
		TinkServerConfig:     ts,
		TinkControllerConfig: tc,
		RufioConfig:          rc,
		SecondStarConfig:     ssc,
	}
}

func rootCommand(ctx context.Context, cancel context.CancelFunc, c Config) error {
	log := defaultLogger(c.GlobalConfig.LogLevel)
	log.Info("starting tinkerbell",
		"version", build.GitRevision(),
		"smeeEnabled", c.GlobalConfig.EnableSmee,
		"tootlesEnabled", c.GlobalConfig.EnableTootles,
		"tinkServerEnabled", c.GlobalConfig.EnableTinkServer,
		"tinkControllerEnabled", c.GlobalConfig.EnableTinkController,
		"rufioEnabled", c.GlobalConfig.EnableRufio,
		"secondStarEnabled", c.GlobalConfig.EnableSecondStar,
		"publicIP", c.GlobalConfig.PublicIP,
		"embeddedKubeAPIServer", c.GlobalConfig.EmbeddedGlobalConfig.EnableKubeAPIServer,
		"embeddedEtcd", c.GlobalConfig.EmbeddedGlobalConfig.EnableETCD,
	)

	g, ctx := errgroup.WithContext(ctx)
	// Etcd server
	g.Go(func() error {
		if !c.GlobalConfig.EmbeddedGlobalConfig.EnableETCD {
			log.Info("embedded etcd is disabled")
			return nil
		}
		if embeddedEtcdExecute != nil {
			if err := retry.Do(func() error {
				if err := embeddedEtcdExecute(ctx, c.GlobalConfig.LogLevel); err != nil {
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
		if !c.GlobalConfig.EmbeddedGlobalConfig.EnableKubeAPIServer {
			log.Info("embedded kube-apiserver is disabled")
			return nil
		}
		if embeddedApiserverExecute != nil {
			if err := retry.Do(func() error {
				if err := embeddedApiserverExecute(ctx, log.WithValues("service", "kube-apiserver")); err != nil {
					return fmt.Errorf("API server error: %w", err)
				}
				return nil
			}, retry.Attempts(10), retry.Delay(2*time.Second), retry.Context(ctx)); err != nil {
				return err
			}
		}
		return nil
	})

	if numEnabled(c.GlobalConfig) == 0 {
		c.GlobalConfig.Backend = "pass"
	}
	switch c.GlobalConfig.Backend {
	case "kube":
		if c.GlobalConfig.EnableCRDMigrations {
			backendNoIndexes, err := newKubeBackend(ctx, c.GlobalConfig.BackendKubeConfig, "", c.GlobalConfig.BackendKubeNamespace, nil)
			if err != nil {
				return fmt.Errorf("failed to create kube backend with no indexes: %w", err)
			}
			// Wait for the API server to be healthy and ready.
			if err := backendNoIndexes.WaitForAPIServer(ctx, log, 20*time.Second, 5*time.Second, nil); err != nil {
				return fmt.Errorf("failed to wait for API server health: %w", err)
			}

			if err := crd.NewTinkerbell(crd.WithRestConfig(backendNoIndexes.ClientConfig)).MigrateAndReady(ctx); err != nil {
				cancel()
				gerr := g.Wait()
				return fmt.Errorf("CRD migrations failed: %w", errors.Join(err, gerr))
			}
			log.Info("CRD migrations completed")
		}

		b, err := newKubeBackend(ctx, c.GlobalConfig.BackendKubeConfig, "", c.GlobalConfig.BackendKubeNamespace, enabledIndexes(c.GlobalConfig.EnableSmee, c.GlobalConfig.EnableTootles, c.GlobalConfig.EnableTinkServer, c.GlobalConfig.EnableSecondStar))
		if err != nil {
			return fmt.Errorf("failed to create kube backend: %w", err)
		}
		c.SmeeConfig.Config.Backend = b
		c.TootlesConfig.Config.BackendEc2 = b
		c.TootlesConfig.Config.BackendHack = b
		c.TinkServerConfig.Config.Backend = b
		c.TinkServerConfig.Config.Auto.Enrollment.Backend = b
		c.TinkServerConfig.Config.Auto.Discovery.Backend = b
		c.TinkControllerConfig.Config.Client = b.ClientConfig
		c.TinkControllerConfig.Config.DynamicClient = b
		c.RufioConfig.Config.Client = b.ClientConfig
		c.SecondStarConfig.Config.Backend = b
	case "file":
		b, err := newFileBackend(ctx, log, c.GlobalConfig.BackendFilePath)
		if err != nil {
			return fmt.Errorf("failed to create file backend: %w", err)
		}
		c.SmeeConfig.Config.Backend = b
	case "none":
		b := newNoopBackend()
		c.SmeeConfig.Config.Backend = b
		c.TootlesConfig.Config.BackendEc2 = b
		c.TootlesConfig.Config.BackendHack = b
	case "pass":
	default:
		return fmt.Errorf("unknown backend %q", c.GlobalConfig.Backend)
	}

	// Kube Controller Manager
	g.Go(func() error {
		if !c.GlobalConfig.EmbeddedGlobalConfig.EnableKubeAPIServer {
			log.Info("embedded kube-controller-manager is disabled")
			return nil
		}
		if err := embeddedKubeControllerManagerExecute(ctx, c.GlobalConfig.BackendKubeConfig); err != nil {
			return fmt.Errorf("kube-controller-manager error: %w", err)
		}
		return nil
	})

	// Smee
	g.Go(func() error {
		if !c.GlobalConfig.EnableSmee {
			log.Info("smee service is disabled")
			return nil
		}
		ll := ternary((c.SmeeConfig.LogLevel != 0), c.SmeeConfig.LogLevel, c.GlobalConfig.LogLevel)
		if err := c.SmeeConfig.Config.Start(ctx, defaultLogger(ll).WithValues("service", "smee")); err != nil {
			return fmt.Errorf("failed to start smee service: %w", err)
		}
		return nil
	})

	// Tootles
	g.Go(func() error {
		if !c.GlobalConfig.EnableTootles {
			log.Info("tootles service is disabled")
			return nil
		}
		ll := ternary((c.TootlesConfig.LogLevel != 0), c.TootlesConfig.LogLevel, c.GlobalConfig.LogLevel)
		if err := c.TootlesConfig.Config.Start(ctx, defaultLogger(ll).WithValues("service", "tootles")); err != nil {
			return fmt.Errorf("failed to start tootles service: %w", err)
		}
		return nil
	})

	// Tink Server
	g.Go(func() error {
		if !c.GlobalConfig.EnableTinkServer {
			log.Info("tink server service is disabled")
			return nil
		}
		ll := ternary((c.TinkServerConfig.LogLevel != 0), c.TinkServerConfig.LogLevel, c.GlobalConfig.LogLevel)
		if err := c.TinkServerConfig.Config.Start(ctx, defaultLogger(ll).WithValues("service", "tink-server")); err != nil {
			return fmt.Errorf("failed to start tink server service: %w", err)
		}
		return nil
	})

	// Tink Controller
	g.Go(func() error {
		if !c.GlobalConfig.EnableTinkController {
			log.Info("tink controller service is disabled")
			return nil
		}
		ll := ternary((c.TinkControllerConfig.LogLevel != 0), c.TinkControllerConfig.LogLevel, c.GlobalConfig.LogLevel)
		if err := c.TinkControllerConfig.Config.Start(ctx, defaultLogger(ll).WithValues("service", "tink-controller")); err != nil {
			return fmt.Errorf("failed to start tink controller service: %w", err)
		}
		return nil
	})

	// Rufio Controller
	g.Go(func() error {
		if !c.GlobalConfig.EnableRufio {
			log.Info("rufio service is disabled")
			return nil
		}
		ll := ternary((c.RufioConfig.LogLevel != 0), c.RufioConfig.LogLevel, c.GlobalConfig.LogLevel)
		if err := c.RufioConfig.Config.Start(ctx, defaultLogger(ll).WithValues("service", "rufio")); err != nil {
			return fmt.Errorf("failed to start rufio service: %w", err)
		}
		return nil
	})

	// SecondStar
	g.Go(func() error {
		if !c.GlobalConfig.EnableSecondStar {
			log.Info("secondstar service is disabled")
			return nil
		}
		ll := ternary((c.SecondStarConfig.LogLevel != 0), c.SecondStarConfig.LogLevel, c.GlobalConfig.LogLevel)
		if err := c.SecondStarConfig.Config.Start(ctx, defaultLogger(ll).WithValues("service", "secondstar")); err != nil {
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
