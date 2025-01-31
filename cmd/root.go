package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/tinkerbell/tinkerbell/cmd/flag"
	"github.com/tinkerbell/tinkerbell/smee"
	"golang.org/x/sync/errgroup"
)

func Execute(ctx context.Context, args []string) error {
	s := &flag.SmeeConfig{
		Config: smee.NewConfig(smee.Config{}, DetectPublicIPv4()),
	}

	globals := &flag.GlobalConfig{
		BackendKubeConfig: func() string {
			hd, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			return filepath.Join(hd, ".kube", "config")
		}(),
		PublicIP: DetectPublicIPv4(),
	}
	gfs := ff.NewFlagSet("globals")
	sfs := ff.NewFlagSet("smee").SetParent(gfs)
	flag.RegisterGlobal(&flag.Set{FlagSet: gfs}, globals)
	flag.RegisterSmeeFlags(&flag.Set{FlagSet: sfs}, s)

	cli := &ff.Command{
		Name:     "tinkerbell",
		Usage:    "tinkerbell [flags]",
		LongHelp: "Tinkerbell stack.",
		Flags:    sfs,
	}
	if err := cli.Parse(args, ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		e := errors.New(ffhelp.Command(cli).String())
		if !errors.Is(err, ff.ErrHelp) {
			e = fmt.Errorf("%w\n%s", e, err)
		}

		return e
	}

	s.Config.DHCP.IPXEHTTPScript.URL.Host = func() string {
		addr, port := splitHostPort(s.Config.DHCP.IPXEHTTPScript.URL.Host)
		if s.DHCPIPXEScript.Host != "" {
			addr = s.DHCPIPXEScript.Host
		}
		if s.DHCPIPXEScript.Port != 0 {
			port = fmt.Sprintf("%d", s.DHCPIPXEScript.Port)
		}

		if port != "" {
			return fmt.Sprintf("%s:%s", addr, port)
		}
		return addr
	}()

	s.Config.DHCP.IPXEHTTPBinaryURL.Host = func() string {
		addr, port := splitHostPort(s.Config.DHCP.IPXEHTTPBinaryURL.Host)
		if s.DHCPIPXEBinary.Host != "" {
			addr = s.DHCPIPXEBinary.Host
		}
		if s.DHCPIPXEBinary.Port != 0 {
			port = fmt.Sprintf("%d", s.DHCPIPXEBinary.Port)
		}

		if port != "" {
			return fmt.Sprintf("%s:%s", addr, port)
		}
		return addr
	}()

	log := defaultLogger(globals.LogLevel)
	log.Info("starting tinkerbell")

	switch globals.Backend {
	case "kube":
		b, err := NewKubeBackend(ctx, globals.BackendKubeConfig, "", globals.BackendKubeNamespace)
		if err != nil {
			return fmt.Errorf("failed to create kube backend: %w", err)
		}
		s.Config.Backend = b
	case "file":
		b, err := NewFileBackend(ctx, log, globals.BackendFilePath)
		if err != nil {
			return fmt.Errorf("failed to create file backend: %w", err)
		}
		s.Config.Backend = b
	case "none":
		s.Config.Backend = NewNoopBackend()
	default:
		return fmt.Errorf("unknown backend %q", globals.Backend)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.Config.Start(ctx, log)
	})
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
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
			ss.Function = ""
			p := strings.Split(ss.File, "/")
			if len(p) > 3 {
				ss.File = filepath.Join(p[len(p)-3:]...)
			}

			return a
		}

		// This changes the slog.Level string representation to an integer.
		// This makes it so that the V-levels passed in to the CLI show up as is in the logs.
		if a.Key == slog.LevelKey {
			v, ok := a.Value.Any().(slog.Level)
			if !ok {
				return a
			}
			switch v {
			case slog.LevelError:
				a.Value = slog.IntValue(0)
			default:
				b, ok := a.Value.Any().(slog.Level)
				if !ok {
					return a
				}
				a.Value = slog.Float64Value(math.Abs(float64(b)))
			}
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
