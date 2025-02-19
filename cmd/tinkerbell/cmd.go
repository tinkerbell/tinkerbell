package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/tinkerbell/tinkerbell/cmd/tinkerbell/flag"
	"github.com/tinkerbell/tinkerbell/pkg/backend/kube"
	"github.com/tinkerbell/tinkerbell/rufio"
	"github.com/tinkerbell/tinkerbell/smee"
	"github.com/tinkerbell/tinkerbell/tink/controller"
	"github.com/tinkerbell/tinkerbell/tink/server"
	"github.com/tinkerbell/tinkerbell/tootles"
	"golang.org/x/sync/errgroup"
)

func Execute(ctx context.Context, args []string) error {
	globals := &flag.GlobalConfig{
		BackendKubeConfig: func() string {
			hd, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			p := filepath.Join(hd, ".kube", "config")
			// if this default location doesn't exist it's highly
			// likely that Tinkerbell is being run from within the
			// cluster. In that case, the loading of the Kubernetes
			// client will only look for in cluster configuration/environment
			// variables if this is empty.
			_, oserr := os.Stat(p)
			if oserr != nil {
				return ""
			}
			return p
		}(),
		PublicIP:             detectPublicIPv4(),
		EnableSmee:           true,
		EnableTootles:        true,
		EnableTinkServer:     true,
		EnableTinkController: true,
		EnableRufio:          true,
	}

	s := &flag.SmeeConfig{
		Config: smee.NewConfig(smee.Config{}, detectPublicIPv4()),
	}

	h := &flag.TootlesConfig{
		Config:   tootles.NewConfig(tootles.Config{}, fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 50061)),
		BindAddr: detectPublicIPv4(),
		BindPort: 50061,
	}
	ts := &flag.TinkServerConfig{
		Config:   server.NewConfig(),
		BindAddr: detectPublicIPv4(),
		BindPort: 42113,
	}
	controllerOpts := []controller.Option{
		controller.WithMetricsAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8080))),
		controller.WithProbeAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8081))),
	}
	tc := &flag.TinkControllerConfig{
		Config: controller.NewConfig(controllerOpts...),
	}

	rufioOpts := []rufio.Option{
		rufio.WithMetricsAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8082))),
		rufio.WithProbeAddr(netip.MustParseAddrPort(fmt.Sprintf("%s:%d", detectPublicIPv4().String(), 8083))),
		rufio.WithBmcConnectTimeout(2 * time.Minute),
		rufio.WithPowerCheckInterval(30 * time.Minute),
	}
	rc := &flag.RufioConfig{
		Config: rufio.NewConfig(rufioOpts...),
	}

	gfs := ff.NewFlagSet("globals")
	sfs := ff.NewFlagSet("smee - DHCP and iPXE service").SetParent(gfs)
	hfs := ff.NewFlagSet("tootles - Metadata service").SetParent(sfs)
	tfs := ff.NewFlagSet("tink server - Workflow server").SetParent(hfs)
	cfs := ff.NewFlagSet("tink controller - Workflow controller").SetParent(tfs)
	rfs := ff.NewFlagSet("rufio - BMC controller").SetParent(cfs)
	flag.RegisterGlobal(&flag.Set{FlagSet: gfs}, globals)
	flag.RegisterSmeeFlags(&flag.Set{FlagSet: sfs}, s)
	flag.RegisterTootlesFlags(&flag.Set{FlagSet: hfs}, h)
	flag.RegisterTinkServerFlags(&flag.Set{FlagSet: tfs}, ts)
	flag.RegisterTinkControllerFlags(&flag.Set{FlagSet: cfs}, tc)
	flag.RegisterRufioFlags(&flag.Set{FlagSet: rfs}, rc)

	cli := &ff.Command{
		Name:     "tinkerbell",
		Usage:    "tinkerbell [flags]",
		LongHelp: "Tinkerbell stack.",
		Flags:    rfs,
	}

	if err := cli.Parse(args, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		e := errors.New(ffhelp.Command(cli).String())
		if !errors.Is(err, ff.ErrHelp) {
			e = fmt.Errorf("%w\n%s", e, err)
		}

		return e
	}

	// Smee
	s.Convert(&globals.TrustedProxies, globals.PublicIP)

	// Tootles
	h.Convert(&globals.TrustedProxies)

	// Tink Server
	ts.Convert()

	log := defaultLogger(globals.LogLevel)
	log.Info("starting tinkerbell",
		"version", gitRevision(),
		"smeeEnabled", globals.EnableSmee,
		"tootlesEnabled", globals.EnableTootles,
		"tinkServerEnabled", globals.EnableTinkServer,
		"tinkControllerEnabled", globals.EnableTinkController,
		"rufioEnabled", globals.EnableRufio,
	)

	switch globals.Backend {
	case "kube":
		b, err := newKubeBackend(ctx, globals.BackendKubeConfig, "", globals.BackendKubeNamespace, enabledIndexes(globals.EnableSmee, globals.EnableTootles, globals.EnableTinkServer))
		if err != nil {
			return fmt.Errorf("failed to create kube backend: %w", err)
		}
		s.Config.Backend = b
		h.Config.BackendEc2 = b
		h.Config.BackendHack = b
		ts.Config.Backend = b
		tc.Config.Client = b.ClientConfig
		rc.Config.Client = b.ClientConfig
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
	default:
		return fmt.Errorf("unknown backend %q", globals.Backend)
	}

	g, ctx := errgroup.WithContext(ctx)

	// Smee
	g.Go(func() error {
		if globals.EnableSmee {
			return s.Config.Start(ctx, log.WithValues("service", "smee"))
		}
		log.Info("smee service is disabled")
		return nil
	})

	// Tootles
	g.Go(func() error {
		if globals.EnableTootles {
			return h.Config.Start(ctx, log.WithValues("service", "tootles"))
		}
		log.Info("tootles service is disabled")
		return nil
	})

	// Tink Server
	g.Go(func() error {
		if globals.EnableTinkServer {
			return ts.Config.Start(ctx, log.WithValues("service", "tink-server"))
		}
		log.Info("tink server service is disabled")
		return nil
	})

	// Tink Controller
	g.Go(func() error {
		if globals.EnableTinkController {
			if err := tc.Config.Start(ctx, log.WithValues("service", "tink-controller")); err != nil {
				return fmt.Errorf("failed to setup tink controller: %w", err)
			}
			return nil
		}
		log.Info("tink controller service is disabled")
		return nil
	})

	// Rufio
	g.Go(func() error {
		if globals.EnableRufio {
			err := rc.Config.Start(ctx, log.WithValues("service", "rufio"))
			if err != nil {
				return fmt.Errorf("failed to start rufio service: %w", err)
			}
			return nil
		}
		log.Info("rufio service is disabled")
		return nil
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	if !globals.EnableSmee && !globals.EnableTootles && !globals.EnableTinkServer && !globals.EnableTinkController && !globals.EnableRufio {
		return errors.New("all services are disabled")
	}

	return nil
}

func enabledIndexes(smeeEnabled, tootlesEnabled, tinkServerEnabled bool) map[kube.IndexType]kube.Index {
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
			// Don't log the function name.
			ss.Function = ""

			return a
		}

		// This changes the slog.Level string representation to an integer.
		// This makes it so that the V-levels passed in to the CLI show up as is in the logs.
		if a.Key == slog.LevelKey {
			b, ok := a.Value.Any().(slog.Level)
			if !ok {
				return a
			}
			a.Value = slog.Float64Value(math.Abs(float64(b)))
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
