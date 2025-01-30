package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
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
		// These are the default values for the smee configuration.
		Config: smee.NewConfig(smee.Config{
			DHCP: smee.DHCP{
				BindAddr:    netip.MustParseAddr("0.0.0.0"),
				IPForPacket: DetectPublicIPv4(),
				SyslogIP:    DetectPublicIPv4(),
				TFTPIP:      DetectPublicIPv4(),
				IPXEHTTPBinaryURL: &url.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s:%v", DetectPublicIPv4(), smee.DefaultHTTPPort),
					Path:   "/ipxe",
				},
				IPXEHTTPScript: smee.IPXEHTTPScript{
					URL: &url.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s:%v", DetectPublicIPv4(), smee.DefaultHTTPPort),
						Path:   "auto.ipxe",
					},
				},
			},
			IPXE: smee.IPXE{
				HTTPScriptServer: smee.IPXEHTTPScriptServer{
					BindAddr: DetectPublicIPv4(),
				},
			},
			Syslog: smee.Syslog{
				BindAddr: DetectPublicIPv4(),
			},
			TFTP: smee.TFTP{
				BindAddr: DetectPublicIPv4(),
			},
		}),
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

	s.Config.DHCP.IPXEHTTPBinaryURL = &url.URL{
		Scheme: s.DHCPIPXEScript.Scheme,
		Host:   fmt.Sprintf("%s:%v", s.DHCPIPXEScript.Host, s.DHCPIPXEScript.Port),
		Path:   s.DHCPIPXEScript.Path,
	}

	s.Config.DHCP.IPXEHTTPBinaryURL = &url.URL{
		Scheme: s.DHCPIPXEBinary.Scheme,
		Host:   fmt.Sprintf("%s:%v", s.DHCPIPXEBinary.Host, s.DHCPIPXEBinary.Port),
		Path:   s.DHCPIPXEBinary.Path,
	}

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
func defaultLogger(level string) logr.Logger {
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

		return a
	}
	opts := &slog.HandlerOptions{AddSource: true, ReplaceAttr: customAttr}
	switch level {
	case "debug":
		opts.Level = slog.LevelDebug
	default:
		opts.Level = slog.LevelInfo
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, opts))

	return logr.FromSlogHandler(log.Handler())
}
