package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/tinkerbell/tinkerbell/cmd"
	"github.com/tinkerbell/tinkerbell/cmd/flag"
	"github.com/tinkerbell/tinkerbell/smee"
	"golang.org/x/sync/errgroup"
)

func main() {
	var exitCode int
	defer func() { os.Exit(exitCode) }()

	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM)
	defer done()

	s := &flag.SmeeConfig{
		// These are the default values for the smee configuration.
		Config: smee.NewConfig(smee.Config{
			DHCP: smee.DHCP{
				BindAddr:    netip.MustParseAddr("0.0.0.0"),
				IPForPacket: cmd.DetectPublicIPv4(),
				SyslogIP:    cmd.DetectPublicIPv4(),
				TFTPIP:      cmd.DetectPublicIPv4(),
				IPXEHTTPBinaryURL: &url.URL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s:%v", cmd.DetectPublicIPv4(), smee.DefaultHTTPPort),
					Path:   "/ipxe",
				},
				IPXEHTTPScript: smee.IPXEHTTPScript{
					URL: &url.URL{
						Scheme: "http",
						Host:   fmt.Sprintf("%s:%v", cmd.DetectPublicIPv4(), smee.DefaultHTTPPort),
						Path:   "auto.ipxe",
					},
				},
			},
			IPXE: smee.IPXE{
				HTTPScriptServer: smee.IPXEHTTPScriptServer{
					BindAddr: cmd.DetectPublicIPv4(),
				},
			},
			Syslog: smee.Syslog{
				BindAddr: cmd.DetectPublicIPv4(),
			},
			TFTP: smee.TFTP{
				BindAddr: cmd.DetectPublicIPv4(),
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
		PublicIP: cmd.DetectPublicIPv4(),
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
	if err := cli.Parse(os.Args[1:], ff.WithEnvVarPrefix("TINKERBELL")); err != nil {
		fmt.Fprintln(os.Stderr, ffhelp.Command(cli))
		if !errors.Is(err, ff.ErrHelp) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}

		exitCode = 1
		return
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
		b, err := cmd.NewKubeBackend(ctx, globals.BackendKubeConfig, "", globals.BackendKubeNamespace)
		if err != nil {
			log.Error(err, "failed to create kube backend")
			exitCode = 1
			return
		}
		s.Config.Backend = b
	case "file":
		b, err := cmd.NewFileBackend(ctx, log, globals.BackendFilePath)
		if err != nil {
			log.Error(err, "failed to create file backend")
			exitCode = 1
			return
		}
		s.Config.Backend = b
	case "none":
		s.Config.Backend = cmd.NewNoopBackend()
	default:
		log.Error(fmt.Errorf("unknown backend %q", globals.Backend), "failed to create backend")
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return s.Config.Start(ctx, log)
	})
	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		panic(err)
	}
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
