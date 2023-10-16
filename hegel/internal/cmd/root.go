package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/equinix-labs/otel-init-go/otelinit"
	"github.com/gin-gonic/gin"
	"github.com/go-logr/zerologr"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tinkerbell/hegel/internal/backend"
	"github.com/tinkerbell/hegel/internal/backend/kubernetes"
	"github.com/tinkerbell/hegel/internal/frontend/ec2"
	"github.com/tinkerbell/hegel/internal/frontend/hack"
	"github.com/tinkerbell/hegel/internal/healthcheck"
	hegelhttp "github.com/tinkerbell/hegel/internal/http"
	hegellogger "github.com/tinkerbell/hegel/internal/logger"
	"github.com/tinkerbell/hegel/internal/metrics"
	"github.com/tinkerbell/hegel/internal/xff"
)

const longHelp = `
Run a Hegel server.

Each CLI argument has a corresponding environment variable in the form of the CLI argument prefixed
with HEGEL. If both the flag and environment variable form are specified, the flag form takes
precedence.

Examples
  --http-port          HEGEL_HTTP_PORT
  --trusted-proxies	   HEGEL_TRUSTED_PROXIES
`

// EnvNamePrefix defines the environment variable prefix required for all environment configuration.
const EnvNamePrefix = "HEGEL"

// RootCommandOptions encompasses all the configurability of the RootCommand.
type RootCommandOptions struct {
	TrustedProxies       string `mapstructure:"trusted-proxies"`
	HTTPAddr             string `mapstructure:"http-addr"`
	Backend              string `mapstructure:"backend"`
	KubernetesAPIServer  string `mapstructure:"kubernetes-apiserver"`
	KubernetesKubeconfig string `mapstructure:"kubernetes-kubeconfig"`
	KubernetesNamespace  string `mapstructure:"kubernetes-namespace"`
	FlatfilePath         string `mapstructure:"flatfile-path"`
	Debug                bool   `mapstructure:"debug"`

	// Hidden CLI flags.
	HegelAPI bool `mapstructure:"hegel-api"`
}

// RootCommand is the root command that represents the entrypoint to Hegel.
type RootCommand struct {
	*cobra.Command
	vpr  *viper.Viper
	Opts RootCommandOptions
}

// NewRootCommand creates new RootCommand instance.
func NewRootCommand() (*RootCommand, error) {
	rootCmd := &RootCommand{
		Command: &cobra.Command{
			Use:          os.Args[0],
			Long:         longHelp,
			SilenceUsage: true,
		},
	}

	rootCmd.PreRunE = rootCmd.PreRun
	rootCmd.RunE = rootCmd.Run
	rootCmd.Flags().SortFlags = false // Print flag help in the order they're specified.

	// Ensure keys with `-` use `_` for env keys else Viper won't match them.
	rootCmd.vpr = viper.NewWithOptions(viper.EnvKeyReplacer(strings.NewReplacer("-", "_")))
	rootCmd.vpr.SetEnvPrefix(EnvNamePrefix)

	if err := rootCmd.configureFlags(); err != nil {
		return nil, err
	}

	return rootCmd, nil
}

// PreRun satisfies cobra.Command.PreRunE and unmarshalls. Its responsible for populating c.Opts.
func (c *RootCommand) PreRun(*cobra.Command, []string) error {
	return c.vpr.Unmarshal(&c.Opts)
}

// Run executes Hegel.
func (c *RootCommand) Run(cmd *cobra.Command, _ []string) error {
	zl := zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	logger := zerologr.New(&zl)

	if !c.Opts.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	logger.Info("Root command options", "opts", fmt.Sprintf("%#v", c.Opts))

	ctx, otelShutdown := otelinit.InitOpenTelemetry(cmd.Context(), "hegel")
	defer otelShutdown(ctx)

	be, err := backend.New(ctx, toBackendOptions(c.Opts))
	if err != nil {
		return errors.Errorf("initialize backend: %v", err)
	}

	xffmw, err := xff.MiddlewareFromUnparsed(c.Opts.TrustedProxies)
	if err != nil {
		return err
	}

	registry := prometheus.NewRegistry()

	router := gin.New()
	router.Use(
		metrics.InstrumentRequestCount(registry),
		metrics.InstrumentRequestDuration(registry),
		gin.Recovery(),
		hegellogger.Middleware(logger),
		xffmw,
	)

	metrics.Configure(router, registry)
	healthcheck.Configure(router, be)

	// TODO(chrisdoherty4) Handle multiple frontends.
	fe := ec2.New(be)
	fe.Configure(router)

	hack.Configure(router, be)

	// Listen for signals to gracefully shutdown.
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	return hegelhttp.Serve(ctx, logger, c.Opts.HTTPAddr, router)
}

func (c *RootCommand) configureFlags() error {
	c.Flags().String(
		"trusted-proxies",
		"",
		"A commma separated list of allowed peer IPs and/or CIDR blocks to replace with X-Forwarded-For",
	)

	c.Flags().String("http-addr", ":50061", "Port to listen on for HTTP requests")

	c.Flags().String("backend", "kubernetes", "Backend to use for metadata. Options: flatfile, kubernetes")

	// Kubernetes backend specific flags.
	c.Flags().String("kubernetes-kubeconfig", "", "Path to a kubeconfig file")
	c.Flags().String("kubernetes-apiserver", "", "URL of the Kubernetes API Server")
	c.Flags().String("kubernetes-namespace", "", "The Kubernetes namespace to target; defaults to the service account")

	// Flatfile backend specific flags.
	c.Flags().String("flatfile-path", "", "Path to the flatfile metadata")

	c.Flags().Bool("debug", false, "Enable debug logging")

	c.Flags().Bool("hegel-api", false, "Toggle to true to enable Hegel's new experimental API. Default is false.")
	if err := c.Flags().MarkHidden("hegel-api"); err != nil {
		return err
	}

	if err := c.vpr.BindPFlags(c.Flags()); err != nil {
		return err
	}

	var err error
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if err != nil {
			return
		}
		err = c.vpr.BindEnv(f.Name)
	})

	return err
}

func toBackendOptions(opts RootCommandOptions) backend.Options {
	var backndOpts backend.Options
	switch opts.Backend {
	case "flatfile":
		backndOpts = backend.Options{
			Flatfile: &backend.Flatfile{
				Path: opts.FlatfilePath,
			},
		}
	case "kubernetes":
		backndOpts = backend.Options{
			Kubernetes: &kubernetes.Config{
				APIServerAddress: opts.KubernetesAPIServer,
				Kubeconfig:       opts.KubernetesKubeconfig,
				Namespace:        opts.KubernetesNamespace,
			},
		}
	}
	return backndOpts
}
