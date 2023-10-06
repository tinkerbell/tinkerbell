package ipxedust

import (
	"context"
	"flag"
	"net/netip"
	"os"
	"time"

	"dario.cat/mergo"
	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
	"github.com/go-playground/validator/v10"
	"github.com/peterbourgon/ff/v3"
	"github.com/peterbourgon/ff/v3/ffcli"
	"github.com/rs/zerolog"
)

// Command represents the ipxe command.
type Command struct {
	// TFTPAddr is the TFTP server address:port.
	TFTPAddr string `validate:"required,hostname_port"`
	// TFTPBlockSize is the maximum block size for serving individual TFTP requests.
	TFTPBlockSize int `validate:"required,gte=512"`
	// TFTPTimeout is the timeout for serving individual TFTP requests.
	TFTPTimeout time.Duration `validate:"required,gte=1s"`
	// HTTPAddr is the HTTP server address:port.
	HTTPAddr string `validate:"required,hostname_port"`
	// HTTPTimeout is the timeout for serving individual HTTP requests.
	HTTPTimeout time.Duration `validate:"required,gte=1s"`
	// Log is the logging implementation.
	Log logr.Logger
	// LogLevel defines the logging level.
	LogLevel string
	// EnableTFTPSinglePort is a flag to enable single port mode for the TFTP server.
	// A standard TFTP server implementation receives requests on port 69 and
	// allocates a new high port (over 1024) dedicated to that request. In single
	// port mode, the same port is used for transmit and receive. If the server
	// is started on port 69, all communication will be done on port 69.
	// This option is required when running in a container that doesn't bind to the hosts
	// network because this type of dynamic port allocation is not generally supported.
	//
	// This option is specific to github.com/pin/tftp. The pin/tftp library says this option is
	// experimental and "Enabling this will negatively impact performance". Please take this into
	// consideration when using this option.
	EnableTFTPSinglePort bool
}

// Execute runs the ipxe command.
// Flags are registered, cli/env vars are parsed, the Command struct is validated,
// and the tftp and http services are run.
func Execute(ctx context.Context, args []string) error {
	c := &Command{}
	fs := flag.NewFlagSet("ipxe", flag.ExitOnError)
	c.RegisterFlags(fs)
	cmd := &ffcli.Command{
		Name:       "ipxe",
		ShortUsage: "Run TFTP and HTTP iPXE binary server",
		FlagSet:    fs,
		Options:    []ff.Option{ff.WithEnvVarPrefix("IPXE")},
		Exec: func(ctx context.Context, args []string) error {
			c.Log = defaultLogger(c.LogLevel)
			c.Log = c.Log.WithName("ipxe")
			if err := c.Validate(); err != nil {
				return err
			}

			return c.Run(ctx)
		},
	}
	return cmd.ParseAndRun(ctx, args)
}

// Run listens and serves the TFTP and HTTP services.
func (c *Command) Run(ctx context.Context) error {
	defaults := Command{
		TFTPAddr:      "0.0.0.0:69",
		TFTPBlockSize: 512,
		TFTPTimeout:   5 * time.Second,
		HTTPAddr:      "0.0.0.0:8080",
		HTTPTimeout:   5 * time.Second,
		Log:           logr.Discard(),
		LogLevel:      "info",
	}

	err := mergo.Merge(c, defaults)
	if err != nil {
		return err
	}
	tAddr, err := netip.ParseAddrPort(c.TFTPAddr)
	if err != nil {
		return err
	}
	hAddr, err := netip.ParseAddrPort(c.HTTPAddr)
	if err != nil {
		return err
	}
	srv := Server{
		TFTP: ServerSpec{
			Addr:      tAddr,
			BlockSize: c.TFTPBlockSize,
			Timeout:   c.TFTPTimeout,
		},
		HTTP: ServerSpec{
			Addr:    hAddr,
			Timeout: c.HTTPTimeout,
		},
		Log:                  c.Log,
		EnableTFTPSinglePort: c.EnableTFTPSinglePort,
	}
	return srv.ListenAndServe(ctx)
}

// RegisterFlags registers a flag set for the ipxe command.
func (c *Command) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&c.TFTPAddr, "tftp-addr", "0.0.0.0:69", "TFTP server address")
	f.IntVar(&c.TFTPBlockSize, "tftp-blocksize", 512, "TFTP server maximum block size")
	f.DurationVar(&c.TFTPTimeout, "tftp-timeout", time.Second*5, "TFTP server timeout")
	f.StringVar(&c.HTTPAddr, "http-addr", "0.0.0.0:8080", "HTTP server address")
	f.DurationVar(&c.HTTPTimeout, "http-timeout", time.Second*5, "HTTP server timeout")
	f.StringVar(&c.LogLevel, "log-level", "info", "Log level")
	f.BoolVar(&c.EnableTFTPSinglePort, "tftp-single-port", false, "Enable single port mode for TFTP server (needed for container deploys)")
}

// Validate checks the Command struct for validation errors.
func (c *Command) Validate() error {
	return validator.New().Struct(c)
}

// defaultLogger is a zerolog logr implementation.
func defaultLogger(level string) logr.Logger {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	zerologr.NameFieldName = "logger"
	zerologr.NameSeparator = "/"

	zl := zerolog.New(os.Stdout)
	zl = zl.With().Caller().Timestamp().Logger()
	var l zerolog.Level
	switch level {
	case "debug":
		l = zerolog.DebugLevel
	default:
		l = zerolog.InfoLevel
	}
	zl = zl.Level(l)

	return zerologr.New(&zl)
}
